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

package filesystem

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

// CreateEmptyFile creates a file of the given size in MiB.
func CreateEmptyFile(fs vfs.FS, filename string, size int64, noSparse bool) error {
	var eof byte

	f, err := fs.Create(filename)
	if err != nil {
		return fmt.Errorf("failed creating image file %s: %w", filename, err)
	}
	err = f.Truncate(int64(size * 1024 * 1024))
	if err != nil {
		f.Close()
		_ = fs.RemoveAll(filename)
		return fmt.Errorf("could not truncate image file %s: %w", filename, err)
	}

	if noSparse {
		_, err = f.Seek(-1, io.SeekEnd)
		if err != nil {
			return fmt.Errorf("could not seek image file %s: %w", filename, err)
		}
		_, err := f.Write([]byte{eof})
		if err != nil {
			return fmt.Errorf("cloud not write last byte of image file %s: %w", filename, err)
		}
	}

	err = f.Close()
	if err != nil {
		_ = fs.RemoveAll(filename)
		return fmt.Errorf("could not close image file %s: %w", filename, err)
	}
	return nil
}

// CreatePreloadedFileSystemImage creates a new raw image with the given filesystem. The size of the image
// is computed form the provided root tree size plus the given overhead. The resulting image size is aligned
// with the given overhead and has a minimum of a full overhead of free space.
func CreatePreloadedFileSystemImage(s *sys.System, root, filename, label string, overheadM int64, fs deployment.FileSystem) error {
	size, err := vfs.DirSize(s.FS(), root)
	if err != nil {
		return fmt.Errorf("could not compute required image size: %w", err)
	}

	// align size to the next full overhead slot (in MiB)
	overheadB := int64(overheadM * 1024 * 1024)
	sizeMB := ((size/overheadB + 2) * overheadB) / (1024 * 1024)

	err = CreateEmptyFile(s.FS(), filename, sizeMB, true)
	if err != nil {
		return fmt.Errorf("could not create filesystem image file %s: %w", filename, err)
	}

	flags := []string{}
	switch fs {
	case deployment.Ext2, deployment.Ext4:
		flags = append(flags, "-d", root)
	case deployment.Btrfs:
		flags = append(flags, "--root-dir", root)
	case deployment.VFat:
	default:
		return fmt.Errorf("preloaded image is not supported for %s: %w", fs.String(), errors.ErrUnsupported)
	}

	mkfsCall := NewMkfsCall(s, filename, fs.String(), label, "", flags...)
	err = mkfsCall.Apply()
	if err != nil {
		return fmt.Errorf("failed formatting preloaded filesystem image %s: %w", filename, err)
	}

	if fs == deployment.VFat {
		files, err := s.FS().ReadDir(root)
		if err != nil {
			return fmt.Errorf("failed reading files from root tree: %w", err)
		}

		for _, f := range files {
			_, err = s.Runner().Run("mcopy", "-s", "-i", filename, filepath.Join(root, f.Name()), "::")
			if err != nil {
				return fmt.Errorf("failed copying file %s to the vfat image %s: %w", f.Name(), filename, err)
			}
		}
	}
	return nil
}
