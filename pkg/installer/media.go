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

package installer

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/filesystem"
	"github.com/suse/elemental/v3/pkg/repart"
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

	LiveMountPoint  = "/run/initramfs/live"
	SquashfsRelPath = liveDir + "/" + squashfsImg
	SquashfsPath    = LiveMountPoint + "/" + SquashfsRelPath
	InstallDesc     = LiveMountPoint + "/" + installDir + "/" + installCfg
	InstallScript   = LiveMountPoint + "/" + installDir + "/" + cfgScript
)

type MediaType int

const (
	ISO MediaType = iota + 1
	Disk
)

func (m MediaType) String() string {
	switch m {
	case ISO:
		return "iso"
	case Disk:
		return "raw"
	default:
		return "unknown"
	}
}

func StringToMediaType(mType string) (MediaType, error) {
	switch mType {
	case "raw":
		return Disk, nil
	case "iso":
		return ISO, nil
	default:
		return 0, fmt.Errorf("unsupported media type %s: %w", mType, errors.ErrUnsupported)
	}
}

type Option func(*Media)

type Media struct {
	Name      string
	OutputDir string
	Label     string
	InputFile string

	mType       MediaType
	s           *sys.System
	ctx         context.Context
	unpackOpts  []unpack.Opt
	bl          bootloader.Bootloader
	outputFile  string
	rawDiskSize deployment.MiB
}

// WithBootloader allows to create an ISO object with the given bootloader interface instance
func WithBootloader(bootloader bootloader.Bootloader) Option {
	return func(i *Media) {
		i.bl = bootloader
	}
}

// WithUnpackOpts allows to create an ISO object with the given unpack package options
func WithUnpackOpts(opts ...unpack.Opt) Option {
	return func(i *Media) {
		i.unpackOpts = opts
	}
}

func WithRawDiskSize(size deployment.MiB) Option {
	return func(i *Media) {
		i.rawDiskSize = size
	}
}

func WithOutputFile(outputFile string) Option {
	return func(i *Media) {
		i.outputFile = outputFile
	}
}

func NewMedia(ctx context.Context, s *sys.System, mType MediaType, opts ...Option) *Media {
	media := &Media{
		Name:       "installer",
		OutputDir:  "build-installer",
		s:          s,
		ctx:        ctx,
		unpackOpts: []unpack.Opt{},
		mType:      mType,
	}
	for _, o := range opts {
		o(media)
	}
	if media.bl == nil {
		media.bl, _ = bootloader.New(bootloader.BootGrub, media.s)
	}
	if media.mType == ISO {
		media.Label = "LIVE"
	}
	return media
}

// extractISO extracts the given source path (relative to iso root) to the destination path
func extractISO(s *sys.System, iso, srcPath, destPath string) error {
	args := []string{
		"-osirrox", "on:auto_chmod_on", "-overwrite", "nondir", "-indev", iso, "-extract", srcPath, destPath,
	}
	out, err := s.Runner().Run("xorriso", args...)
	s.Logger().Debug("xorriso output: %s", string(out))
	if err != nil {
		return fmt.Errorf("failed extracting '%s' to '%s' from iso '%s': %w", srcPath, destPath, iso, err)
	}
	return nil
}

// LoadISOInstallDesc extracts the install description file form the given ISO and parses it into a new deployment
func LoadISOInstallDesc(s *sys.System, tempDir, iso string) (*deployment.Deployment, error) {
	installDst := filepath.Join(tempDir, installCfg)
	installSrc := filepath.Join(installDir, installCfg)

	err := extractISO(s, iso, installSrc, installDst)
	if err != nil {
		return nil, fmt.Errorf("failed extracting install description: %w", err)
	}

	data, err := s.FS().ReadFile(installDst)
	if err != nil {
		return nil, fmt.Errorf("reading deployment file '%s': %w", installDst, err)
	}
	d := &deployment.Deployment{}
	err = yaml.Unmarshal(data, d)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling deployment file '%s': %w: %s", installDst, err, string(data))
	}

	return d, nil
}

