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

package cmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

type KernelModulesFlags struct {
	Reload bool
	Unload bool
}

var KernelModulesArgs KernelModulesFlags

func NewKernelModulesCommand(appName string, action func(context.Context, *cli.Command) error) *cli.Command {
	return &cli.Command{
		Name:      "kmod",
		Usage:     "Manage kernel modules on the system",
		UsageText: fmt.Sprintf("%s kmod [OPTIONS]", appName),
		Before: func(ctx context.Context, _ *cli.Command) (context.Context, error) {
			if KernelModulesArgs.Reload && KernelModulesArgs.Unload {
				return ctx, cli.Exit("Error: Both --reload and --unload flags cannot be used together.", 1)
			}

			if !KernelModulesArgs.Reload && !KernelModulesArgs.Unload {
				return ctx, cli.Exit("Error: At least one of --reload or --unload flags must be specified.", 1)
			}

			return ctx, nil
		},
		Action: action,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "reload",
				Usage:       "Reload kernel modules",
				Destination: &KernelModulesArgs.Reload,
			},
			&cli.BoolFlag{
				Name:        "unload",
				Usage:       "[EXPERIMENTAL] Unload kernel modules",
				Destination: &KernelModulesArgs.Unload,
			},
		},
	}
}
