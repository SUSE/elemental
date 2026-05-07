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

// MaxContentCompareSize bounds how large a regular file may be before
// FileChange stops comparing its content byte-for-byte. Files above this
// size whose stat metadata matches are reported as modified rather than
// read in full; this trades a small false-positive risk for bounded I/O
// on user-supplied blobs.
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

// FileChange reports how a single file changed between oldPath and newPath.
// Returns empty string if the file is unchanged (or absent from both sides).
//
// Regular files are compared by size and then content (up to
// MaxContentCompareSize; larger matched-size files are conservatively
// flagged as modified without being read). Symlinks compared by target.
// Type changes (file->dir, file->symlink, ...) reported as modified.
// Directory-only additions/deletions are ignored (contents at those paths
// are what's meaningful, and snapper reports them individually).
func FileChange(fs vfs.FS, oldPath, newPath string) (ChangeType, error) {
	oldInfo, oldErr := fs.Lstat(oldPath)
	if oldErr != nil && !os.IsNotExist(oldErr) {
		return "", oldErr
	}
	newInfo, newErr := fs.Lstat(newPath)
	if newErr != nil && !os.IsNotExist(newErr) {
		return "", newErr
	}

	oldExists := oldErr == nil
	newExists := newErr == nil

	switch {
	case !oldExists && !newExists:
		return "", nil
	case !oldExists:
		if newInfo.IsDir() {
			return "", nil
		}
		return ChangeTypeAdded, nil
	case !newExists:
		if oldInfo.IsDir() {
			return "", nil
		}
		return ChangeTypeDeleted, nil
	}

	differs, err := entriesDiffer(fs, oldInfo, newInfo, oldPath, newPath)
	if err != nil {
		return "", err
	}
	if differs {
		return ChangeTypeModified, nil
	}
	return "", nil
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
		// Bigger than the cap; conservatively report modified instead of
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
