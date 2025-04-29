/*
Copyright © 2025 SUSE LLC
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
	"context"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/archive"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type Tar struct {
	s       *sys.System
	tarball string
}

func NewTarUnpacker(s *sys.System, tarball string) *Tar {
	return &Tar{s: s, tarball: tarball}
}

func (t Tar) Unpack(ctx context.Context, destination string) (string, error) {
	err := archive.ExtractTarball(ctx, t.s, t.tarball, destination)
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
	unpackD := NewDirectoryUnpacker(t.s, tempDir)
	digest, err = unpackD.SynchedUnpack(ctx, destination, excludes, deleteExcludes)
	if err != nil {
		return "", err
	}
	return digest, nil
}
