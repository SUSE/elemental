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
	_ "embed"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/rsync"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/platform"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type Grub struct {
	s *sys.System
}

type grubBootEntry struct {
	linux       string
	initrd      string
	cmdline     string
	displayName string
	id          string
}

func NewGrub(s *sys.System) *Grub {
	return &Grub{s}
}

const (
	OsReleasePath = "/etc/os-release"
	Initrd        = "initrd"
	DefaultBootID = "active"
)

//go:embed grub.cfg
var grubCfg []byte

// Install installs the bootloader to the specified root.
func (g *Grub) Install(rootPath, snapshotID, kernelCmdline string, d *deployment.Deployment) error {
	esp := d.GetEfiSystemPartition()
	if esp == nil {
		return fmt.Errorf("ESP not found")
	}

	g.s.Logger().Info("Installing GRUB bootloader to partition '%s'", esp.Label)

	if esp.Role != deployment.EFI {
		return fmt.Errorf("installing bootloader to partition role %s: %w", esp.Role, errors.ErrUnsupported)
	}

	err := g.installElementalEFI(rootPath, esp)
	if err != nil {
		return fmt.Errorf("installing elemental EFI apps: %w", err)
	}

	err = g.installGrub(rootPath, esp)
	if err != nil {
		return fmt.Errorf("installing grub config: %w", err)
	}

	entries, err := g.installKernelInitrd(rootPath, snapshotID, kernelCmdline, esp)
	if err != nil {
		return fmt.Errorf("installing kernel+initrd: %w", err)
	}

	err = g.updateBootEntries(rootPath, esp, entries...)
	if err != nil {
		return fmt.Errorf("updating boot entries: %w", err)
	}

	return nil
}

// installElementalEFI installs the efi applications (shim, MokManager, grub.efi) and grub.cfg into the ESP.
func (g *Grub) installElementalEFI(rootPath string, esp *deployment.Partition) error {
	g.s.Logger().Info("Installing EFI applications")

	for _, efiEntry := range []string{"BOOT", "ELEMENTAL"} {
		targetDir := filepath.Join(rootPath, esp.MountPoint, "EFI", efiEntry)
		err := vfs.MkdirAll(g.s.FS(), targetDir, vfs.DirPerm)
		if err != nil {
			return fmt.Errorf("creating dir '%s': %w", targetDir, err)
		}

		srcDir := filepath.Join(rootPath, "usr", "share", "efi", grubArch(g.s.Platform().Arch))
		for _, name := range []string{"grub.efi", "MokManager.efi"} {
			src := filepath.Join(srcDir, name)
			target := filepath.Join(targetDir, name)
			err = vfs.CopyFile(g.s.FS(), src, target)
			if err != nil {
				return fmt.Errorf("copying file '%s': %w", src, err)
			}
		}

		src := filepath.Join(srcDir, "shim.efi")
		target := filepath.Join(targetDir, defaultEfiBootFileName(g.s.Platform()))
		err = vfs.CopyFile(g.s.FS(), src, target)
		if err != nil {
			return fmt.Errorf("copying file '%s': %w", src, err)
		}

		// Write grub.cfg
		err = g.s.FS().WriteFile(filepath.Join(targetDir, "grub.cfg"), grubCfg, 0511)
		if err != nil {
			g.s.Logger().Warn("Failed installing grub.cfg: %s, skipping", err.Error())
		}
	}

	return nil
}

func grubArch(arch string) string {
	switch arch {
	case platform.ArchArm64:
		return platform.ArchAarch64
	default:
		return arch
	}
}

// defaultEfiBootFileName returns the default efi application name for the provided platform:
// * x86_64: bootx64.efi
// * aarch64: bootaa64.efi
// * riscv64: bootriscv64.efi
// defaults to x86_64.
func defaultEfiBootFileName(p *platform.Platform) string {
	switch p.Arch {
	case platform.ArchAarch64:
		return "bootaa64.efi"
	case platform.ArchArm64:
		return "bootaa64.efi"
	case platform.ArchRiscv64:
		return "bootriscv64.efi"
	default:
		return "bootx64.efi"
	}
}

// installGrub installs grub themes and configs to $ESP/grub2
func (g *Grub) installGrub(rootPath string, esp *deployment.Partition) error {
	g.s.Logger().Info("Syncing grub2 directory to ESP...")

	// Since we are copying to a vfat filesystem we have to skip symlinks.
	r := rsync.NewRsync(g.s, rsync.WithFlags("--archive", "--recursive", "--no-links"))

	err := r.SyncData(filepath.Join(rootPath, "/usr/share/grub2"), filepath.Join(rootPath, esp.MountPoint, "grub2"))
	if err != nil {
		return fmt.Errorf("syncing grub files: %w", err)
	}

	return nil
}

