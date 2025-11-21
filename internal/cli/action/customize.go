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
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/installer"
	"github.com/suse/elemental/v3/pkg/sys"
)

func Customize(ctx *cli.Context) error {
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	s := ctx.App.Metadata["system"].(*sys.System)
	args := &cmd.CustomizeArgs
	logger := s.Logger()

	ctxCancel, cancelFunc := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer cancelFunc()

	logger.Info("Customizing image")

	media := installer.NewMedia(ctxCancel, s, installer.ISO)

	digestCustomizeSetup(args, media)

	d, err := digestCustomizeDeploymentSetup(s, args)
	if err != nil {
		s.Logger().Error("Failed to collect deployment setup")
		return err
	}

	err = media.Customize(d)
	if err != nil {
		return fmt.Errorf("failed customizing installer media: %w", err)
	}

	s.Logger().Info("Customize complete")

	return nil
}

func digestCustomizeDeploymentSetup(s *sys.System, flags *cmd.CustomizeFlags) (*deployment.Deployment, error) {
	// use an empty deployment instead of a default one, as this will be merged on top of the one embedded
	// inside the installer media.
	d := &deployment.Deployment{}
	if flags.Overlay != "" {
		src, err := deployment.NewSrcFromURI(flags.Overlay)
		if err != nil {
			return nil, fmt.Errorf("invalid overlay data URI (%s) to add into the customization: %w", flags.Overlay, err)
		}
		d.Installer.OverlayTree = src
	}

	if flags.ConfigScript != "" {
		d.Installer.CfgScript = flags.ConfigScript
	}

	if flags.KernelCmdline != "" {
		d.Installer.KernelCmdline = flags.KernelCmdline
	}

	err := applyInstallSpec(s, d, flags.InstallSpec)
	if err != nil {
		return nil, fmt.Errorf("failed applying install flags to deployment description: %w", err)
	}

	return d, nil
}

func digestCustomizeSetup(flags *cmd.CustomizeFlags, media *installer.Media) {
	if flags.Name != "" {
		media.Name = flags.Name
	}

	if flags.OutputDir != "" {
		media.OutputDir = flags.OutputDir
	}

	if flags.Label != "" {
		media.Label = flags.Label
	}

	if flags.InputFile != "" {
		media.InputFile = flags.InputFile
	}
}
