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

package btrfs

import (
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const TopSubVol = "@"

// EnableQuota enables btrfs quota the btrfs filesystem, path is usually the
// mountpoint of the btrfs filesystem
func EnableQuota(s *sys.System, path string) error {
	s.Logger().Debug("Enabling btrfs quota")
	cmdOut, err := s.Runner().Run("btrfs", "quota", "enable", path)
	if err != nil {
		return fmt.Errorf("setting quota for btrfs partition at %s: %s: %w", path, string(cmdOut), err)
	}
	return nil
}

// CreateSubvolume creates a btrfs subvolume to the given path
func CreateSubvolume(s *sys.System, path string, copyOnWrite bool) error {
	s.Logger().Debug("Creating subvolume: %s", path)
	err := vfs.MkdirAll(s.FS(), filepath.Dir(path), vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("creating subvolume path %s: %w", path, err)
	}
	cmdOut, err := s.Runner().Run("btrfs", "subvolume", "create", path)
	if err != nil {
		return fmt.Errorf("creating subvolume %s: %s: %w", path, string(cmdOut), err)
	}
	if !copyOnWrite {
		return NoCopyOnWrite(s, path)
	}
	return nil
}

// NoCopyOnWrite disables copy on write to the given subvolume
func NoCopyOnWrite(s *sys.System, path string) error {
	cmdOut, err := s.Runner().Run("chattr", "+C", path)
	if err != nil {
		return fmt.Errorf("setting no copy on write for volume '%s': %s: %w", path, string(cmdOut), err)
	}
	return nil
}

// CreateSnapshot creates a btrfs snapshot to the given path from the given base
func CreateSnapshot(s *sys.System, path, base string, copyOnWrite bool) error {
	s.Logger().Debug("Creating snapshot: %s", path)
	err := vfs.MkdirAll(s.FS(), filepath.Dir(path), vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("creating snapshot subvolume path %s: %w", path, err)
	}

	cmdOut, err := s.Runner().Run("btrfs", "subvolume", "snapshot", base, path)
	if err != nil {
		return fmt.Errorf("creating snapshot subvolume '%s': %s: %w", path, string(cmdOut), err)
	}
	if !copyOnWrite {
		return NoCopyOnWrite(s, path)
	}
	return nil
}

// CreateQuotaGroup creates the given quota group for the btrfs filesystem,
// path is usually the mountpoint of the btrfs filesystem
func CreateQuotaGroup(s *sys.System, path, qGroup string) error {
	s.Logger().Debug("Create btrfs quota group")
	cmdOut, err := s.Runner().Run("btrfs", "qgroup", "create", qGroup, path)
	if err != nil {
		return fmt.Errorf("creating quota group for %s: %s: %w", path, string(cmdOut), err)
	}
	return nil
}

// SetBtrfsPartition configures toplevel subvolume, enables quota sets the quota group 1/0,
// and defines the toplevel subvolume as the default subvolume. Path is the mountpoint of the btrfs filesystem.
func SetBtrfsPartition(s *sys.System, path string) error {
	err := EnableQuota(s, path)
	if err != nil {
		return err
	}
	subvolume := filepath.Join(path, TopSubVol)
	err = CreateSubvolume(s, subvolume, true)
	if err != nil {
		return err
	}
	err = CreateQuotaGroup(s, path, "1/0")
	if err != nil {
		return err
	}
	return SetDefaultSubvolume(s, subvolume)
}

// DeleteSubvolume removes the given subvolume. Before removing the subvolume
// it sets the RW property to ensure it can be deleted, if deletion fails
// the property change remains applied.
func DeleteSubvolume(s *sys.System, path string) error {
	s.Logger().Debug("Setting rw property to subvolume: %s", path)
	_, err := s.Runner().Run("btrfs", "property", "set", "-ts", path, "ro", "false")
	if err != nil {
		return fmt.Errorf("setting rw permissions before deletion: %w", err)
	}
	_, err = s.Runner().Run("btrfs", "subvolume", "delete", "-c", "-R", path)
	return err
}

// SetDefaultSubvolume sets the given subvolume as the default subvolume to mount
func SetDefaultSubvolume(s *sys.System, path string) error {
	s.Logger().Debug("Setting default subvolume")
	_, err := s.Runner().Run("btrfs", "subvolume", "set-default", path)
	if err != nil {
		return fmt.Errorf("setting default subvolume to '%s': %w", path, err)
	}
	return nil
}
