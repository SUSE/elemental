/*
Copyright © 2022-2026 SUSE LLC
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

	"github.com/suse/elemental/v3/pkg/sys"
)

type Bootloader interface {
	Install(i InstallCtx) error
	InstallLive(i InstallCtx) error
	InstallNetboot(i InstallCtx) error
	Prune(rootPath, espDir string, keepEntryIDs []int) error
}

// InstallCtx defines the parameters requierd by the bootloader to perform an installation
type InstallCtx struct {
	// RootDir is the path for the root tree of the system to install the bootloader for. This path
	// is expected to be a full OS including the kernel, initrd and bootloader binaries at default
	// known locations.
	RootDir string

	// Target is the path where the bootloader binaries and configuration should be installed. This is
	// typically the mountpoint of the ESP partition.
	Target string

	// ESPLabel is the filesystem label of the ESP partition.
	ESPLabel string

	// EntryID this is the ID if the current bootloader entry to be installed.
	EntryID string

	// KernelCmdline is the full kernel command line we want to set for this specific entry.
	KernelCmdline string

	// RecKernelCmdline is the full kernel command line for the recovery entry, if any.
	RecKernelCmdline string

	// InitrdExtensions is the list of CPIO files to stack into the stock initrd. These CPIO files are mostly
	// used to inject additional setup into the stock initrd.
	InitrdExtensions []string
}

const (
	BootNone = "none"
	BootGrub = "grub"
)

type None struct {
	s *sys.System
}

func NewNone(s *sys.System) *None {
	return &None{s}
}

func (n *None) Install(_ InstallCtx) error {
	n.s.Logger().Info("Skipping bootloader installation")
	return nil
}

func (n *None) InstallLive(_ InstallCtx) error {
	n.s.Logger().Info("Skipping bootloader installation")
	return nil
}

func (n *None) InstallNetboot(_ InstallCtx) error {
	n.s.Logger().Info("Skipping bootloader installation")
	return nil
}

func (n *None) Prune(_, _ string, _ []int) error {
	n.s.Logger().Info("Skipping bootloader pruning")
	return nil
}

func New(name string, s *sys.System) (Bootloader, error) {
	switch name {
	case BootNone:
		return NewNone(s), nil
	case BootGrub:
		return NewGrub(s), nil
	}

	return nil, fmt.Errorf("new bootloader '%s': %w", name, errors.ErrUnsupported)
}
