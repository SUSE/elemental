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

package installer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/filesystem"
	"github.com/suse/elemental/v3/pkg/rsync"
	"github.com/suse/elemental/v3/pkg/selinux"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

const (
	liveDir        = "LiveOS"
	installDir     = "Install"
	overlayDir     = "Overlay"
	squashfsImg    = "squashfs.img"
	installCfg     = "install.yaml"
	isoBootCatalog = "boot.catalog"
	cfgScript      = "setup.sh"
	xorriso        = "xorriso"

	LiveMountPoint = "/run/initramfs/live"
	SquashfsPath   = LiveMountPoint + "/" + liveDir + "/" + squashfsImg
	InstallDesc    = LiveMountPoint + "/" + installDir + "/" + installCfg
	InstallScript  = LiveMountPoint + "/" + installDir + "/" + cfgScript
)

type Option func(*ISO)

type ISO struct {
	Name      string
	OutputDir string
	Label     string
	InputFile string

	s          *sys.System
	ctx        context.Context
	unpackOpts []unpack.Opt
	bl         bootloader.Bootloader
	outputFile string
}

// WithBootloader allows to create an ISO object with the given bootloader interface instance
func WithBootloader(bootloader bootloader.Bootloader) Option {
	return func(i *ISO) {
		i.bl = bootloader
	}
}

// WithUnpackOpts allows to create an ISO object with the given unpack package options
func WithUnpackOpts(opts ...unpack.Opt) Option {
	return func(i *ISO) {
		i.unpackOpts = opts
	}
}

// NewISO returns a new ISO object
func NewISO(ctx context.Context, s *sys.System, opts ...Option) *ISO {
	iso := &ISO{
		Name:       "installer",
		Label:      "LIVE",
		s:          s,
		ctx:        ctx,
		unpackOpts: []unpack.Opt{},
	}
	for _, o := range opts {
		o(iso)
	}
	if iso.bl == nil {
		iso.bl, _ = bootloader.New(bootloader.BootGrub, iso.s)
	}
	return iso
}

// Build creates a new ISO installer image with the given installation or deployment
// paramters
func (i ISO) Build(d *deployment.Deployment) (err error) {
	err = i.sanitize()
	if err != nil {
		return fmt.Errorf("cannot proceed with installer build due to inconsistent setup: %w", err)
	}

	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	err = vfs.MkdirAll(i.s.FS(), i.OutputDir, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating output directory: %w", err)
	}

	tempDir, err := vfs.TempDir(i.s.FS(), i.OutputDir, "elemental-installer")
	if err != nil {
		return fmt.Errorf("could note create working directory for installer ISO build: %w", err)
	}
	cleanup.Push(func() error { return i.s.FS().RemoveAll(tempDir) })

	rootfs := filepath.Join(tempDir, "rootfs")
	err = vfs.MkdirAll(i.s.FS(), rootfs, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating rootfs directory: %w", err)
	}

	efiDir := filepath.Join(tempDir, "efi")
	err = vfs.MkdirAll(i.s.FS(), efiDir, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating EFI directory: %w", err)
	}

	isoDir := filepath.Join(tempDir, "iso")
	err = vfs.MkdirAll(i.s.FS(), isoDir, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating ISO directory: %w", err)
	}

	err = i.prepareISO(isoDir, rootfs, d)
	if err != nil {
		return fmt.Errorf("failed preparing iso directory tree: %w", err)
	}

	err = i.prepareEFI(isoDir, efiDir)
	if err != nil {
		return fmt.Errorf("failed preparing efi partition: %w", err)
	}

	efiImg := filepath.Join(tempDir, filepath.Base(efiDir)+".img")
	err = filesystem.CreatePreloadedFileSystemImage(i.s, efiDir, efiImg, "EFI", 1, deployment.VFat)
	if err != nil {
		return fmt.Errorf("failed creating EFI image for the installer image: %w", err)
	}

	err = i.burnISO(isoDir, i.outputFile, efiImg)
	if err != nil {
		return fmt.Errorf("failed creating live iso image: %w", err)
	}

	return nil
}

