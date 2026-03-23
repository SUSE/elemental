/*
Copyright © 2026 SUSE LLC
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

package cmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

type InitFlags struct {
	TargetDir string
}

var InitArgs InitFlags

func NewInitCommand(appName string, action func(context.Context, *cli.Command) error) *cli.Command {
	return &cli.Command{
		Name:      "init",
		Usage:     "Create a new image configuration directory",
		UsageText: fmt.Sprintf("%s init [OPTIONS] [TARGET_DIR]", appName),
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			if cmd.Args().Len() > 1 {
				return ctx, fmt.Errorf("expected just one argument for TARGET_DIR")
			}
			if cmd.Args().Len() == 1 {
				InitArgs.TargetDir = cmd.Args().First()
			}
			return ctx, nil
		},
		Action: action,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "target-dir",
				Usage:       "Directory to write the configuration files to",
				Destination: &InitArgs.TargetDir,
				Value:       ".",
			},
		},
	}
}
