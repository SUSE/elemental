/*
Copyright © 2022-2025 SUSE LLC
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

package bootloader

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type Grub struct {
	s *sys.System
}

func NewGrub(s *sys.System) *Grub {
	return &Grub{s}
}

// Install installs the bootloader to the specified root.
func (g *Grub) Install(rootPath string, esp *deployment.Partition) error {
	if esp == nil {
		g.s.Logger().Error("ESP not found")
		return fmt.Errorf("ESP not found")
	}

	g.s.Logger().Info("Installing GRUB bootloader to partition '%s'", esp.Label)

	if esp.Role != deployment.EFI {
		g.s.Logger().Error("")
		return fmt.Errorf("%w: installing bootloader to partition role %d", errors.ErrUnsupported, esp.Role)
	}

	err := g.installElementalEFI(rootPath, esp)
	if err != nil {
		g.s.Logger().Error("Error installing elemental EFI app: %s", err.Error())
		return err
	}

	// CopyKernelInitrd()
	// UpdateBootEntries()
	return nil
}

func (g *Grub) installElementalEFI(rootPath string, esp *deployment.Partition) error {
	g.s.Logger().Info("Installing EFI applications")

	targetDir := filepath.Join(rootPath, esp.MountPoint, "EFI", "ELEMENTAL")
	err := vfs.MkdirAll(g.s.FS(), targetDir, vfs.DirPerm)
	if err != nil {
		g.s.Logger().Error("Error creating EFI dir '%s': %s", targetDir, err.Error())
		return err
	}

	srcDir := filepath.Join(rootPath, "usr", "share", "efi", g.s.Platform().Arch)
	for _, name := range []string{"shim.efi", "grub.efi", "MokManager.efi"} {
		src := filepath.Join(srcDir, name)
		target := filepath.Join(targetDir, name)
		err = vfs.CopyFile(g.s.FS(), src, target)
		if err != nil {
			g.s.Logger().Error("Error copying EFI app '%s': %s", src, err.Error())
			return err
		}
	}

	return nil
}
