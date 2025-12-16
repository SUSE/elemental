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

package customize

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	_ "embed"

	"github.com/suse/elemental/v3/internal/config"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/install"
	"github.com/suse/elemental/v3/internal/template"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/fips"
	"github.com/suse/elemental/v3/pkg/installer"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	autoInstallerScriptName = "auto_installer.sh"
)

//go:embed templates/auto_installer.sh.tpl
var autoInstallerScriptTpl string

type configManager interface {
	ConfigureComponents(ctx context.Context, conf *image.Configuration, output config.Output) (*resolver.ResolvedManifest, error)
}

type ociFileExtractor interface {
	ExtractFrom(uri string) (path string, err error)
}

type media interface {
	Customize(d *deployment.Deployment) error
}

type Runner struct {
	System        *sys.System
	ConfigManager configManager
	FileExtractor ociFileExtractor
	Media         media
}

func (r *Runner) Run(ctx context.Context, def *image.Definition, output config.Output) (err error) {
	logger := r.System.Logger()

	logger.Info("Configuring image components")
	rm, err := r.ConfigManager.ConfigureComponents(ctx, def.Configuration, output)
	if err != nil {
		logger.Error("Configuring image components failed")
		return err
	}

	containerImage := rm.CorePlatform.Components.OperatingSystem.Image.ISO
	logger.Info("Extracting ISO from container image %s", containerImage)
	iso, err := r.FileExtractor.ExtractFrom(containerImage)
	if err != nil {
		logger.Error("Extracting ISO from container image '%s' failed", containerImage)
		return err
	}

	logger.Info("Loading ISO install description")
	installerDeployment, err := loadISOInstallDesc(r.System, iso, output.RootPath)
	if err != nil {
		logger.Error("Loading ISO install description failed")
		return err
	}

	logger.Info("Parsing media type")
	mediaType, err := installer.StringToMediaType(def.Image.ImageType)
	if err != nil {
		logger.Error("Parsing media type failed")
		return err
	}

	dep, err := parseDeployment(
		r.System.FS(),
		mediaType,
		&def.Configuration.Installation,
		installerDeployment,
		output,
	)

	if err != nil {
		logger.Error("Parsing customization deployment failed")
		return err
	}

	mediaOpts := []installer.Option{
		installer.WithOutputFile(def.Image.OutputImageName),
	}
	if mediaType == installer.Disk {
		diskSizeStr := def.Configuration.Installation.RAW.DiskSize
		if diskSizeStr == "" {
			diskSizeStr = "12G"
		}
		if !diskSizeStr.IsValid() {
			return fmt.Errorf("invalid disk size definition '%s'", diskSizeStr)
		}
		diskMiB, err := diskSizeStr.ToMiB()
		if err != nil {
			return fmt.Errorf("could not parse disk size '%s': %w", diskSizeStr, err)
		}
		mediaOpts = append(mediaOpts, installer.WithRawDiskSize(deployment.MiB(diskMiB)))
	}

	// TODO(ipetrov117): Consider refactoring installer.Media, as right now
	// it is hiding too much information when exposing the Customize() command.
	// This makes abstracting the object behind an interface hard. Perhaps we should separate
	// the disk-installer logic from the disk-customizing logic, or move some of the values
	// currently set in installer.NewMedia into the appropriate functions as parameters.
	if r.Media == nil {
		media := installer.NewMedia(ctx, r.System, mediaType, mediaOpts...)
		media.InputFile = iso
		media.OutputDir = filepath.Dir(def.Image.OutputImageName)
		r.Media = media
	}

	logger.Info("Customizing image media")
	if err = r.Media.Customize(dep); err != nil {
		logger.Error("Customizing image media failed")
		return err
	}

	logger.Info("Customize complete")
	return nil
}

func loadISOInstallDesc(s *sys.System, iso, outputDir string) (dep *deployment.Deployment, err error) {
	tempDir, err := vfs.TempDir(s.FS(), outputDir, "iso-desc-install")
	if err != nil {
		return nil, fmt.Errorf("creating ISO description extract directory: %w", err)
	}

	defer func() {
		rmErr := s.FS().RemoveAll(tempDir)
		if rmErr != nil {
			err = errors.Join(err, rmErr)
		}
	}()

	return installer.LoadISOInstallDesc(s, tempDir, iso)
}

