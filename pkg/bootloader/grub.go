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
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/joho/godotenv"

	"github.com/suse/elemental/v3/pkg/rsync"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/platform"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type Grub struct {
	s *sys.System
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
	g := &Grub{s}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

const (
	OsReleasePath  = "/etc/os-release"
	Initrd         = "initrd"
	DefaultBootID  = "active"
	RecoveryBootID = "recovery"

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

	entry, err := g.installKernelInitrd(rootPath, target, liveBootPath)
	if err != nil {
		return fmt.Errorf("installing kernel+initrd: %w", err)
	}
	entry.CmdLine = kernelCmdLine

	err = g.writeGrubConfig(filepath.Join(target, liveBootPath, "grub2"), grubLiveCfg, entry)
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
func (g *Grub) Install(rootPath, espDir, espLabel, entryID, kernelCmdline, recKernelCmdline string) error {
	err := g.installElementalEFI(rootPath, espDir, espLabel)
	if err != nil {
		return fmt.Errorf("installing elemental EFI apps: %w", err)
	}

	err = g.installGrub(rootPath, espDir)
	if err != nil {
		return fmt.Errorf("installing grub config: %w", err)
	}

	entry, err := g.installKernelInitrd(rootPath, espDir, "")
	if err != nil {
		return fmt.Errorf("installing kernel+initrd: %w", err)
	}

	displayName := entry.DisplayName
	entry.ID = entryID
	entry.CmdLine = kernelCmdline
	entries := []*grubBootEntry{&entry}

	// append default entry
	entry.DisplayName = fmt.Sprintf("%s (%s)", displayName, entryID)
	defaultEntry := grubBootEntry{
		Linux:       entry.Linux,
		Initrd:      entry.Initrd,
		DisplayName: displayName,
		CmdLine:     entry.CmdLine,
		ID:          DefaultBootID,
	}
	entries = append(entries, &defaultEntry)

	if recKernelCmdline != "" {
		recoveryEntry := grubBootEntry{
			Linux:       entry.Linux,
			Initrd:      entry.Initrd,
			DisplayName: fmt.Sprintf("%s (%s)", displayName, RecoveryBootID),
			CmdLine:     recKernelCmdline,
			ID:          RecoveryBootID,
		}
		entries = append(entries, &recoveryEntry)
	}

	err = g.updateBootEntries(espDir, entries...)
	if err != nil {
		return fmt.Errorf("updating boot entries: %w", err)
	}

	return nil
}

// Prune prunes old boot entries and artifacts not in the passed in keepSnapshotIDs.
func (g Grub) Prune(rootPath, espDir string, keepSnapshotIDs []int) (err error) {
	g.s.Logger().Info("Pruning old boot artifacts in %s", espDir)

	grubEnvPath := filepath.Join(espDir, grubEnvFile)
	grubEnv, err := g.readGrubEnv(grubEnvPath)
	if err != nil {
		return fmt.Errorf("reading grubenv: %w", err)
	}

	toDelete := []string{}
	activeEntries := []string{}
	hasRecovery := false
	hasDefault := false

	for _, entry := range strings.Fields(grubEnv["entries"]) {
		if entry == DefaultBootID {
			hasDefault = true
			continue
		}
		if entry == RecoveryBootID {
			hasRecovery = true
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

	entriesDir := filepath.Join(espDir, "loader", "entries")
	for _, entry := range toDelete {
		err = g.s.FS().Remove(filepath.Join(entriesDir, entry))
		if err != nil {
			g.s.Logger().Warn("failed removing '%s'", entry)
			return err
		}
	}

	slices.Reverse(activeEntries)
	if hasDefault {
		activeEntries = append([]string{DefaultBootID}, activeEntries...)
	}
	if hasRecovery {
		activeEntries = append(activeEntries, RecoveryBootID)
	}

	// update entries variable in /boot/grubenv
	stdOut, err := g.s.Runner().Run("grub2-editenv", grubEnvPath, "set", fmt.Sprintf("entries=%s", strings.Join(activeEntries, " ")))
	g.s.Logger().Debug("grub2-editenv stdout: %s", string(stdOut))

	if err != nil {
		return fmt.Errorf("failed saving %s: %w", grubEnvPath, err)
	}

	return g.pruneOldKernels(rootPath, espDir, activeEntries)
}

func (g Grub) pruneOldKernels(rootPath, espDir string, activeEntries []string) error {
	activeKernels := map[string]bool{}

	for _, entry := range activeEntries {
		grubEnv := filepath.Join(espDir, "loader", "entries", entry)
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
	kernelDir := filepath.Join(espDir, osID)
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
func (g *Grub) installElementalEFI(rootPath, espDir, espLabel string) error {
	g.s.Logger().Info("Installing EFI applications")

	for _, efiEntry := range []string{"BOOT", "ELEMENTAL"} {
		targetDir := filepath.Join(espDir, "EFI", efiEntry)
		err := g.installEFIEntry(rootPath, targetDir, grubCfg, map[string]string{"Label": espLabel})
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

	target := filepath.Join(espDir, "grub2")
	err := vfs.MkdirAll(g.s.FS(), target, vfs.DirPerm)
	if err != nil {
		return err
	}

	// Since we are copying to a vfat filesystem we have to skip symlinks.
	r := rsync.NewRsync(g.s, rsync.WithFlags("--archive", "--recursive", "--no-links"))

	err = r.SyncData(filepath.Join(rootPath, "/usr/share/grub2"), target)
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
func (g *Grub) installKernelInitrd(rootPath, espDir, subfolder string) (grubBootEntry, error) {
	g.s.Logger().Info("Installing kernel/initrd")
	entry := grubBootEntry{}

	osID, displayName, err := g.readIDAndName(rootPath)
	if err != nil {
		return entry, fmt.Errorf("failed parsing OS release: %w", err)
	}

	kernel, kernelVersion, err := vfs.FindKernel(g.s.FS(), rootPath)
	if err != nil {
		return entry, fmt.Errorf("finding kernel: %w", err)
	}

	targetDir := filepath.Join(espDir, subfolder, osID, kernelVersion)
	err = vfs.MkdirAll(g.s.FS(), targetDir, vfs.DirPerm)
	if err != nil {
		return entry, fmt.Errorf("creating kernel dir '%s': %w", targetDir, err)
	}

	err = vfs.CopyFile(g.s.FS(), kernel, targetDir)
	if err != nil {
		return entry, fmt.Errorf("copying kernel '%s': %w", kernel, err)
	}

	// Copy kernel .hmac in order to enable FIPS.
	kernelHmac, err := vfs.FindKernelHmac(g.s.FS(), kernel)
	if err != nil {
		return entry, fmt.Errorf("finding kernel hmac '%s': %w", kernel, err)
	}

	err = vfs.CopyFile(g.s.FS(), kernelHmac, targetDir)
	if err != nil {
		return entry, fmt.Errorf("copying kernel hmac '%s': %w", kernelHmac, err)
	}

	initrdPath := filepath.Join(filepath.Dir(kernel), Initrd)
	if exists, _ := vfs.Exists(g.s.FS(), initrdPath); !exists {
		return entry, fmt.Errorf("initrd not found")
	}

	err = vfs.CopyFile(g.s.FS(), initrdPath, targetDir)
	if err != nil {
		return entry, fmt.Errorf("copying initrd '%s': %w", initrdPath, err)
	}
	entry.Linux = filepath.Join("/", subfolder, osID, kernelVersion, filepath.Base(kernel))
	entry.Initrd = filepath.Join("/", subfolder, osID, kernelVersion, Initrd)
	entry.DisplayName = displayName

	return entry, nil
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

func (g *Grub) updateBootEntries(espDir string, newEntries ...*grubBootEntry) error {
	grubEnvPath := filepath.Join(espDir, grubEnvFile)
	activeEntries := []string{}
	hasRecovery := false
	hasDefault := false

	// Read current entries
	if ok, _ := vfs.Exists(g.s.FS(), grubEnvPath); ok {
		grubEnv, err := g.readGrubEnv(grubEnvPath)
		if err != nil {
			return fmt.Errorf("loading grubenv '%s': %w", grubEnvPath, err)
		}

		for _, entry := range strings.Fields(grubEnv["entries"]) {
			if entry == DefaultBootID {
				hasDefault = true
				continue
			}
			if entry == RecoveryBootID {
				hasRecovery = true
				continue
			}

			activeEntries = append(activeEntries, entry)
		}
	}

	err := vfs.MkdirAll(g.s.FS(), filepath.Join(espDir, "loader", "entries"), vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("creating loader dir: %w", err)
	}

	// create boot entries
	for _, entry := range newEntries {
		// do not update recovery entry if already exists
		if entry.ID == RecoveryBootID && hasRecovery {
			continue
		}

		err = g.writeBootEntry(espDir, entry)
		if err != nil {
			return err
		}

		if entry.ID == DefaultBootID {
			hasDefault = true
			continue
		}
		activeEntries = append(activeEntries, entry.ID)
	}

	slices.Reverse(activeEntries)
	if hasDefault {
		activeEntries = append([]string{DefaultBootID}, activeEntries...)
	}
	if hasRecovery {
		activeEntries = append(activeEntries, RecoveryBootID)
	}

	// update entries variable in /boot/grubenv
	stdOut, err := g.s.Runner().Run("grub2-editenv", grubEnvPath, "set", fmt.Sprintf("entries=%s", strings.Join(activeEntries, " ")))
	g.s.Logger().Debug("grub2-editenv stdout: %s", string(stdOut))

	return err
}

func (g Grub) writeBootEntry(espDir string, entry *grubBootEntry) error {
	displayName := fmt.Sprintf("display_name=%s", entry.DisplayName)
	linux := fmt.Sprintf("linux=%s", entry.Linux)
	initrd := fmt.Sprintf("initrd=%s", entry.Initrd)
	cmdline := fmt.Sprintf("cmdline=%s", entry.CmdLine)

	stdOut, err := g.s.Runner().Run("grub2-editenv", filepath.Join(espDir, "loader", "entries", entry.ID), "set", displayName, linux, initrd, cmdline)
	g.s.Logger().Debug("grub2-editenv stdout: %s", string(stdOut))
	if err != nil {
		return err
	}
	return nil
}