// PrepareInstallerFS prepares the directory tree of the installer image, rootDir is the path
// of the directory tree root and workDir is the path to extract source image, typically a temporary
// directory entirely managed by the caller logic.
func (i *ISO) PrepareInstallerFS(rootDir, workDir string, d *deployment.Deployment) error {
	imgDir := filepath.Join(rootDir, liveDir)
	err := vfs.MkdirAll(i.s.FS(), imgDir, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed preparing ISO, could not create %s: %w", imgDir, err)
	}
	squashImg := filepath.Join(imgDir, squashfsImg)

	switch {
	case d.SourceOS.IsRaw():
		// We assume this is comming from a ready to be used installer media
		// no need to unpack and repack
		err = vfs.CopyFile(i.s.FS(), d.SourceOS.URI(), squashImg)
		if err != nil {
			return fmt.Errorf("failed copying OS image to installer root tree: %w", err)
		}
	default:
		err = i.prepareOSRoot(d.SourceOS, workDir)
		if err != nil {
			return fmt.Errorf("preparing unpack: %w", err)
		}
		err = filesystem.CreateSquashFS(i.ctx, i.s, workDir, squashImg, filesystem.DefaultSquashfsCompressionOptions())
		if err != nil {
			return fmt.Errorf("failed creating image (%s) for live ISO: %w", squashImg, err)
		}
	}
	// Store the deployment with the OS image we just created/copied
	d.SourceOS = deployment.NewRawSrc(SquashfsPath)

	if d.Installer.CfgScript != "" {
		err = vfs.CopyFile(i.s.FS(), d.Installer.CfgScript, filepath.Join(imgDir, cfgScript))
		if err != nil {
			return fmt.Errorf("failed copying %s to image directory: %w", d.Installer.CfgScript, err)
		}
	}

	if d.Installer.OverlayTree != nil {
		unpacker, err := unpack.NewUnpacker(
			i.s, d.Installer.OverlayTree,
			append(i.unpackOpts, unpack.WithRsyncFlags(rsync.OverlayTreeSyncFlags()...))...,
		)
		if err != nil {
			return fmt.Errorf("could not initate overlay unpacker: %w", err)
		}
		_, err = unpacker.Unpack(i.ctx, rootDir)
		if err != nil {
			return fmt.Errorf("overlay unpack failed: %w", err)
		}
	}

	err = i.addInstallationAssets(rootDir, d)
	if err != nil {
		return fmt.Errorf("failed adding installation assets and configuration: %w", err)
	}

	return nil
}

// Customize repacks an existing installer with more artifacts.
func (i *ISO) Customize(d *deployment.Deployment) (err error) {
	err = i.sanitize()
	if err != nil {
		return fmt.Errorf("cannot proceed with customize due to inconsistent setup: %w", err)
	}

	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	tempDir, err := vfs.TempDir(i.s.FS(), "/tmp", "elemental-installer")
	if err != nil {
		return fmt.Errorf("could note create working directory for installer ISO build: %w", err)
	}
	cleanup.Push(func() error { return i.s.FS().RemoveAll(tempDir) })

	ovDir := filepath.Join(tempDir, "overlay")
	err = vfs.MkdirAll(i.s.FS(), ovDir, vfs.FilePerm)
	if err != nil {
		return fmt.Errorf("could note create working directory for installer ISO build: %w", err)
	}

	grubEnvPath := filepath.Join(tempDir, "grubenv")
	cmdline := strings.TrimSpace(fmt.Sprintf("%s %s", deployment.LiveKernelCmdline(i.Label), d.Installer.KernelCmdline))
	err = i.writeGrubEnv(grubEnvPath, map[string]string{"cmdline": cmdline})
	if err != nil {
		return fmt.Errorf("error writing %s: %s", grubEnvPath, err.Error())
	}

	m := map[string]string{
		grubEnvPath: "/boot/grubenv",
	}

	if d.Installer.CfgScript != "" {
		m[d.Installer.CfgScript] = filepath.Join("/", liveDir, cfgScript)
	}

	if d.Installer.OverlayTree != nil {
		unpacker, err := unpack.NewUnpacker(
			i.s, d.Installer.OverlayTree,
			append(i.unpackOpts, unpack.WithRsyncFlags(rsync.OverlayTreeSyncFlags()...))...,
		)
		if err != nil {
			return fmt.Errorf("could not initate overlay unpacker: %w", err)
		}
		_, err = unpacker.Unpack(i.ctx, ovDir)
		if err != nil {
			return fmt.Errorf("overlay unpack failed: %w", err)
		}

		m[ovDir] = "/"
	}

	assetsPath := filepath.Join(tempDir, "assets")

	err = vfs.MkdirAll(i.s.FS(), assetsPath, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating assets dir '%s': %w", assetsPath, err)
	}

	err = i.addInstallationAssets(assetsPath, d)
	if err != nil {
		return fmt.Errorf("failed adding installation assets and configuration: %w", err)
	}

	m[assetsPath] = "/Install"

	return i.mapFiles(i.InputFile, i.outputFile, m)
}

