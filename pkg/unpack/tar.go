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

package unpack

import (
	"archive/tar"
	"context"
	"path/filepath"
	"strings"

	"github.com/suse/elemental/v3/pkg/archive"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type Tar struct {
	s          *sys.System
	tarball    string
	rsyncFlags []string
}

type TarOpt func(*Tar)

func WithRsyncFlagsTar(flags ...string) TarOpt {
	return func(t *Tar) {
		t.rsyncFlags = flags
	}
}

func NewTarUnpacker(s *sys.System, tarball string, opts ...TarOpt) *Tar {
	t := &Tar{s: s, tarball: tarball}
	for _, o := range opts {
		o(t)
	}
	return t
}

func (t Tar) Unpack(ctx context.Context, destination string, excludes ...string) (string, error) {
	err := archive.ExtractTarball(ctx, t.s, t.tarball, destination, excludesFilter(destination, excludes...))
	return "", err
}

// SynchedUnpack for tarball files will extract tar contents to a destination sibling directory first and
// after that it will sync it to the destination directory. Ideally the destination path should
// not be mountpoint to a different filesystem of the sibling directories in order to benefit of
// copy on write features of the underlaying filesystem.
func (t Tar) SynchedUnpack(ctx context.Context, destination string, excludes []string, deleteExcludes []string) (digest string, err error) {
	tempDir := filepath.Clean(destination) + workDirSuffix
	err = vfs.MkdirAll(t.s.FS(), tempDir, vfs.DirPerm)
	if err != nil {
		return "", err
	}
	defer func() {
		e := vfs.ForceRemoveAll(t.s.FS(), tempDir)
		if err == nil && e != nil {
			err = e
		}
	}()
	_, err = t.Unpack(ctx, tempDir)
	if err != nil {
		return "", err
	}
	unpackD := NewDirectoryUnpacker(t.s, tempDir, WithRsyncFlagsDir(t.rsyncFlags...))
	digest, err = unpackD.SynchedUnpack(ctx, destination, excludes, deleteExcludes)
	if err != nil {
		return "", err
	}
	return digest, nil
}

// excludesFilter returns a filter to exclude given path in a tarball extraction. Given paths
// are assumed to be always tied to tarball root
func excludesFilter(root string, excludes ...string) func(h *tar.Header) (bool, error) {
	rootedExcl := make([]string, len(excludes))
	for i, exclude := range excludes {
		rootedExcl[i] = filepath.Clean(filepath.Join(root, exclude))
	}
	return func(h *tar.Header) (bool, error) {
		fullName := filepath.Join(root, filepath.Clean(h.Name))
		for _, exclude := range rootedExcl {
			if after, ok := strings.CutPrefix(fullName, exclude); ok {
				if after == "" {
					return false, nil
				}
				return !filepath.IsAbs(after), nil
			}
		}
		return true, nil
	}
}