// Build creates a new media installer image with the given installation and deployment
// parameters
func (i Media) Build(d *deployment.Deployment) (err error) {
	err = i.sanitize()
	if err != nil {
		return fmt.Errorf("cannot proceed with installer build due to inconsistent setup: %w", err)
	}

	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	tempDir, err := vfs.TempDir(i.s.FS(), i.OutputDir, "elemental-installer")
	if err != nil {
		return fmt.Errorf("could note create working directory for installer disk build: %w", err)
	}
	cleanup.Push(func() error { return i.s.FS().RemoveAll(tempDir) })

	osRoot := filepath.Join(tempDir, "osroot")
	err = vfs.MkdirAll(i.s.FS(), osRoot, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating rootfs directory: %w", err)
	}

	liveRoot := filepath.Join(tempDir, "liveroot")
	err = vfs.MkdirAll(i.s.FS(), liveRoot, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating ISO directory: %w", err)
	}

	err = i.PrepareInstallerFS(liveRoot, osRoot, d)
	if err != nil {
		return fmt.Errorf("failed to populate ISO directory tree: %w", err)
	}

	switch i.mType {
	case ISO:
		cmdline := fmt.Sprintf("%s %s", deployment.LiveKernelCmdline(i.Label), d.Installer.KernelCmdline)
		err = i.buildISO(tempDir, liveRoot, osRoot, cmdline)
	case Disk:
		err = i.buildDisk(tempDir, liveRoot, osRoot, d)
	default:
		return fmt.Errorf("unknown media type: %w", errors.ErrUnsupported)
	}
	if err != nil {
		return err
	}

	return i.writeChecksum()
}

// PrepareInstallerFS prepares the directory tree of the installer image, rootDir is the path
// of the directory tree root and workDir is the path to extract source image, typically a temporary
// directory entirely managed by the caller logic.
func (i *Media) PrepareInstallerFS(rootDir, workDir string, d *deployment.Deployment) error {
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
		_, err = unpacker.Unpack(i.ctx, rootDir, reservedPaths()...)
		if err != nil {
			return fmt.Errorf("overlay unpack failed: %w", err)
		}
	}

	err = i.addInstallationAssets(rootDir, d)
	if err != nil {
		return fmt.Errorf("failed adding installation assets and configuration: %w", err)
	}

	recPart := d.GetRecoveryPartition()
	if recPart != nil {
		size, err := vfs.DirSizeMB(i.s.FS(), rootDir)
		if err != nil {
			return fmt.Errorf("failed to compute recovery partition size: %w", err)
		}
		recSize := deployment.MiB((size/128)*128 + 256)
		if recPart.Size < recSize {
			i.s.Logger().Debug("Increasing recovery partition size to %dMiB", recSize)
			recPart.Size = recSize
		}
	}

	return i.writeInstallDescription(filepath.Join(rootDir, installDir), d)
}

