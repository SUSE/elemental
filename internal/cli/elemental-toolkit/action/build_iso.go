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
	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/internal/cli/elemental-toolkit/cmd"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/installermedia"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
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

	d, err := digestInstallerDeploymentSetup(s, args)
	if err != nil {
		s.Logger().Error("Failed to collect build setup")
		return err
	}

	s.Logger().Info("Running build process")

	ctxCancel, stop := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		stop()
	}()

	media := installermedia.NewISO(ctxCancel, s, installermedia.WithUnpackOpts(unpack.WithLocal(args.Local), unpack.WithVerify(args.Verify)))

	err = digestInstallerSetup(args, media)
	if err != nil {
		return fmt.Errorf("invalid installer setup: %w", err)
	}

	err = media.Build(d)
	if err != nil {
		return fmt.Errorf("failed building installer media: %w", err)
	}

	s.Logger().Info("Build complete")

	return nil
}

func digestInstallerSetup(flags *cmd.InstallerFlags, media *installermedia.ISO) error {
	src, err := deployment.NewSrcFromURI(flags.OperatingSystemImage)
	if err != nil {
		return fmt.Errorf("invalid OS image URI (%s) to build installer: %w", flags.OperatingSystemImage, err)
	}
	media.SourceOS = src

	if flags.Overlay != "" {
		src, err = deployment.NewSrcFromURI(flags.Overlay)
		if err != nil {
			return fmt.Errorf("invalid overlay data URI (%s) to add into the installer: %w", flags.Overlay, err)
		}
		media.OverlayTree = src
	}

	if flags.ConfigScript != "" {
		media.CfgScript = flags.ConfigScript
	}

	if flags.Name != "" {
		media.Name = flags.Name
	}

	if flags.OutputDir != "" {
		media.OutputDir = flags.OutputDir
	}

	if flags.Label != "" {
		media.Label = flags.Label
	}

	if flags.KernelCmdLine != "" {
		media.KernelCmdLine = flags.KernelCmdLine
	}
	return nil
}

func digestInstallerDeploymentSetup(s *sys.System, flags *cmd.InstallerFlags) (*deployment.Deployment, error) {
	d := deployment.DefaultDeployment()
	if flags.InstallSpec.Description != "" {
		if ok, _ := vfs.Exists(s.FS(), flags.InstallSpec.Description); !ok {
			return nil, fmt.Errorf("config file '%s' not found", flags.InstallSpec.Description)
		}
		data, err := s.FS().ReadFile(flags.InstallSpec.Description)
		if err != nil {
			return nil, fmt.Errorf("could not read description file '%s': %w", flags.InstallSpec.Description, err)
		}
		err = yaml.Unmarshal(data, d)
		if err != nil {
			return nil, fmt.Errorf("could not unmarshal config file: %w", err)
		}
	}
	if flags.InstallSpec.Target != "" && len(d.Disks) > 0 {
		d.Disks[0].Device = flags.InstallSpec.Target
	}

	// Only overwrite OS source to installer OS if undefined
	if d.SourceOS == nil || d.SourceOS.IsEmpty() {
		srcOS := deployment.NewRawSrc(installermedia.SquashfsPath)
		d.SourceOS = srcOS
	}

	if flags.InstallSpec.Overlay != "" {
		overlay, err := deployment.NewSrcFromURI(flags.InstallSpec.Overlay)
		if err != nil {
			return nil, fmt.Errorf("failed parsing overlay source URI ('%s'): %w", flags.InstallSpec.Overlay, err)
		}
		d.OverlayTree = overlay
	}

	if flags.InstallSpec.ConfigScript != "" {
		d.CfgScript = flags.InstallSpec.ConfigScript
	}

	// Always add the default boot entry
	d.Firmware.BootEntries = []*firmware.EfiBootEntry{
		firmware.DefaultBootEntry(s.Platform(), d.Disks[0].Device),
	}

	// Default to grub bootloader if none is defined
	if d.BootConfig.Bootloader == bootloader.BootNone {
		d.BootConfig.Bootloader = bootloader.BootGrub
	}

	if flags.InstallSpec.KernelCmdline != "" {
		d.BootConfig.KernelCmdline = flags.InstallSpec.KernelCmdline
	}

	err := d.Sanitize(s)
	if err != nil {
		return nil, fmt.Errorf("inconsistent deployment setup found: %w", err)
	}
	return d, nil
}