func parseDeployment(
	fs vfs.FS,
	mediaType installer.MediaType,
	install *install.Installation,
	installerDep *deployment.Deployment,
	output config.Output,
	customPartitions ...*deployment.Partition,
) (dep *deployment.Deployment, err error) {
	customizeDisk := &deployment.Disk{}
	d := &deployment.Deployment{Disks: []*deployment.Disk{customizeDisk}}

	additionalPartitions := append([]*deployment.Partition{}, customPartitions...)
	firstbootConfigExists, _ := vfs.Exists(fs, output.FirstbootConfigDir())
	if firstbootConfigExists && output.ConfigPath == "" {
		configSize, err := vfs.DirSizeMB(fs, output.FirstbootConfigDir())
		if err != nil {
			return nil, fmt.Errorf("computing configuration partition size: %w", err)
		}

		configPart := &deployment.Partition{
			Label:      deployment.ConfigLabel,
			MountPoint: deployment.ConfigMnt,
			Role:       deployment.Data,
			FileSystem: deployment.Btrfs,
			Size:       deployment.MiB(configSize/128)*128 + 256,
			Hidden:     true,
		}

		additionalPartitions = append(additionalPartitions, configPart)
	}

	if len(additionalPartitions) > 0 {
		customizeDisk.Partitions = prepareDeploymentPartitions(installerDep.Disks[0].Partitions, additionalPartitions)
	}

	if mediaType == installer.ISO {
		if install.ISO.Device == "" {
			return nil, fmt.Errorf("missing device configuration for ISO image type")
		}

		customizeDisk.Device = install.ISO.Device
	}

	d.BootConfig = &deployment.BootConfig{
		Bootloader:    install.Bootloader,
		KernelCmdline: install.KernelCmdLine,
	}

	d.Security = &deployment.SecurityConfig{
		CryptoPolicy: install.CryptoPolicy,
	}

	if d.IsFipsEnabled() {
		d.BootConfig.KernelCmdline = fips.AppendCommandLine(d.BootConfig.KernelCmdline)
	}

	overlaysURI := fmt.Sprintf("%s://%s", deployment.Dir, output.OverlaysDir())
	overlaySource, err := deployment.NewSrcFromURI(overlaysURI)
	if err != nil {
		return nil, fmt.Errorf("parsing overlay source URI %q: %w", overlaysURI, err)
	}
	d.OverlayTree = overlaySource

	// Make sure that we define a valid installer config script if such was not defined
	// during the build process of the ISO that is currently being customized.
	if installerDep.Installer.CfgScript == "" {
		autoInst, err := writeAutoInstaller(fs, output.RootPath, mediaType)
		if err != nil {
			return nil, fmt.Errorf("writing default '%s' installer script: %w", autoInstallerScriptName, err)
		}

		d.Installer.CfgScript = autoInst
	}

	return d, nil
}

// prepareDeploymentPartitions produces a partition slice that is ready for
// merge with a partition slice coming from the 'install.yaml' file that was
// created during the `build-installer` command.
//
// Currently only additions of new partitions is supported. More complex logic
// is not yet needed and needs to be defined once we flesh out the correct deployment
// merging approach.
func prepareDeploymentPartitions(src, add []*deployment.Partition) []*deployment.Partition {
	preparedPartitionSlice := []*deployment.Partition{}

	for i := range src {
		if i == len(src)-1 {
			// Assume the last partition is SYSTEM and mark its position in
			// the new slice as nil. This is done in order to not break the
			// deployment merge process and have either the SYSTEM partition
			// not defined last, or have it mangled up with another partition.
			preparedPartitionSlice = append(preparedPartitionSlice, nil)
			continue
		}

		// Any existing partitions that are not the last partition are defined as
		// empty partitions in the new slice. This will instruct the deployment merge
		// logic to skip these entries and not merge them with the current deployment
		// partitions.
		preparedPartitionSlice = append(preparedPartitionSlice, &deployment.Partition{})
	}

	// Include the additional partitions to the newly produced slice
	preparedPartitionSlice = append(preparedPartitionSlice, add...)

	// Lastly, redefine the SYSTEM partition, as seen in src.
	// This is done to ensure that the SYSTEM partition will always be defined
	// last, as is the requirement for the OS.
	// ---
	// The final result will look like this:
	// preparedPartitions = [{}, {}, nil, add..., SYSTEM].
	//
	// When this result is merged with the installer partition slice - '[EFI, RECOVERY, SYSTEM]';
	// we will get the following merged result:
	// mergedPartitions = [EFI, RECOVERY, add..., SYSTEM]
	return append(preparedPartitionSlice, src[len(src)-1])
}

func writeAutoInstaller(fs vfs.FS, out string, mediaType installer.MediaType) (string, error) {
	values := struct {
		MediaType string
	}{
		MediaType: mediaType.String(),
	}

	data, err := template.Parse(autoInstallerScriptName, autoInstallerScriptTpl, &values)
	if err != nil {
		return "", fmt.Errorf("parsing auto-installer template: %w", err)
	}

	scriptPath := filepath.Join(out, autoInstallerScriptName)
	if err = fs.WriteFile(scriptPath, []byte(data), 0o744); err != nil {
		return "", fmt.Errorf("writing auto-installer script %q: %w", scriptPath, err)
	}

	return scriptPath, nil
}
