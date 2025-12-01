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

package build

import (
	"context"
	"fmt"
	"strings"

	"github.com/suse/elemental/v3/internal/config"
	"github.com/suse/elemental/v3/internal/image"
	imginstall "github.com/suse/elemental/v3/internal/image/install"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/fips"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/install"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
	"github.com/suse/elemental/v3/pkg/upgrade"
)

type configManager interface {
	ConfigureComponents(ctx context.Context, conf *image.Configuration, output config.OutputDir) (*resolver.ResolvedManifest, error)
}

type Builder struct {
	System        *sys.System
	ConfigManager configManager
	Local         bool
}

func (b *Builder) Run(ctx context.Context, d *image.Definition, outputDir config.OutputDir) error {
	logger := b.System.Logger()
	runner := b.System.Runner()

	logger.Info("Configuring image components")
	rm, err := b.ConfigManager.ConfigureComponents(ctx, d.Configuration, outputDir)
	if err != nil {
		logger.Error("Configuring image components")
		return err
	}

	logger.Info("Creating RAW disk image")
	if err = createDisk(runner, d.Image, d.Configuration.Installation.RAW.DiskSize); err != nil {
		logger.Error("Creating RAW disk image failed")
		return err
	}

	logger.Info("Attaching loop device to RAW disk image")
	device, err := attachDevice(runner, d.Image)
	if err != nil {
		logger.Error("Attaching loop device failed")
		return err
	}
	defer func() {
		if dErr := detachDevice(runner, device); dErr != nil {
			logger.Error("Detaching loop device failed: %v", dErr)
		}
	}()

	err = vfs.MkdirAll(b.System.FS(), outputDir.OverlaysDir(), vfs.DirPerm)
	if err != nil {
		logger.Error("Failed creating overlay dir")
		return err
	}

	logger.Info("Preparing installation setup")
	dep, err := newDeployment(
		b.System,
		device,
		rm.CorePlatform.Components.OperatingSystem.Image.Base,
		&d.Configuration.Installation,
		outputDir,
	)
	if err != nil {
		logger.Error("Preparing installation setup failed")
		return err
	}

	boot, err := bootloader.New(dep.BootConfig.Bootloader, b.System)
	if err != nil {
		logger.Error("Parsing boot config failed")
		return err
	}

	unpackOpts := unpack.WithLocal(b.Local)
	manager := firmware.NewEfiBootManager(b.System)
	upgrader := upgrade.New(
		ctx, b.System, upgrade.WithBootManager(manager), upgrade.WithBootloader(boot),
		upgrade.WithUnpackOpts(unpackOpts),
	)
	installer := install.New(
		ctx, b.System, install.WithUpgrader(upgrader),
		install.WithUnpackOpts(unpackOpts),
	)

	logger.Info("Installing OS")
	if err = installer.Install(dep); err != nil {
		logger.Error("Installation failed")
		return err
	}

	logger.Info("Installation complete")

	return nil
}

func newDeployment(
	system *sys.System,
	installationDevice, osImage string,
	installation *imginstall.Installation,
	outputDir config.OutputDir,
	customPartitions ...*deployment.Partition,
) (*deployment.Deployment, error) {
	deploymentOpts := []deployment.Opt{
		deployment.WithPartitions(1, customPartitions...),
	}

	if ok, _ := vfs.Exists(system.FS(), outputDir.FirstbootConfigDir()); ok {
		configSize, err := vfs.DirSizeMB(system.FS(), outputDir.FirstbootConfigDir())
		if err != nil {
			return nil, fmt.Errorf("computing configuration partition size: %w", err)
		}

		deploymentOpts = append(deploymentOpts, deployment.WithConfigPartition(deployment.MiB(configSize)))
	}

	d := deployment.New(deploymentOpts...)

	d.Disks[0].Device = installationDevice
	d.BootConfig.Bootloader = installation.Bootloader
	d.BootConfig.KernelCmdline = installation.KernelCmdLine
	d.Security.CryptoPolicy = installation.CryptoPolicy

	if d.IsFipsEnabled() {
		d.BootConfig.KernelCmdline = fips.AppendCommandLine(d.BootConfig.KernelCmdline)
	}

	osURI := fmt.Sprintf("%s://%s", deployment.OCI, osImage)
	osSource, err := deployment.NewSrcFromURI(osURI)
	if err != nil {
		return nil, fmt.Errorf("parsing OS source URI %q: %w", osURI, err)
	}
	d.SourceOS = osSource

	overlaysURI := fmt.Sprintf("%s://%s", deployment.Dir, outputDir.OverlaysDir())
	overlaySource, err := deployment.NewSrcFromURI(overlaysURI)
	if err != nil {
		return nil, fmt.Errorf("parsing overlay source URI %q: %w", overlaysURI, err)
	}
	d.OverlayTree = overlaySource

	if err = d.Sanitize(system); err != nil {
		return nil, fmt.Errorf("sanitizing deployment: %w", err)
	}

	return d, nil
}

func createDisk(runner sys.Runner, img image.Image, diskSize imginstall.DiskSize) error {
	const defaultSize = "10G"

	if diskSize == "" {
		diskSize = defaultSize
	} else if !diskSize.IsValid() {
		return fmt.Errorf("invalid disk size definition '%s'", diskSize)
	}

	_, err := runner.Run("truncate", "-s", string(diskSize), img.OutputImageName)
	return err
}

func attachDevice(runner sys.Runner, img image.Image) (string, error) {
	out, err := runner.Run("losetup", "-f", "--show", img.OutputImageName)
	if err != nil {
		return "", err
	}

	device := strings.TrimSpace(string(out))
	return device, nil
}

func detachDevice(runner sys.Runner, device string) error {
	_, err := runner.Run("losetup", "-d", device)
	return err
}
