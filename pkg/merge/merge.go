/*
Copyright © 2025-2026 SUSE LLC
SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package merge

import (
	"bytes"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

// ChangeType represents the kind of change made to a file relative to a
// common ancestor. The zero value is the empty string, deliberately not a
// valid change, so an uninitialised ChangeType reads as obviously missing.
type ChangeType string

const (
	ChangeTypeAdded    ChangeType = "added"
	ChangeTypeModified ChangeType = "modified"
	ChangeTypeDeleted  ChangeType = "deleted"
)

const MaxContentCompareSize = 16 * 1024 * 1024

// Conflict represents a file the user and the OS both changed relative to
// the common ancestor.
type Conflict struct {
	Path       string
	UserChange ChangeType
	OSChange   ChangeType
}

func (c Conflict) String() string {
	return fmt.Sprintf("  %s — user: %s, OS: %s", c.Path, c.UserChange, c.OSChange)
}

var skippedDirNames = map[string]bool{
	".snapshots": true,
}

// DetectConflicts walks oldRoot and newRoot to derive the OS defaults delta,
// then returns the files that also appear in userChanges (i.e. that both the
// user and the OS modified relative to the common ancestor).
//
// userChanges is keyed by rel-path with a leading "/" (e.g. "/sshd_config"),
// matching what the walk produces.
//
// Regular files are compared by size and then content (up to
// MaxContentCompareSize -- larger matched-size files are conservatively
// flagged as modified without being read). Symlinks compared by target.
// Type changes (file->dir, file->symlink, ...) reported as modified.
// Directories whose basename is in skippedDirNames (e.g. snapper's
// ".snapshots") are not descended into.
func DetectConflicts(fs vfs.FS, userChanges map[string]ChangeType, oldRoot, newRoot string) ([]Conflict, error) {
	osChanges, err := computeOSChanges(fs, oldRoot, newRoot)
	if err != nil {
		return nil, err
	}

	var conflicts []Conflict
	for path, userChange := range userChanges {
		osChange, ok := osChanges[path]
		if !ok {
			continue
		}
		conflicts = append(conflicts, Conflict{
			Path:       path,
			UserChange: userChange,
			OSChange:   osChange,
		})
	}
	slices.SortFunc(conflicts, func(a, b Conflict) int {
		return strings.Compare(a.Path, b.Path)
	})
	return conflicts, nil
}

// FormatConflictSummary returns a human-readable summary of detected
// conflicts; returns "" if there are none.
func FormatConflictSummary(volumePath string, conflicts []Conflict) string {
	if len(conflicts) == 0 {
		return ""
	}

	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "Merge conflicts detected for %s (%d file(s)), user version kept:\n",
		volumePath, len(conflicts))
	for _, c := range conflicts {
		_, _ = fmt.Fprintf(&b, "%s\n", c)
	}

	return b.String()
}

func computeOSChanges(fs vfs.FS, oldRoot, newRoot string) (map[string]ChangeType, error) {
	oldEntries, err := walkEntries(fs, oldRoot)
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", oldRoot, err)
	}

	newEntries, err := walkEntries(fs, newRoot)
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", newRoot, err)
	}

	changes := make(map[string]ChangeType)
	for rel, newInfo := range newEntries {
		oldInfo, ok := oldEntries[rel]
		if !ok {
			if !newInfo.IsDir() {
				changes[rel] = ChangeTypeAdded
			}
			continue
		}
		delete(oldEntries, rel)

		differs, err := entriesDiffer(fs, oldInfo, newInfo, filepath.Join(oldRoot, rel), filepath.Join(newRoot, rel))
		if err != nil {
			return nil, err
		}
		if differs {
			changes[rel] = ChangeTypeModified
		}
	}

	for rel, oldInfo := range oldEntries {
		if oldInfo.IsDir() {
			continue
		}
		changes[rel] = ChangeTypeDeleted
	}
	return changes, nil
}

// walkEntries walks root and returns a map of rel-path → FileInfo. The root
// itself is excluded and directories listed in skippedDirNames are not
// descended into. Returns an empty map (no error) if root does not exist.
func walkEntries(fs vfs.FS, root string) (map[string]iofs.FileInfo, error) {
	entries := make(map[string]iofs.FileInfo)
	if _, err := fs.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, err
	}
	err := vfs.WalkDirFs(fs, root, func(path string, d iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if d.IsDir() && skippedDirNames[d.Name()] {
			return filepath.SkipDir
		}
		rel := strings.TrimPrefix(path, root)
		info, err := d.Info()
		if err != nil {
			return err
		}
		entries[rel] = info
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// entriesDiffer compares two entries that exist at the same relative path on
// both sides and reports whether they should be considered modified.
func entriesDiffer(fs vfs.FS, oldInfo, newInfo iofs.FileInfo, oldPath, newPath string) (bool, error) {
	if oldInfo.Mode().Type() != newInfo.Mode().Type() {
		return true, nil
	}
	if oldInfo.IsDir() {
		return false, nil
	}
	if oldInfo.Mode()&os.ModeSymlink != 0 {
		oldTarget, err := fs.Readlink(oldPath)
		if err != nil {
			return false, err
		}
		newTarget, err := fs.Readlink(newPath)
		if err != nil {
			return false, err
		}
		return oldTarget != newTarget, nil
	}
	if !oldInfo.Mode().IsRegular() {
		return false, nil
	}
	if oldInfo.Size() != newInfo.Size() {
		return true, nil
	}
	if oldInfo.Size() > MaxContentCompareSize {
		// Bigger than the cap — conservatively report modified instead of
		// reading the file. Worst case is a false-positive conflict warning.
		return true, nil
	}
	return regularFilesContentDiffer(fs, oldPath, newPath)
}

func regularFilesContentDiffer(fs vfs.FS, oldPath, newPath string) (bool, error) {
	oldF, err := fs.Open(oldPath)
	if err != nil {
		return false, err
	}
	defer oldF.Close()
	newF, err := fs.Open(newPath)
	if err != nil {
		return false, err
	}
	defer newF.Close()

	const bufSize = 32 * 1024
	bufA := make([]byte, bufSize)
	bufB := make([]byte, bufSize)
	for {
		nA, errA := io.ReadFull(oldF, bufA)
		nB, errB := io.ReadFull(newF, bufB)
		if nA != nB || !bytes.Equal(bufA[:nA], bufB[:nB]) {
			return true, nil
		}
		if errA == io.EOF || errA == io.ErrUnexpectedEOF {
			return false, nil
		}
		if errA != nil {
			return false, errA
		}
		if errB != nil {
			return false, errB
		}
	}
}
