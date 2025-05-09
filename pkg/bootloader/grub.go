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
	"github.com/suse/elemental/v3/pkg/sys"
)

type Grub struct {
	s *sys.System
}

func NewGrub(s *sys.System) *Grub {
	return &Grub{s}
}

// Install installs the bootloader to the specified root.
func (g *Grub) Install(rootPath string) error {
	g.s.Logger().Info("Installing GRUB bootloader")

	// InstallElementalEFI()
	// CopyKernelInitrd()
	// UpdateBootEntries()
	return nil
}
