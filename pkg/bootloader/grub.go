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
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/joho/godotenv"

	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/block/lsblk"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/rsync"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/platform"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type Grub struct {
	s      *sys.System
	clean  *cleanstack.CleanStack
	device block.Device
}

type grubBootEntry struct {
	Linux       string
	Initrd      string
	CmdLine     string
	DisplayName string
	ID          string
}

type Option func(*Grub)

func NewGrub(s *sys.System, opts ...Option) *Grub {
	g := &Grub{s, cleanstack.NewCleanStack(), lsblk.NewLsDevice(s)}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

func WithDevice(device block.Device) Option {
	return func(g *Grub) {
		g.device = device
	}
}

const (
	OsReleasePath = "/etc/os-release"
	Initrd        = "initrd"
	DefaultBootID = "active"

	liveBootPath = "/boot"
	grubEnvFile  = "grubenv"
)

//go:embed grubtemplates/grub.cfg
var grubCfg []byte

//go:embed grubtemplates/grub_live_efi.cfg
var grubLiveEFICfg []byte

//go:embed grubtemplates/grub_live.cfg
var grubLiveCfg []byte

// InstallLive installs the live bootloader to the specified target.
func (g *Grub) InstallLive(rootPath, target, kernelCmdLine string) error {
	g.s.Logger().Info("Preparing GRUB bootloader for live media")

	err := g.installGrub(rootPath, filepath.Join(target, liveBootPath))
	if err != nil {
		return fmt.Errorf("installing grub config: %w", err)
	}

	entries, err := g.installKernelInitrd(rootPath, target, liveBootPath, "", kernelCmdLine)
	if err != nil {
		return fmt.Errorf("installing kernel+initrd: %w", err)
	}

	err = g.writeGrubConfig(filepath.Join(target, liveBootPath, "grub2"), grubLiveCfg, entries[0])
	if err != nil {
		return fmt.Errorf("failed writing grub config file: %w", err)
	}

	// update cmdline variable in /boot/grubenv
	grubEnvPath := filepath.Join(target, liveBootPath, grubEnvFile)
	_, err = g.s.Runner().Run("grub2-editenv", grubEnvPath, "set", fmt.Sprintf("cmdline=%s", kernelCmdLine))
	if err != nil {
		return fmt.Errorf("failed setting kernel command line for grub: %w", err)
	}

	randomID, err := g.generateIDFile(filepath.Join(target, liveBootPath))
	if err != nil {
		return fmt.Errorf("failed creating identifier file for the bootloader: %w", err)
	}

	efiEntryDir := filepath.Join(target, "EFI", "BOOT")
	data := map[string]string{"IDFile": filepath.Join(liveBootPath, randomID)}
	err = g.installEFIEntry(rootPath, efiEntryDir, grubLiveEFICfg, data)
	if err != nil {
		return fmt.Errorf("installing elemental EFI apps: %w", err)
	}

	return nil
}

// Install installs the bootloader to the specified root.
func (g *Grub) Install(rootPath, snapshotID, kernelCmdline string, d *deployment.Deployment) error {
	esp := d.GetEfiPartition()
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

	err = g.installGrub(rootPath, filepath.Join(rootPath, esp.MountPoint))
	if err != nil {
		return fmt.Errorf("installing grub config: %w", err)
	}

	entries, err := g.installKernelInitrd(rootPath, filepath.Join(rootPath, esp.MountPoint), "", snapshotID, kernelCmdline)
	if err != nil {
		return fmt.Errorf("installing kernel+initrd: %w", err)
	}

	err = g.updateBootEntries(rootPath, esp, entries...)
	if err != nil {
		return fmt.Errorf("updating boot entries: %w", err)
	}

	return nil
}

// Prune prunes old boot entries and artifacts not in the passed in keepSnapshotIDs.
func (g Grub) Prune(rootPath string, keepSnapshotIDs []int, d *deployment.Deployment) (err error) {
	esp := d.GetEfiPartition()
	if esp == nil {
		return fmt.Errorf("ESP not found")
	}

	g.s.Logger().Info("Pruning old boot artifacts in %s", esp.Label)

	bootDir := filepath.Join(rootPath, esp.MountPoint)
	err = vfs.MkdirAll(g.s.FS(), bootDir, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating target directory %s: %w", bootDir, err)
	}

	efiDisk := d.GetEfiDisk()
	if efiDisk == nil {
		return fmt.Errorf("EFI disk not found")
	}

	hwPartitions, err := g.device.GetDevicePartitions(efiDisk.Device)
	if err != nil {
		return fmt.Errorf("listing host partitions: %w", err)
	}

	hwEfiPart := hwPartitions.GetByLabel(esp.Label)
	if hwEfiPart == nil {
		return fmt.Errorf("unknown efi partition")
	}

	err = g.s.Mounter().Mount(hwEfiPart.Path, bootDir, esp.FileSystem.String(), esp.MountOpts)
	if err != nil {
		return fmt.Errorf("failed mounting ESP to %s: %w", bootDir, err)
	}
	g.clean.Push(func() error { return g.s.Mounter().Unmount(bootDir) })

	defer func() {
		err = g.clean.Cleanup(err)
		if err != nil {
			g.s.Logger().Error("Failed cleaning up after bootloader pruning: %s", err.Error())
		}
	}()

	grubEnvPath := filepath.Join(bootDir, grubEnvFile)
	grubEnv, err := g.readGrubEnv(grubEnvPath)
	if err != nil {
		return fmt.Errorf("reading grubenv: %w", err)
	}

	toDelete := []string{}
	activeEntries := []string{}

	for _, entry := range strings.Fields(grubEnv["entries"]) {
		if entry == DefaultBootID {
			continue
		}

		snapshotID, err := strconv.Atoi(entry)
		if err != nil {
			g.s.Logger().Warn("Failed parsing snapshot ID '%s': %s", entry, err.Error())
			continue
		}

		if slices.Contains(keepSnapshotIDs, snapshotID) {
			activeEntries = append(activeEntries, entry)
			continue
		}

		toDelete = append(toDelete, entry)
	}

	entriesDir := filepath.Join(rootPath, esp.MountPoint, "loader", "entries")
	for _, entry := range toDelete {
		err = g.s.FS().Remove(filepath.Join(entriesDir, entry))
		if err != nil {
			g.s.Logger().Warn("failed removing '%s'", entry)
			return err
		}
	}

	slices.Reverse(activeEntries)
	activeEntries = append([]string{DefaultBootID}, activeEntries...)

	// update entries variable in /boot/grubenv
	stdOut, err := g.s.Runner().Run("grub2-editenv", grubEnvPath, "set", fmt.Sprintf("entries=%s", strings.Join(activeEntries, " ")))
	g.s.Logger().Debug("grub2-editenv stdout: %s", string(stdOut))

	if err != nil {
		return fmt.Errorf("failed saving %s: %w", grubEnvPath, err)
	}

	return g.pruneOldKernels(rootPath, esp, activeEntries)
}

func (g Grub) pruneOldKernels(rootPath string, esp *deployment.Partition, activeEntries []string) error {
	activeKernels := map[string]bool{}

	for _, entry := range activeEntries {
		grubEnv := filepath.Join(rootPath, esp.MountPoint, "loader", "entries", entry)
		vars, err := g.readGrubEnv(grubEnv)
		if err != nil {
			return fmt.Errorf("failed reading grubenv '%s': %w", grubEnv, err)
		}

		linux := vars["linux"]
		linuxDir, _ := filepath.Split(linux)
		version := filepath.Base(linuxDir)

		activeKernels[version] = true
	}

	osVars, err := vfs.LoadEnvFile(g.s.FS(), filepath.Join(rootPath, OsReleasePath))
	if err != nil {
		return fmt.Errorf("loading %s vars: %w", OsReleasePath, err)
	}

	var (
		ok   bool
		osID string
	)
	if osID, ok = osVars["ID"]; !ok {
		return fmt.Errorf("%s ID not set", OsReleasePath)
	}

	// look for older kernels
	kernelDir := filepath.Join(rootPath, esp.MountPoint, osID)
	kernelDirs, err := g.s.FS().ReadDir(kernelDir)
	if err != nil {
		return fmt.Errorf("reading sub-directories: %w", err)
	}

	for _, dirEntry := range kernelDirs {
		if !dirEntry.IsDir() {
			continue
		}

		if _, ok := activeKernels[dirEntry.Name()]; !ok {
			path := filepath.Join(kernelDir, dirEntry.Name())
			err := g.s.FS().RemoveAll(path)
			if err != nil {
				return fmt.Errorf("failed removing old kernel '%s': %w", path, err)
			}
		}
	}

	return nil
}

func (g Grub) generateIDFile(targetDir string) (string, error) {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed generating random boot identifier: %w", err)
	}
	randomID := hex.EncodeToString(bytes)

	idFile := filepath.Join(targetDir, randomID)
	err := g.s.FS().WriteFile(idFile, []byte(randomID), vfs.FilePerm)
	if err != nil {
		return "", fmt.Errorf("failed writing file '%s': %w", idFile, err)
	}
	return randomID, nil
}

