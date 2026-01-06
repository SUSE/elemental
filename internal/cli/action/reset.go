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

package action

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"

	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/block/lsblk"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/install"
	"github.com/suse/elemental/v3/pkg/installer"
	"github.com/suse/elemental/v3/pkg/sys"
)

func Reset(ctx *cli.Context) error {
	var s *sys.System
	args := &cmd.InstallArgs
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	s = ctx.App.Metadata["system"].(*sys.System)

	s.Logger().Info("Starting reset action")
	s.Logger().Debug("Reset action called with args: %+v", args)

	d, err := digestResetSetup(s, args)
	if err != nil {
		s.Logger().Error("Failed to collect reset setup")
		return err
	}

	ctxCancel, stop := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		stop()
	}()

	installer, err := initInstaller(ctxCancel, s, d, args)
	if err != nil {
		return fmt.Errorf("initiating installer components: %w", err)
	}

	s.Logger().Info("Running reset process")

	err = installer.Reset(d)
	if err != nil {
		s.Logger().Error("Reset failed")
		return err
	}

	s.Logger().Info("Reset complete")

	return nil
}

// disgestResetSetup produces the Deployment object required to describe the installation parameters
func digestResetSetup(s *sys.System, flags *cmd.InstallFlags) (*deployment.Deployment, error) {
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

	err = applyInstallFlags(s, d, flags)
	if err != nil {
		return nil, fmt.Errorf("defining the deployment details: %w", err)
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
