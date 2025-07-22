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

package installermedia

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/diskrepart"
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
	isoBootCatalog = "boot.catalog"
	isoMountPoint  = "/run/initramfs/live"
	installerCfg   = "setup.sh"

	SquashfsPath = isoMountPoint + "/" + liveDir + "/" + squashfsImg
)

type Option func(*ISO)

type ISO struct {
	SourceOS      *deployment.ImageSource
	OverlayTree   *deployment.ImageSource
	CfgScript     string
	Name          string
	OutputDir     string
	Label         string
	KernelCmdLine string

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

	rootDir := filepath.Join(tempDir, "rootfs")
	err = vfs.MkdirAll(i.s.FS(), rootDir, vfs.DirPerm)
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

	err = i.prepareRoot(rootDir)
	if err != nil {
		return fmt.Errorf("failed preparing root: %w", err)
	}

	err = i.prepareISO(rootDir, isoDir, d)
	if err != nil {
		return fmt.Errorf("failed preparing iso directory tree: %w", err)
	}

	err = i.prepareEFI(isoDir, efiDir)
	if err != nil {
		return fmt.Errorf("failed preparing efi partition: %w", err)
	}

	efiImg := filepath.Join(tempDir, filepath.Base(efiDir)+".img")
	err = diskrepart.CreatePreloadedFileSystemImage(i.s, efiDir, efiImg, "EFI", 1, deployment.VFat)
	if err != nil {
		return fmt.Errorf("failed creating EFI image for the installer image: %w", err)
	}

	err = i.burnISO(isoDir, i.outputFile, efiImg)
	if err != nil {
		return fmt.Errorf("failed creating live iso image: %w", err)
	}

	return nil
}

// sanitize checks the current public attributes of the ISO object
// and checks they good enough to proceed with an ISO build.
func (i *ISO) sanitize() error {
	if i.SourceOS == nil || i.SourceOS.IsEmpty() {
		return fmt.Errorf("undefined OS image to build the installer from")
	}

	if i.Label == "" {
		return fmt.Errorf("undefined label for the installer filesystem")
	}

	if i.KernelCmdLine == "" {
		i.KernelCmdLine = fmt.Sprintf("root=live:CDLABEL=%s rd.live.overlay.overlayfs=1", i.Label)
	}

	if i.OutputDir == "" {
		return fmt.Errorf("undefined output directory")
	}

	if i.Name == "" {
		return fmt.Errorf("undefined name of the installer media")
	}

	i.outputFile = filepath.Join(i.OutputDir, fmt.Sprintf("%s.iso", i.Name))
	if ok, _ := vfs.Exists(i.s.FS(), i.outputFile); ok {
		return fmt.Errorf("target output file %s is an already existing file", i.outputFile)
	}

	return nil
}