func (g Grub) writeGrubConfig(targetDir string, cfgTemplate []byte, data any) error {
	err := vfs.MkdirAll(g.s.FS(), targetDir, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating grub target directory %s: %w", targetDir, err)
	}

	gCfg := filepath.Join(targetDir, "grub.cfg")
	f, err := g.s.FS().Create(gCfg)
	if err != nil {
		return fmt.Errorf("failed creating bootloader config file %s: %w", gCfg, err)
	}

	gcfg := template.New("grub")
	gcfg = template.Must(gcfg.Parse(string(cfgTemplate)))
	err = gcfg.Execute(f, data)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("failed rendering bootloader config file: %w", err)
	}

	err = f.Close()
	if err != nil {
		return fmt.Errorf("falied closing bootloader config file %s: %w", gCfg, err)
	}
	return nil
}

// installElementalEFI installs the efi applications (shim, MokManager, grub.efi) and grub.cfg into the ESP.
func (g *Grub) installElementalEFI(rootPath string, esp *deployment.Partition) error {
	g.s.Logger().Info("Installing EFI applications")

	for _, efiEntry := range []string{"BOOT", "ELEMENTAL"} {
		targetDir := filepath.Join(rootPath, esp.MountPoint, "EFI", efiEntry)
		err := g.installEFIEntry(rootPath, targetDir, grubCfg, map[string]string{"Label": esp.Label})
		if err != nil {
			return fmt.Errorf("failed setting '%s' EFI entry: %w", efiEntry, err)
		}
	}

	return nil
}