// sanitize checks the current public attributes of the ISO object
// and checks if they are good enough to proceed with an ISO build.
func (i *ISO) sanitize() error {
	if i.Label == "" {
		return fmt.Errorf("undefined label for the installer filesystem")
	}

	if i.OutputDir == "" {
		return fmt.Errorf("undefined output directory")
	}

	if i.Name == "" {
		return fmt.Errorf("undefined name of the installer media")
	}

	if i.InputFile != "" {
		if ok, _ := vfs.Exists(i.s.FS(), i.InputFile); !ok {
			return fmt.Errorf("target input file %s does not exist", i.InputFile)
		}
	}

	i.outputFile = filepath.Join(i.OutputDir, fmt.Sprintf("%s.iso", i.Name))
	if ok, _ := vfs.Exists(i.s.FS(), i.outputFile); ok {
		return fmt.Errorf("target output file %s is an already existing file", i.outputFile)
	}

	return nil
}

func (i ISO) mapFiles(inputFile, outputFile string, fileMap map[string]string) error {
	args := []string{"-indev", inputFile, "-outdev", outputFile, "-boot_image", "any", "replay"}

	for f, m := range fileMap {
		args = append(args, "-map", f, m)
	}

	_, err := i.s.Runner().RunContext(i.ctx, xorriso, args...)
	if err != nil {
		return fmt.Errorf("failed creating the installer ISO image: %w", err)
	}

	return nil
}

func (i ISO) writeGrubEnv(file string, vars map[string]string) error {
	arr := make([]string, len(vars)+2)

	arr[0] = file
	arr[1] = "set"

	j := 2
	for k, v := range vars {
		arr[j] = fmt.Sprintf("%s=%s", k, v)

		j++
	}

	_, err := i.s.Runner().Run("grub2-editenv", arr...)
	return err
}

// prepareOSRoot arranges the root directory tree that will be used to build the ISO's
// squashfs image. It essentially extracts OS OCI images to the given location.
func (i ISO) prepareOSRoot(sourceOS *deployment.ImageSource, rootDir string) error {
	i.s.Logger().Info("Extracting OS %s", sourceOS.String())

	unpacker, err := unpack.NewUnpacker(i.s, sourceOS, i.unpackOpts...)
	if err != nil {
		return fmt.Errorf("could not initate OS unpacker: %w", err)
	}
	digest, err := unpacker.Unpack(i.ctx, rootDir)
	if err != nil {
		return fmt.Errorf("OS unpack failed: %w", err)
	}
	sourceOS.SetDigest(digest)

	// Store the source image reference and digest as part of the ISO
	d := &deployment.Deployment{
		SourceOS: sourceOS,
	}
	err = d.WriteDeploymentFile(i.s, rootDir)
	if err != nil {
		return fmt.Errorf("OS source write failed: %w", err)
	}

	err = selinux.Relabel(i.ctx, i.s, rootDir)
	if err != nil {
		i.s.Logger().Warn("Error selinux relabelling: %s", err.Error())
	}
	return nil
}

// prepareEFI sets the root directory tree of the EFI partition
func (i ISO) prepareEFI(isoDir, efiDir string) error {
	i.s.Logger().Info("Preparing EFI partition at %s", efiDir)

	err := vfs.MkdirAll(i.s.FS(), filepath.Join(efiDir, "EFI"), vfs.FilePerm)
	if err != nil {
		return fmt.Errorf("failed creating EFI directory tree: %w", err)
	}
	r := rsync.NewRsync(
		i.s, rsync.WithFlags("--archive", "--recursive", "--no-links"),
		rsync.WithContext(i.ctx),
	)
	return r.SyncData(filepath.Join(isoDir, "EFI"), filepath.Join(efiDir, "EFI"))
}

// prepareISO sets the root directory three of the ISO filesystem
func (i ISO) prepareISO(isoDir, rootfs string, d *deployment.Deployment) error {
	i.s.Logger().Info("Preparing ISO contents at %s", isoDir)

	err := i.PrepareInstallerFS(isoDir, rootfs, d)
	if err != nil {
		return fmt.Errorf("failed to populate ISO directory tree: %w", err)
	}

	bootPath := filepath.Join(isoDir, "boot")
	err = vfs.MkdirAll(i.s.FS(), bootPath, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed preparing ISO, could not create %s: %w", bootPath, err)
	}

	cmdline := strings.TrimSpace(fmt.Sprintf("%s %s", deployment.LiveKernelCmdline(i.Label), d.Installer.KernelCmdline))
	err = i.bl.InstallLive(rootfs, isoDir, cmdline)
	if err != nil {
		return fmt.Errorf("failed installing bootloader in ISO directory tree: %w", err)
	}

	return nil
}

