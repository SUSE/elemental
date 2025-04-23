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
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/upgrade"
)

func Upgrade(ctx *cli.Context) error { //nolint:dupl
	var s *sys.System
	args := &cmd.UpgradeArgs
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	s = ctx.App.Metadata["system"].(*sys.System)

	s.Logger().Info("Starting upgrade action with args: %+v", args)

	d, err := digestUpgradeSetup(s, args)
	if err != nil {
		s.Logger().Error("failed to collect upgrade setup: %v", err)
		return err
	}

	s.Logger().Info("Checked configuration, running upgrade process")

	ctxCancel, stop := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		stop()
	}()

	upgrader := upgrade.New(ctxCancel, s)
	err = upgrader.Upgrade(d)
	if err != nil {
		s.Logger().Error("upgrade failed: %v", err)
		return err
	}

	s.Logger().Info("Upgrade completed")

	return nil
}

func digestUpgradeSetup(s *sys.System, flags *cmd.UpgradeFlags) (*deployment.Deployment, error) {
	d, err := deployment.ReadDeployment(s, "/")
	if err != nil || d == nil {
		return nil, fmt.Errorf("could not read deployment file: %w", err)
	}

	srcOS, err := deployment.NewSrcFromURI(flags.OperatingSystemImage)
	if err != nil {
		return nil, fmt.Errorf("failed parsing OS source URI ('%s'): %w", flags.OperatingSystemImage, err)
	}
	d.SourceOS = srcOS

	err = d.Sanitize(s)
	if err != nil {
		return nil, fmt.Errorf("inconsistent deployment setup found: %w", err)
	}
	return d, nil
}
