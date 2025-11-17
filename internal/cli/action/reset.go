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
	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/block/lsblk"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/crypto"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/install"
	"github.com/suse/elemental/v3/pkg/installer"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/transaction"
	"github.com/suse/elemental/v3/pkg/upgrade"
)

func Reset(ctx *cli.Context) error { //nolint:dupl
	var s *sys.System
	args := &cmd.ResetArgs
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	s = ctx.App.Metadata["system"].(*sys.System)

	s.Logger().Info("Starting reset action with args: %+v", args)

	d, err := digestResetSetup(s, args)
	if err != nil {
		s.Logger().Error("Failed to collect reset setup")
		return err
	}

	s.Logger().Info("Checked configuration, running reset process")

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
	)
	installer := install.New(
		ctxCancel, s, install.WithUpgrader(upgrader),
		install.WithBootloader(bootloader),
	)

	err = installer.Reset(d)
	if err != nil {
		s.Logger().Error("Reset failed")
		return err
	}

	s.Logger().Info("Reset complete")

	return nil
}

// disgestResetSetup produces the Deployment object required to describe the installation parameters
func digestResetSetup(s *sys.System, flags *cmd.ResetFlags) (*deployment.Deployment, error) {
	d := &deployment.Deployment{}

	if !install.IsRecovery(s) {
		return nil, fmt.Errorf("reset command requires booting from recovery system")
	}

	descriptionFile := installer.InstallDesc
	if flags.Description != "" {
		descriptionFile = flags.Description
	}
	err := loadDescriptionFile(s, descriptionFile, d)
	if err != nil {
		return nil, err
	}

	err = setResetTarget(s, d)
	if err != nil {
		return nil, fmt.Errorf("failed to define target disk: %w", err)
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

	if flags.EnableFips {
		d.Security.CryptoPolicy = crypto.FIPSPolicy
	} else {
		d.Security.CryptoPolicy = crypto.DefaultPolicy
	}

	setBootloader(s, d, flags.Bootloader, flags.KernelCmdline, flags.CreateBootEntry)

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

	err = d.Sanitize(s)
	if err != nil {
		return nil, fmt.Errorf("inconsistent deployment setup found: %w", err)
	}

	return d, nil
}

// setResetTarget sets the target disk of the given deployment to the disk including the live mount point
func setResetTarget(s *sys.System, d *deployment.Deployment) error {
	part, err := block.GetPartitionByMountPoint(s, lsblk.NewLsDevice(s), installer.LiveMountPoint, 1)
	if err != nil {
		return fmt.Errorf("partition for the live mount point not found: %w", err)
	}

	disk := d.GetSystemDisk()
	if disk == nil {
		return fmt.Errorf("no system partition found in deployment")
	}
	disk.Device = part.Disk
	return nil
}