// addInstallationAssets adds to the ISO directory three the configuration and files required for
// the installation from the current media
func (i ISO) addInstallationAssets(root string, d *deployment.Deployment) error {
	var err error

	installPath := filepath.Join(root, installDir)
	err = vfs.MkdirAll(i.s.FS(), installPath, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed preparing ISO, could not create %s: %w", installPath, err)
	}

	if d.CfgScript != "" {
		err = vfs.CopyFile(i.s.FS(), d.CfgScript, filepath.Join(installPath, cfgScript))
		if err != nil {
			return fmt.Errorf("failed copying %s to install directory: %w", d.CfgScript, err)
		}
		d.CfgScript = InstallScript
	}

	if d.OverlayTree != nil {
		overlayPath := filepath.Join(installPath, overlayDir)
		err = vfs.MkdirAll(i.s.FS(), overlayPath, vfs.DirPerm)
		if err != nil {
			return fmt.Errorf("failed preparing ISO, could not create %s: %w", overlayPath, err)
		}

		switch {
		case d.OverlayTree.IsDir():
			r := rsync.NewRsync(i.s, rsync.WithFlags(rsync.OverlayTreeSyncFlags()...), rsync.WithContext(i.ctx))
			err = r.SyncData(d.OverlayTree.URI(), overlayPath)
			if err != nil {
				return fmt.Errorf("failed adding overlay tree to ISO directory tree: %w", err)
			}
			d.OverlayTree = deployment.NewDirSrc(filepath.Join(LiveMountPoint, installDir, overlayDir))
		case d.OverlayTree.IsRaw() || d.OverlayTree.IsTar():
			overlayFile := filepath.Join(overlayPath, filepath.Base(d.OverlayTree.URI()))
			err = vfs.CopyFile(i.s.FS(), d.OverlayTree.URI(), overlayFile)
			if err != nil {
				return fmt.Errorf("failed adding overlay image to ISO directory tree: %w", err)
			}
			path := filepath.Join(LiveMountPoint, installDir, overlayDir, filepath.Base(d.OverlayTree.URI()))
			if d.OverlayTree.IsTar() {
				d.OverlayTree = deployment.NewTarSrc(path)
			} else {
				d.OverlayTree = deployment.NewRawSrc(path)
			}
		default:
		}
	}

	installFile := filepath.Join(installPath, installCfg)
	dBytes, err := yaml.Marshal(d)
	if err != nil {
		return fmt.Errorf("marshalling deployment: %w", err)
	}

	err = i.s.FS().WriteFile(installFile, dBytes, 0444)
	if err != nil {
		return fmt.Errorf("writing deployment file '%s': %w", installFile, err)
	}

	return nil
}

// burnISO creates the ISO image from the prepared data
func (i ISO) burnISO(isoDir, output, efiImg string) error {
	args := []string{
		"-volid", "LIVE", "-padding", "0",
		"-outdev", output, "-map", isoDir, "/", "-chmod", "0755", "--",
	}
	args = append(args, xorrisoBootloaderArgs(efiImg)...)

	_, err := i.s.Runner().RunContext(i.ctx, xorriso, args...)
	if err != nil {
		return fmt.Errorf("failed creating the installer ISO image: %w", err)
	}

	checksum, err := calcFileChecksum(i.s.FS(), output)
	if err != nil {
		return fmt.Errorf("could not compute ISO's checksum: %w", err)
	}

	checksumFile := fmt.Sprintf("%s.sha256", output)
	err = i.s.FS().WriteFile(checksumFile, fmt.Appendf(nil, "%s %s\n", checksum, filepath.Base(output)), vfs.FilePerm)
	if err != nil {
		return fmt.Errorf("failed writing ISO's checksum file %s: %w", checksumFile, err)
	}

	return nil
}

// xorrisoBootloaderArgs returns a slice of flags for xorriso to defined a common bootloader parameters
func xorrisoBootloaderArgs(efiImg string) []string {
	args := []string{
		"-append_partition", "2", "0xef", efiImg,
		"-boot_image", "any", fmt.Sprintf("cat_path=%s", isoBootCatalog),
		"-boot_image", "any", "cat_hidden=on",
		"-boot_image", "any", "efi_path=--interval:appended_partition_2:all::",
		"-boot_image", "any", "platform_id=0xef",
		"-boot_image", "any", "appended_part_as=gpt",
		"-boot_image", "any", "partition_offset=16",
	}
	return args
}

// calcFileChecksum opens the given file and returns the sha256 checksum of it.
func calcFileChecksum(fs vfs.FS, fileName string) (string, error) {
	f, err := fs.Open(fileName)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("reading data for a sha256 checksum failed: %w", err)
	}

	err = f.Close()
	if err != nil {
		return "", fmt.Errorf("failed closing file %s after calculating checksum: %w", fileName, err)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