// prepareRoot arranges the root directory tree that will be used to build the ISO's
// squashfs image. It essentially extracts OS OCI images to the given location.
func (i ISO) prepareRoot(rootDir string) error {
	i.s.Logger().Info("Extracting OS %s", i.SourceOS.String())

	unpacker, err := unpack.NewUnpacker(i.s, i.SourceOS, i.unpackOpts...)
	if err != nil {
		return fmt.Errorf("could not initate OS unpacker: %w", err)
	}
	digest, err := unpacker.Unpack(i.ctx, rootDir)
	if err != nil {
		return fmt.Errorf("OS unpack failed: %w", err)
	}
	i.SourceOS.SetDigest(digest)

	// Store the source image reference and digest as part of the ISO
	d := &deployment.Deployment{
		SourceOS: i.SourceOS,
	}
	err = d.WriteDeploymentFile(i.s, rootDir)
	if err != nil {
		return fmt.Errorf("OS source write failed: %w", err)
	}

	err = selinux.Relabel(i.ctx, i.s, rootDir)
	if err != nil {
		return fmt.Errorf("SELinux labelling failed: %w", err)
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
func (i ISO) prepareISO(rootDir, isoDir string, d *deployment.Deployment) error {
	i.s.Logger().Info("Preparing ISO contents at %s", isoDir)

	imgDir := filepath.Join(isoDir, liveDir)
	err := vfs.MkdirAll(i.s.FS(), imgDir, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed preparing ISO, could not create %s: %w", imgDir, err)
	}

	squashImg := filepath.Join(imgDir, squashfsImg)
	err = diskrepart.CreateSquashFS(i.ctx, i.s, rootDir, squashImg, diskrepart.DefaultSquashfsCompressionOptions())
	if err != nil {
		return fmt.Errorf("failed creating image (%s) for live ISO: %w", squashImg, err)
	}

	installPath := filepath.Join(isoDir, installDir)
	err = vfs.MkdirAll(i.s.FS(), installPath, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed preparing ISO, could not create %s: %w", installPath, err)
	}

	bootPath := filepath.Join(isoDir, "boot")
	err = vfs.MkdirAll(i.s.FS(), bootPath, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed preparing ISO, could not create %s: %w", bootPath, err)
	}

	err = i.bl.InstallLive(rootDir, isoDir, i.KernelCmdLine)
	if err != nil {
		return fmt.Errorf("failed installing bootloader in ISO directory tree: %w", err)
	}

	if i.CfgScript != "" {
		err = vfs.CopyFile(i.s.FS(), i.CfgScript, filepath.Join(imgDir, installerCfg))
		if err != nil {
			return fmt.Errorf("failed copying %s to image directory: %w", i.CfgScript, err)
		}
	}

	if i.OverlayTree != nil {
		unpacker, err := unpack.NewUnpacker(
			i.s, i.OverlayTree,
			append(i.unpackOpts, unpack.WithRsyncFlags(rsync.OverlayTreeSyncFlags()...))...,
		)
		if err != nil {
			return fmt.Errorf("could not initate overlay unpacker: %w", err)
		}
		_, err = unpacker.Unpack(i.ctx, isoDir)
		if err != nil {
			return fmt.Errorf("overlay unpack failed: %w", err)
		}
	}

	err = i.addInstallationAssets(installPath, d)
	if err != nil {
		return fmt.Errorf("failed adding installation assets and configuration: %w", err)
	}

	return nil
}

// addInstallationAssets adds to the ISO directory three the configuration and files required for
// the installation from the current media
func (i ISO) addInstallationAssets(targetDir string, d *deployment.Deployment) error {
	var err error

	if d.CfgScript != "" {
		err = vfs.CopyFile(i.s.FS(), d.CfgScript, filepath.Join(targetDir, filepath.Base(d.CfgScript)))
		if err != nil {
			return fmt.Errorf("failed copying %s to install directory: %w", d.CfgScript, err)
		}
		d.CfgScript = filepath.Join(isoMountPoint, installDir, filepath.Base(d.CfgScript))
	}

	if d.OverlayTree != nil {
		overlayPath := filepath.Join(targetDir, overlayDir)
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
			d.OverlayTree = deployment.NewDirSrc(filepath.Join(isoMountPoint, installDir, overlayDir))
		case d.OverlayTree.IsRaw() || d.OverlayTree.IsTar():
			overlayFile := filepath.Join(overlayPath, filepath.Base(d.OverlayTree.URI()))
			err = vfs.CopyFile(i.s.FS(), d.OverlayTree.URI(), overlayFile)
			if err != nil {
				return fmt.Errorf("failed adding overlay image to ISO directory tree: %w", err)
			}
			path := filepath.Join(isoMountPoint, installDir, overlayDir, filepath.Base(d.OverlayTree.URI()))
			if d.OverlayTree.IsTar() {
				d.OverlayTree = deployment.NewTarSrc(path)
			} else {
				d.OverlayTree = deployment.NewRawSrc(path)
			}
		default:
		}
	}

	installFile := filepath.Join(targetDir, "install.yaml")
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
	cmd := "xorriso"
	args := []string{
		"-volid", "LIVE", "-padding", "0",
		"-outdev", output, "-map", isoDir, "/", "-chmod", "0755", "--",
	}
	args = append(args, xorrisoBootloaderArgs(efiImg)...)

	_, err := i.s.Runner().RunContext(i.ctx, cmd, args...)
	if err != nil {
		return fmt.Errorf("failed creating the installer ISO image: %w", err)
	}

	checksum, err := calcFileChecksum(i.s.FS(), output)
	if err != nil {
		return fmt.Errorf("could not comput ISO's checksum: %w", err)
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