// Customize repacks an existing installer with more artifacts.
func (i *Media) Customize(d *deployment.Deployment) (err error) {
	err = i.sanitize()
	if err != nil {
		return fmt.Errorf("cannot proceed with customize due to inconsistent setup: %w", err)
	}

	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	tempDir, err := vfs.TempDir(i.s.FS(), i.OutputDir, "elemental-installer")
	if err != nil {
		return fmt.Errorf("could note create working directory for installer ISO build: %w", err)
	}
	cleanup.Push(func() error { return i.s.FS().RemoveAll(tempDir) })

	installDesc, err := LoadISOInstallDesc(i.s, tempDir, i.InputFile)
	if err != nil {
		return fmt.Errorf("failed extracting install description from '%s': %w", i.InputFile, err)
	}

	m := map[string]string{}

	grubEnvPath := filepath.Join(tempDir, "grubenv")
	err = i.recreateGrubenv(grubEnvPath, d.Installer.KernelCmdline, installDesc)
	if err != nil {
		return fmt.Errorf("failed rewriting grubenv file: %w", err)
	}
	m[grubEnvPath] = "/boot/grubenv"

	if d.Installer.CfgScript != "" {
		m[d.Installer.CfgScript] = filepath.Join("/", liveDir, cfgScript)
	}

	var ovDir string
	if d.Installer.OverlayTree != nil {
		if d.Installer.OverlayTree.IsDir() {
			ovDir = d.Installer.OverlayTree.URI()
		} else {
			ovDir = filepath.Join(tempDir, "overlay")
			err = vfs.MkdirAll(i.s.FS(), ovDir, vfs.FilePerm)
			if err != nil {
				return fmt.Errorf("could note create working directory for installer ISO build: %w", err)
			}
			unpacker, err := unpack.NewUnpacker(
				i.s, d.Installer.OverlayTree,
				append(i.unpackOpts, unpack.WithRsyncFlags(rsync.OverlayTreeSyncFlags()...))...,
			)
			if err != nil {
				return fmt.Errorf("could not initate overlay unpacker: %w", err)
			}
			_, err = unpacker.Unpack(i.ctx, ovDir, reservedPaths()...)
			if err != nil {
				return fmt.Errorf("overlay unpack failed: %w", err)
			}
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
	m[assetsPath] = "/"

	err = deployment.Merge(installDesc, d)
	if err != nil {
		return fmt.Errorf("failed merging deployment description: %w", err)
	}

	err = i.increaseRecoverySize(m, installDesc)
	if err != nil {
		return fmt.Errorf("failed computing recovery size increasal: %w", err)
	}

	err = i.writeInstallDescription(filepath.Join(assetsPath, installDir), installDesc)
	if err != nil {
		return err
	}

	switch i.mType {
	case ISO:
		err = i.customizeISO(i.InputFile, i.outputFile, m)
	case Disk:
		err = i.customizeDisk(tempDir, installDesc, m)
	default:
		err = fmt.Errorf("unknown media type: %w", errors.ErrUnsupported)
	}
	if err != nil {
		return err
	}

	return i.writeChecksum()
}

// writeChecksum computes the checksum for the current media output file and writes
// the checksum file to the same output file path, but with the *.sha256 suffix
func (i Media) writeChecksum() error {
	checksum, err := calcFileChecksum(i.s.FS(), i.outputFile)
	if err != nil {
		return fmt.Errorf("could not compute image checksum: %w", err)
	}

	checksumFile := fmt.Sprintf("%s.sha256", i.outputFile)
	err = i.s.FS().WriteFile(checksumFile, fmt.Appendf(nil, "%s %s\n", checksum, filepath.Base(i.outputFile)), vfs.FilePerm)
	if err != nil {
		return fmt.Errorf("failed writing image checksum file %s: %w", checksumFile, err)
	}
	return nil
}

// recreateGrubenv creates again the grubenv file on customize process. If no new kernel command line
// is provided it keeps whatever it was defined in the loaded Deployment.
func (i Media) recreateGrubenv(target, kernelCmdline string, loadedDep *deployment.Deployment) error {
	if kernelCmdline == "" {
		kernelCmdline = loadedDep.Installer.KernelCmdline
	}
	switch i.mType {
	case ISO:
		kernelCmdline = fmt.Sprintf("%s %s", deployment.LiveKernelCmdline(i.Label), kernelCmdline)
	case Disk:
		kernelCmdline = fmt.Sprintf("%s %s %s", loadedDep.RecoveryKernelCmdline(), deployment.ResetMark, kernelCmdline)
	default:
		return fmt.Errorf("invalid media type")
	}
	err := i.writeGrubEnv(target, map[string]string{"cmdline": kernelCmdline})
	if err != nil {
		return fmt.Errorf("error writing %s: %w", target, err)
	}
	return nil
}

// increaseRecoverySize increases the recovery partition size based on the data included as part
// of the customize process
func (i Media) increaseRecoverySize(mappedFiles map[string]string, d *deployment.Deployment) error {
	recovery := d.GetRecoveryPartition()
	if recovery == nil {
		if i.mType == Disk {
			return fmt.Errorf("no recovery partition defined")
		}
		// A recovery partition in ISOs is not mandatory
		return nil
	}
	for k := range mappedFiles {
		size, err := vfs.DirSizeMB(i.s.FS(), k)
		if err != nil {
			return fmt.Errorf("failed computing size for '%s': %w", k, err)
		}
		recovery.Size += deployment.MiB(size)
	}

	// Align recovery partition size to 128MiB blocks, this adds between 128MiB and 256MiB to the original size
	recovery.Size = deployment.MiB((recovery.Size/128)*128 + 256)
	i.s.Logger().Debug("Recovery partition resized to %dMiB", recovery.Size)
	return nil
}

// sanitize checks the current public attributes of the ISO object
// and checks if they are good enough to proceed with an ISO build.
func (i *Media) sanitize() error {
	if !filepath.IsAbs(i.OutputDir) {
		path, err := filepath.Abs(i.OutputDir)
		if err != nil {
			return err
		}
		i.OutputDir = path
	}
	if i.Label == "" && i.mType == ISO {
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

	if i.outputFile == "" {
		i.outputFile = filepath.Join(i.OutputDir, fmt.Sprintf("%s.%s", i.Name, i.mType.String()))
		if ok, _ := vfs.Exists(i.s.FS(), i.outputFile); ok {
			return fmt.Errorf("target output file %s is an already existing file", i.outputFile)
		}
	}

	return nil
}

func (i Media) customizeISO(inputFile, outputFile string, fileMap map[string]string) error {
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

func (i Media) writeGrubEnv(file string, vars map[string]string) error {
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
func (i Media) prepareOSRoot(sourceOS *deployment.ImageSource, rootDir string) error {
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
func (i Media) prepareEFI(isoDir, efiDir string) error {
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

// addInstallationAssets adds to the ISO directory three the configuration and files required for
// the installation from the current media
func (i Media) addInstallationAssets(root string, d *deployment.Deployment) error {
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
		}
	}

	return nil
}

// writeInstallDescription writes the installation yaml file embedded in installer media
// with the installer assets related to the live system mount point.
func (i Media) writeInstallDescription(installPath string, d *deployment.Deployment) error {
	// Do not modify original data as the deployment could still be used later stages
	// here we want to store it from live installer PoV
	d, err := d.DeepCopy()
	if err != nil {
		return fmt.Errorf("failed creating a deep copy a deployment: %w", err)
	}

	if d.OverlayTree != nil && !d.OverlayTree.IsEmpty() {
		switch {
		case d.OverlayTree.IsDir():
			d.OverlayTree = deployment.NewDirSrc(filepath.Join(LiveMountPoint, installDir, overlayDir))
		case d.OverlayTree.IsRaw() || d.OverlayTree.IsTar():
			path := filepath.Join(LiveMountPoint, installDir, overlayDir, filepath.Base(d.OverlayTree.URI()))
			if d.OverlayTree.IsTar() {
				d.OverlayTree = deployment.NewTarSrc(path)
			} else {
				d.OverlayTree = deployment.NewRawSrc(path)
			}
		}
	}

	if d.CfgScript != "" {
		d.CfgScript = InstallScript
	}

	if d.Installer.CfgScript != "" {
		d.Installer.CfgScript = filepath.Join(LiveMountPoint, liveDir, cfgScript)
	}

	d.SourceOS = deployment.NewRawSrc(SquashfsPath)
	d.Installer.OverlayTree = deployment.NewDirSrc(LiveMountPoint)

	if i.mType == Disk {
		for _, disk := range d.Disks {
			disk.Device = ""
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

// customizeDisk creates an installer disk image from the prepared root
func (i Media) customizeDisk(tempDir string, d *deployment.Deployment, mappedFiles map[string]string) error {
	isoDir := filepath.Join(tempDir, "extracted-iso")
	err := vfs.MkdirAll(i.s.FS(), isoDir, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating ESP directory: %w", err)
	}

	esp := d.GetEfiPartition()
	recovery := d.GetRecoveryPartition()
	if esp == nil || recovery == nil {
		return fmt.Errorf("undefined essential recovery or esp partitions")
	}

	err = extractISO(i.s, i.InputFile, "/", isoDir)
	if err != nil {
		return err
	}

	sync := rsync.NewRsync(i.s, rsync.WithFlags(rsync.DefaultFlags()...))
	for k, v := range mappedFiles {
		target := filepath.Join(isoDir, v)
		err = vfs.MkdirAll(i.s.FS(), filepath.Dir(target), vfs.DirPerm)
		if err != nil {
			return err
		}
		info, err := i.s.FS().Lstat(k)
		if err != nil {
			return err
		}
		if info.IsDir() {
			err = sync.SyncData(k, target)
		} else {
			err = vfs.CopyFile(i.s.FS(), k, target)
		}
		if err != nil {
			return err
		}
	}

	parts := []repart.Partition{
		{
			Partition: esp,
			CopyFiles: []string{
				fmt.Sprintf("%s/boot:/boot", isoDir), fmt.Sprintf("%s/EFI:/EFI", isoDir),
			},
		}, {
			Partition: recovery,
			CopyFiles: []string{fmt.Sprintf("%s:/", isoDir)},
			Excludes:  []string{filepath.Join(isoDir, "boot"), filepath.Join(isoDir, "EFI")},
		},
	}
	return repart.CreateDiskImage(i.s, i.outputFile, i.rawDiskSize, parts)
}

// buildDisk creates an installer disk image from the prepared root
func (i Media) buildDisk(tempDir, liveRoot, osRoot string, d *deployment.Deployment) error {
	espDir := filepath.Join(tempDir, "esp")
	err := vfs.MkdirAll(i.s.FS(), espDir, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating ESP directory: %w", err)
	}

	esp := d.GetEfiPartition()
	recovery := d.GetRecoveryPartition()
	if esp == nil || recovery == nil {
		return fmt.Errorf("undefined essential recovery or esp partitions")
	}

	// include the reset flag so it can be detected at boot this is an installer image
	cmdline := fmt.Sprintf("%s %s %s", d.RecoveryKernelCmdline(), deployment.ResetMark, d.Installer.KernelCmdline)
	err = i.bl.InstallLive(osRoot, espDir, cmdline)
	if err != nil {
		return fmt.Errorf("failed installing the bootloader for a installer raw image: %w", err)
	}

	parts := []repart.Partition{
		{
			Partition: esp,
			CopyFiles: []string{fmt.Sprintf("%s:/", espDir)},
		}, {
			Partition: recovery,
			CopyFiles: []string{fmt.Sprintf("%s:/", liveRoot)},
		},
	}
	err = repart.CreateDiskImage(i.s, i.outputFile, 0, parts)
	if err != nil {
		return fmt.Errorf("failed creating disk image: %w", err)
	}
	return nil
}

// buildISO creates an ISO image from the prepared root
func (i Media) buildISO(tempDir, isoDir, osRoot, kernelCmdline string) error {
	err := i.bl.InstallLive(osRoot, isoDir, kernelCmdline)
	if err != nil {
		return fmt.Errorf("failed installing bootloader in ISO directory tree: %w", err)
	}

	espDir := filepath.Join(tempDir, "esp")
	err = vfs.MkdirAll(i.s.FS(), espDir, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating ESP directory: %w", err)
	}

	err = i.prepareEFI(isoDir, espDir)
	if err != nil {
		return fmt.Errorf("failed preparing efi partition: %w", err)
	}

	efiImg := filepath.Join(tempDir, filepath.Base(espDir)+".img")
	err = filesystem.CreatePreloadedFileSystemImage(i.s, espDir, efiImg, deployment.EfiLabel, 1, deployment.VFat)
	if err != nil {
		return fmt.Errorf("failed creating EFI image for the installer image: %w", err)
	}

	args := []string{
		"-volid", "LIVE", "-padding", "0",
		"-outdev", i.outputFile, "-map", isoDir, "/", "-chmod", "0755", "--",
	}
	args = append(args, xorrisoBootloaderArgs(efiImg)...)

	_, err = i.s.Runner().RunContext(i.ctx, xorriso, args...)
	if err != nil {
		return fmt.Errorf("failed creating the installer ISO image: %w", err)
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

// reservedPaths returns an array of the paths which can't be overlayed in installer media
func reservedPaths() []string {
	return []string{liveDir, installDir, "EFI", "boot"}
}
