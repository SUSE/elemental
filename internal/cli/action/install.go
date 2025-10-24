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

	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/install"
	"github.com/suse/elemental/v3/pkg/installer"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/transaction"
	"github.com/suse/elemental/v3/pkg/unpack"
	"github.com/suse/elemental/v3/pkg/upgrade"
)

func Install(ctx *cli.Context) error { //nolint:dupl
	var s *sys.System
	args := &cmd.InstallArgs
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	s = ctx.App.Metadata["system"].(*sys.System)

	s.Logger().Info("Starting install action with args: %+v", args)

	d, err := digestInstallSetup(s, args)
	if err != nil {
		s.Logger().Error("Failed to collect installation setup")
		return err
	}

	s.Logger().Info("Checked configuration, running installation process")

	ctxCancel, stop := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		stop()
	}()

	bootloader, err := bootloader.New(d.BootConfig.Bootloader, s)
	if err != nil {
		s.Logger().Error("Parsing boot config failed")
		return err
	}

	snapshotter, err := transaction.New(ctxCancel, s, d, d.Snapshotter.Name)
	if err != nil {
		s.Logger().Error("Parsing snapshotter config failed")
		return err
	}

	manager := firmware.NewEfiBootManager(s)
	upgrader := upgrade.New(
		ctxCancel, s, upgrade.WithBootManager(manager), upgrade.WithBootloader(bootloader),
		upgrade.WithSnapshotter(snapshotter),
		upgrade.WithUnpackOpts(unpack.WithVerify(args.Verify), unpack.WithLocal(args.Local)),
	)
	installer := install.New(ctxCancel, s, install.WithUpgrader(upgrader))

	err = installer.Install(d)
	if err != nil {
		s.Logger().Error("Installation failed")
		return err
	}

	s.Logger().Info("Installation complete")

	return nil
}

// isLiveMedia returns true if the current host is a live media, for instance an ISO
func isLiveMedia(s *sys.System) bool {
	mnt, err := s.Mounter().IsMountPoint(installer.LiveMountPoint)
	if !mnt || err != nil {
		return false
	}
	exists, _ := vfs.Exists(s.FS(), installer.SquashfsPath)
	return exists
}

// loadDescriptionFile reads the given deployment description file into the given deployment object
func loadDescriptionFile(s *sys.System, file string, d *deployment.Deployment) error {
	data, err := s.FS().ReadFile(file)
	if err != nil {
		return fmt.Errorf("could not read description file '%s': %w", file, err)
	}
	err = yaml.Unmarshal(data, d)
	if err != nil {
		return fmt.Errorf("could not unmarshal config file: %w", err)
	}
	s.Logger().Info("Loaded deployment description file: %s", file)
	return nil
}

// setBootloader configures the bootloader for the given deployment with the given flags
func setBootloader(s *sys.System, d *deployment.Deployment, flags *cmd.InstallFlags) {
	disk := d.GetSystemDisk()
	if flags.CreateBootEntry && disk != nil {
		d.Firmware.BootEntries = []*firmware.EfiBootEntry{
			firmware.DefaultBootEntry(s.Platform(), disk.Device),
		}
	}

	if d.BootConfig == nil {
		d.BootConfig = &deployment.BootConfig{}
	}
	if flags.Bootloader != bootloader.BootNone {
		d.BootConfig.Bootloader = flags.Bootloader
	}

	if flags.KernelCmdline != "" {
		d.BootConfig.KernelCmdline = fmt.Sprintf("%s %s", d.BootConfig.KernelCmdline, flags.KernelCmdline)
	}

	if flags.EnableFips {
		d.Fips = &deployment.FipsConfig{
			Enabled: true,
		}

		bootFlag := fmt.Sprintf("boot=LABEL=%s", deployment.EfiLabel)
		d.BootConfig.KernelCmdline = fmt.Sprintf("%s %s %s", d.BootConfig.KernelCmdline, "fips=1", bootFlag)
	}
}

// disgestInstallSetup produces the Deployment object required to describe the installation parameters
func digestInstallSetup(s *sys.System, flags *cmd.InstallFlags) (*deployment.Deployment, error) {
	d := deployment.DefaultDeployment()

	// Given flags have always precedence compared to in place configuration of live media
	if flags.Description != "" {
		err := loadDescriptionFile(s, flags.Description, d)
		if err != nil {
			return nil, err
		}
	} else if isLiveMedia(s) {
		if ok, _ := vfs.Exists(s.FS(), installer.InstallDesc); ok {
			err := loadDescriptionFile(s, installer.InstallDesc, d)
			if err != nil {
				return nil, err
			}
		}
	}

	disk := d.GetSystemDisk()
	if flags.Target != "" && disk != nil {
		disk.Device = flags.Target
	}

	if flags.OperatingSystemImage != "" {
		srcOS, err := deployment.NewSrcFromURI(flags.OperatingSystemImage)
		if err != nil {
			return nil, fmt.Errorf("failed parsing OS source URI ('%s'): %w", flags.OperatingSystemImage, err)
		}
		d.SourceOS = srcOS
	}

	if flags.Overlay != "" {
		overlay, err := deployment.NewSrcFromURI(flags.Overlay)
		if err != nil {
			return nil, fmt.Errorf("failed parsing overlay source URI ('%s'): %w", flags.Overlay, err)
		}
		d.OverlayTree = overlay
	}

	if flags.ConfigScript != "" {
		d.CfgScript = flags.ConfigScript
	}

	setBootloader(s, d, flags)

	if flags.Snapshotter != "" {
		d.Snapshotter.Name = flags.Snapshotter

		if d.Snapshotter.Name == "overwrite" {
			s.Logger().Warn("'overwrite' snapshotter is a debugging tool and should not be used for production installation")

			sysPart := d.GetSystemPartition()
			if sysPart != nil {
				sysPart.FileSystem = deployment.Ext4
				sysPart.RWVolumes = nil
			}
		}
	}

	err := d.Sanitize(s)
	if err != nil {
		return nil, fmt.Errorf("inconsistent deployment setup found: %w", err)
	}

	return d, nil
}
