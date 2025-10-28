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

package action

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"

	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/installer"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/unpack"
)

func BuildInstaller(ctx *cli.Context) error { //nolint:dupl
	var s *sys.System
	args := &cmd.InstallerArgs
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	s = ctx.App.Metadata["system"].(*sys.System)

	s.Logger().Info("Starting build ISO action with args: %+v", args)

	ctxCancel, stop := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		stop()
	}()

	media := installer.NewISO(ctxCancel, s, installer.WithUnpackOpts(unpack.WithLocal(args.Local), unpack.WithVerify(args.Verify)))

	digestInstallerSetup(args, media)

	d, err := digestInstallerDeploymentSetup(s, args)
	if err != nil {
		s.Logger().Error("Failed to collect build setup")
		return err
	}

	s.Logger().Info("Running build process")

	err = media.Build(d)
	if err != nil {
		return fmt.Errorf("failed building installer media: %w", err)
	}

	s.Logger().Info("Build complete")

	return nil
}

func digestInstallerDeploymentSetup(s *sys.System, flags *cmd.InstallerFlags) (*deployment.Deployment, error) {
	d := deployment.DefaultDeployment()
	if flags.Overlay != "" {
		src, err := deployment.NewSrcFromURI(flags.Overlay)
		if err != nil {
			return nil, fmt.Errorf("invalid overlay data URI (%s) to add into the installer: %w", flags.Overlay, err)
		}
		d.Installer.OverlayTree = src
	}

	if flags.ConfigScript != "" {
		d.Installer.CfgScript = flags.ConfigScript
	}
	if flags.KernelCmdLine != "" {
		d.Installer.KernelCmdline = flags.KernelCmdLine
	}

	src, err := deployment.NewSrcFromURI(flags.OperatingSystemImage)
	if err != nil {
		return nil, fmt.Errorf("invalid OS image URI (%s) to build installer: %w", flags.OperatingSystemImage, err)
	}
	d.SourceOS = src

	err = applyInstallFlags(s, d, flags.InstallSpec)
	if err != nil {
		return nil, fmt.Errorf("failed applying install flags to deployment description: %w", err)
	}

	err = d.Sanitize(s, deployment.CheckDiskDevice)
	if err != nil {
		return nil, fmt.Errorf("inconsistent deployment setup found: %w", err)
	}
	return d, err
}

func digestInstallerSetup(flags *cmd.InstallerFlags, media *installer.ISO) {
	if flags.Name != "" {
		media.Name = flags.Name
	}

	if flags.OutputDir != "" {
		media.OutputDir = flags.OutputDir
	}

	if flags.Label != "" {
		media.Label = flags.Label
	}
}

func applyInstallFlags(s *sys.System, d *deployment.Deployment, flags cmd.InstallFlags) error {
	if flags.Description != "" {
		err := loadDescriptionFile(s, flags.Description, d)
		if err != nil {
			return err
		}
	}

	disk := d.GetSystemDisk()
	if flags.Target != "" && disk != nil {
		disk.Device = flags.Target
	}

	if flags.Overlay != "" {
		overlay, err := deployment.NewSrcFromURI(flags.Overlay)
		if err != nil {
			return fmt.Errorf("failed parsing overlay source URI ('%s'): %w", flags.Overlay, err)
		}
		d.OverlayTree = overlay
	}

	if flags.ConfigScript != "" {
		d.CfgScript = flags.ConfigScript
	}

	// Default to grub bootloader if none is defined
	if d.BootConfig.Bootloader == bootloader.BootNone {
		d.BootConfig.Bootloader = bootloader.BootGrub
	}

	if flags.KernelCmdline != "" {
		d.BootConfig.KernelCmdline = flags.KernelCmdline
	}
	return nil
}