// installEFIEntry installs the efi applications (shim, MokManager, grub.efi) and grub.cfg to the given path
func (g *Grub) installEFIEntry(rootPath, targetDir string, grubTmpl []byte, data any) error {
	g.s.Logger().Info("Copying EFI artifacts at %s", targetDir)

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

	err = g.writeGrubConfig(targetDir, grubTmpl, data)
	if err != nil {
		return fmt.Errorf("failed writing EFI grub config file: %w", err)
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
func (g *Grub) installGrub(rootPath, espDir string) error {
	g.s.Logger().Info("Syncing grub2 directory to ESP...")

	// Since we are copying to a vfat filesystem we have to skip symlinks.
	r := rsync.NewRsync(g.s, rsync.WithFlags("--archive", "--recursive", "--no-links"))

	err := r.SyncData(filepath.Join(rootPath, "/usr/share/grub2"), filepath.Join(espDir, "grub2"))
	if err != nil {
		return fmt.Errorf("syncing grub files: %w", err)
	}

	return nil
}

// readIDAndName parses OS ID and OS name from os-relese file. Returns error of no OS ID is found.
func (g *Grub) readIDAndName(rootPath string) (osID string, displayName string, err error) {
	g.s.Logger().Info("Reading OS Relese")

	osVars, err := vfs.LoadEnvFile(g.s.FS(), filepath.Join(rootPath, OsReleasePath))
	if err != nil {
		return "", "", fmt.Errorf("loading %s vars: %w", OsReleasePath, err)
	}

	var ok bool
	if osID, ok = osVars["ID"]; !ok {
		return "", "", fmt.Errorf("%s ID not set", OsReleasePath)
	}

	displayName, ok = osVars["PRETTY_NAME"]
	if !ok {
		displayName, ok = osVars["VARIANT"]
		if !ok {
			displayName = osVars["NAME"]
		}
	}
	return osID, displayName, nil
}

// installKernelInitrd copies the kernel and initrd to the given ESP path.
//
// This function takes a rootPath to find and copy kernel and initrd from there. The espDir parameter
// is the target path where artifacts will be copied to. The subfolder specifies the location under espDir
// where artifacts will be copied (mostly used on live images to specify a "boot" folder). The snapshotID parameter
// is an identifier of the non default generated grubBootEntry. Finally kernelCmdline provides the kernel arguments
// for the generated grubBootEntries.
//
// Returns a grubBootEntry list with two items, one defined as a default entry and another one identified with the provided ID.
func (g *Grub) installKernelInitrd(rootPath, espDir, subfolder, snapshotID, kernelCmdline string) ([]grubBootEntry, error) {
	g.s.Logger().Info("Installing kernel/initrd")

	osID, displayName, err := g.readIDAndName(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed parsing OS release: %w", err)
	}

	kernel, kernelVersion, err := vfs.FindKernel(g.s.FS(), rootPath)
	if err != nil {
		return nil, fmt.Errorf("finding kernel: %w", err)
	}

	targetDir := filepath.Join(espDir, subfolder, osID, kernelVersion)
	err = vfs.MkdirAll(g.s.FS(), targetDir, vfs.DirPerm)
	if err != nil {
		return nil, fmt.Errorf("creating kernel dir '%s': %w", targetDir, err)
	}

	err = vfs.CopyFile(g.s.FS(), kernel, targetDir)
	if err != nil {
		return nil, fmt.Errorf("copying kernel '%s': %w", kernel, err)
	}

	// Copy kernel .hmac in order to enable FIPS.
	kernelHmac, err := vfs.FindKernelHmac(g.s.FS(), kernel)
	if err != nil {
		return nil, fmt.Errorf("finding kernel hmac '%s': %w", kernel, err)
	}

	err = vfs.CopyFile(g.s.FS(), kernelHmac, targetDir)
	if err != nil {
		return nil, fmt.Errorf("copying kernel hmac '%s': %w", kernelHmac, err)
	}

	initrdPath := filepath.Join(filepath.Dir(kernel), Initrd)
	if exists, _ := vfs.Exists(g.s.FS(), initrdPath); !exists {
		return nil, fmt.Errorf("initrd not found")
	}

	err = vfs.CopyFile(g.s.FS(), initrdPath, targetDir)
	if err != nil {
		return nil, fmt.Errorf("copying initrd '%s': %w", initrdPath, err)
	}

	snapshotName := fmt.Sprintf("%s (%s)", displayName, snapshotID)

	return []grubBootEntry{
		{
			Linux:       filepath.Join("/", subfolder, osID, kernelVersion, filepath.Base(kernel)),
			Initrd:      filepath.Join("/", subfolder, osID, kernelVersion, Initrd),
			DisplayName: displayName,
			CmdLine:     kernelCmdline,
			ID:          DefaultBootID,
		},
		{
			Linux:       filepath.Join("/", subfolder, osID, kernelVersion, filepath.Base(kernel)),
			Initrd:      filepath.Join("/", subfolder, osID, kernelVersion, Initrd),
			DisplayName: snapshotName,
			ID:          snapshotID,
			CmdLine:     kernelCmdline,
		},
	}, nil
}

func (g *Grub) readGrubEnv(path string) (map[string]string, error) {
	stdOut, err := g.s.Runner().Run("grub2-editenv", path, "list")
	if err != nil {
		return nil, fmt.Errorf("reading grubenv '%s': %w", path, err)
	}

	val, err := godotenv.UnmarshalBytes(stdOut)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling '%s': %w", path, err)
	}

	return val, nil
}

func (g *Grub) updateBootEntries(rootPath string, esp *deployment.Partition, newEntries ...grubBootEntry) error {
	grubEnvPath := filepath.Join(rootPath, esp.MountPoint, grubEnvFile)
	activeEntries := []string{}

	// Read current entries
	if ok, _ := vfs.Exists(g.s.FS(), grubEnvPath); ok {
		grubEnv, err := g.readGrubEnv(grubEnvPath)
		if err != nil {
			return fmt.Errorf("loading grubenv '%s': %w", grubEnvPath, err)
		}

		for _, entry := range strings.Fields(grubEnv["entries"]) {
			if entry == DefaultBootID {
				continue
			}

			activeEntries = append(activeEntries, entry)
		}
	}

	err := vfs.MkdirAll(g.s.FS(), filepath.Join(rootPath, esp.MountPoint, "loader", "entries"), vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("creating loader dir: %w", err)
	}

	// create boot entries
	for _, entry := range newEntries {
		displayName := fmt.Sprintf("display_name=%s", entry.DisplayName)
		linux := fmt.Sprintf("linux=%s", entry.Linux)
		initrd := fmt.Sprintf("initrd=%s", entry.Initrd)
		cmdline := fmt.Sprintf("cmdline=%s", entry.CmdLine)

		stdOut, err := g.s.Runner().Run("grub2-editenv", filepath.Join(rootPath, esp.MountPoint, "loader", "entries", entry.ID), "set", displayName, linux, initrd, cmdline)
		g.s.Logger().Debug("grub2-editenv stdout: %s", string(stdOut))
		if err != nil {
			return err
		}

		if entry.ID == DefaultBootID {
			continue
		}

		activeEntries = append(activeEntries, entry.ID)
	}

	slices.Reverse(activeEntries)
	activeEntries = append([]string{DefaultBootID}, activeEntries...)

	// update entries variable in /boot/grubenv
	stdOut, err := g.s.Runner().Run("grub2-editenv", grubEnvPath, "set", fmt.Sprintf("entries=%s", strings.Join(activeEntries, " ")))
	g.s.Logger().Debug("grub2-editenv stdout: %s", string(stdOut))

	return err
}
