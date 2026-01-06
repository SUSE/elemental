/*
Copyright © 2021-2026 SUSE LLC
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

package firmware

import (
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/platform"
)

const (
	EfiEntryPath     = "/EFI/ELEMENTAL"
	EfiFallbackPath  = "/EFI/BOOT"
	EfivarsMountPath = "/sys/firmware/efi/efivars"
	EfiBootEntryName = "elemental-shim"
	EfiImgX86        = "bootx64.efi"
	EfiImgArm64      = "bootaa64.efi"
	EfiImgRiscv64    = "bootriscv64.efi"
)

// EfiBootManager contains logic to update the EFI variables and boot-entries for a system.
type EfiBootManager struct {
	s *sys.System
}

// EfiBootEntry contains information about a EFI boot entry.
type EfiBootEntry struct {
	Label  string
	Loader string
	Disk   string
}

// NewEfiBootManager creates a new EfiBootManager.
func NewEfiBootManager(s *sys.System) *EfiBootManager {
	return &EfiBootManager{s}
}

// CreateBootEntries creates the EFI boot entries using efibootmgr.
func (b *EfiBootManager) CreateBootEntries(entries []*EfiBootEntry) error {
	b.s.Logger().Info("Creating %d boot entries...", len(entries))

	for _, entry := range entries {
		cmdOut, err := b.s.Runner().Run("efibootmgr", "--create", "--disk", entry.Disk, "--label", entry.Label, "--loader", entry.Loader)
		if err != nil {
			b.s.Logger().Error("failed creating boot entry (%s): %s", err.Error(), string(cmdOut))
			return err
		}
	}

	return nil
}

// DefaultBootEntry generates the default EFI boot entry for the platform.
func DefaultBootEntry(p *platform.Platform, disk string) *EfiBootEntry {
	efiImgName := ""
	switch p.Arch {
	case platform.Archx86:
		efiImgName = EfiImgX86
	case platform.ArchAarch64:
		efiImgName = EfiImgArm64
	case platform.ArchRiscv64:
		efiImgName = EfiImgRiscv64
	}

	return &EfiBootEntry{
		Label:  EfiBootEntryName,
		Loader: filepath.Join(EfiEntryPath, efiImgName),
		Disk:   disk,
	}
}