// installKernelInitrd copies the kernel to the ESP and generates an initrd using dracut.
func (g *Grub) installKernelInitrd(rootPath, snapshotID, kernelCmdline string, esp *deployment.Partition) ([]grubBootEntry, error) {
	g.s.Logger().Info("Installing kernel/initrd")

	osVars, err := vfs.LoadEnvFile(g.s.FS(), filepath.Join(rootPath, OsReleasePath))
	if err != nil {
		return nil, fmt.Errorf("loading %s vars: %w", OsReleasePath, err)
	}

	var (
		osID string
		ok   bool
	)
	if osID, ok = osVars["ID"]; !ok {
		return nil, fmt.Errorf("/etc/os-release ID not set")
	}

	kernel, kernelVersion, err := vfs.FindKernel(g.s.FS(), rootPath)
	if err != nil {
		return nil, fmt.Errorf("finding kernel: %w", err)
	}

	targetDir := filepath.Join(rootPath, esp.MountPoint, osID, kernelVersion)
	err = vfs.MkdirAll(g.s.FS(), targetDir, vfs.DirPerm)
	if err != nil {
		return nil, fmt.Errorf("creating kernel dir '%s': %w", targetDir, err)
	}

	err = vfs.CopyFile(g.s.FS(), kernel, targetDir)
	if err != nil {
		return nil, fmt.Errorf("copying kernel '%s': %w", kernel, err)
	}

	initrdPath := filepath.Join(filepath.Dir(kernel), Initrd)
	if exists, _ := vfs.Exists(g.s.FS(), initrdPath); !exists {
		return nil, fmt.Errorf("initrd not found")
	}

	err = vfs.CopyFile(g.s.FS(), initrdPath, targetDir)
	if err != nil {
		return nil, fmt.Errorf("copying initrd '%s': %w", initrdPath, err)
	}

	displayName, ok := osVars["PRETTY_NAME"]
	if !ok {
		displayName, ok = osVars["VARIANT"]
		if !ok {
			displayName = osVars["NAME"]
		}
	}

	snapshotName := fmt.Sprintf("%s (%s)", displayName, snapshotID)

	return []grubBootEntry{
		{
			linux:       filepath.Join("/", osID, kernelVersion, filepath.Base(kernel)),
			initrd:      filepath.Join("/", osID, kernelVersion, Initrd),
			displayName: displayName,
			cmdline:     kernelCmdline,
			id:          DefaultBootID,
		},
		{
			linux:       filepath.Join("/", osID, kernelVersion, filepath.Base(kernel)),
			initrd:      filepath.Join("/", osID, kernelVersion, Initrd),
			displayName: snapshotName,
			id:          snapshotID,
			cmdline:     kernelCmdline,
		},
	}, nil
}

func (g *Grub) updateBootEntries(rootPath string, esp *deployment.Partition, newEntries ...grubBootEntry) error {
	grubEnvPath := filepath.Join(rootPath, esp.MountPoint, "grubenv")
	activeEntries := []string{}

	// Read current entries
	if ok, _ := vfs.Exists(g.s.FS(), grubEnvPath); ok {
		stdOut, err := g.s.Runner().Run("grub2-editenv", grubEnvPath, "list")
		if err != nil {
			return fmt.Errorf("reading current boot entries: %w", err)
		}

		g.s.Logger().Debug("grub2-editenv stdout: %s", string(stdOut))
		for line := range strings.SplitSeq(string(stdOut), "\n") {
			if after, found := strings.CutPrefix(line, "entries="+DefaultBootID); found {
				activeEntries = append(activeEntries, strings.Fields(after)...)
			}
		}
	}

	err := vfs.MkdirAll(g.s.FS(), filepath.Join(rootPath, esp.MountPoint, "loader", "entries"), vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("creating loader dir: %w", err)
	}

	// create boot entries
	for _, entry := range newEntries {
		displayName := fmt.Sprintf("display_name=%s", entry.displayName)
		linux := fmt.Sprintf("linux=%s", entry.linux)
		initrd := fmt.Sprintf("initrd=%s", entry.initrd)
		cmdline := fmt.Sprintf("cmdline=%s", entry.cmdline)

		stdOut, err := g.s.Runner().Run("grub2-editenv", filepath.Join(rootPath, esp.MountPoint, "loader", "entries", entry.id), "set", displayName, linux, initrd, cmdline)
		g.s.Logger().Debug("grub2-editenv stdout: %s", string(stdOut))
		if err != nil {
			return err
		}

		if entry.id == DefaultBootID {
			continue
		}

		activeEntries = append(activeEntries, entry.id)
	}

	slices.Reverse(activeEntries)
	activeEntries = append([]string{DefaultBootID}, activeEntries...)

	// update entries variable in /boot/grubenv
	stdOut, err := g.s.Runner().Run("grub2-editenv", grubEnvPath, "set", fmt.Sprintf("entries=%s", strings.Join(activeEntries, " ")))
	g.s.Logger().Debug("grub2-editenv stdout: %s", string(stdOut))

	return err
}
