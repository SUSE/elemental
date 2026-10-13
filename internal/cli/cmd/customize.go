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
	"runtime"
	"slices"

	"github.com/suse/elemental/v3/pkg/cache"
	"github.com/suse/elemental/v3/pkg/installer"
	"github.com/urfave/cli/v3"
)

type CustomizeFlags struct {
	ConfigDir  string
	OutputPath string
	Mode       string
	Platform   string
	MediaType  string
	Cache      string
	CacheDir   string
	Airgap     bool
}

var CustomizeArgs CustomizeFlags

func NewCustomizeCommand(appName string, action func(context.Context, *cli.Command) error) *cli.Command {
	return &cli.Command{
		Name:      "customize",
		Usage:     "Customize an image based on a release",
		UsageText: fmt.Sprintf("%s customize", appName),
		Before: func(ctx context.Context, _ *cli.Command) (context.Context, error) {
			modes := []string{"", "embedded", "split"}
			if !slices.Contains(modes, CustomizeArgs.Mode) {
				return ctx, cli.Exit("Error: Unsupported --mode option.", 1)
			}

			cacheMode := cache.Mode(CustomizeArgs.Cache)
			if !cacheMode.IsValid() {
				return ctx, cli.Exit("Error: Unsupported --cache option.", 1)
			}

			if cacheMode == cache.Off && CustomizeArgs.CacheDir != cache.DefaultDir {
				return ctx, cli.Exit("Error: --cache-dir cannot be specified when --cache is 'off'.", 1)
			}

			return ctx, nil
		},
		Action: action,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "type",
				Usage:       "Type of the installer media, 'iso' or 'raw'",
				Destination: &CustomizeArgs.MediaType,
				Value:       installer.ISO.String(),
			},
			&cli.StringFlag{
				Name:        "config-dir",
				Usage:       "Full path to the image configuration directory",
				Destination: &CustomizeArgs.ConfigDir,
				Value:       "/config",
			},
			&cli.StringFlag{
				Name:        outputFlg,
				Aliases:     []string{"o"},
				Usage:       outputDesc,
				Destination: &CustomizeArgs.OutputPath,
				DefaultText: "image-<timestamp>.<image-type>",
			},
			&cli.StringFlag{
				Name: "mode",
				Usage: "Customization mode, 'embedded' (config partition within image) or " +
					"'split' (configuration directory separate from the image)",
				Destination: &CustomizeArgs.Mode,
				Value:       "embedded",
			},
			&cli.StringFlag{
				Name:        platformFlg,
				Usage:       platformDesc,
				Destination: &CustomizeArgs.Platform,
				Value:       fmt.Sprintf("linux/%s", runtime.GOARCH),
			},
			&cli.StringFlag{
				Name: "cache",
				Usage: "Cache mode, 'auto' serves cached artifacts and downloads any that are missing, " +
					"'offline' serves cached artifacts only and fails if any are missing, " +
					"'refresh' discards the cached artifacts and downloads them again, " +
					"'off' neither uses nor populates the cache",
				Destination: &CustomizeArgs.Cache,
				Value:       string(cache.Auto),
			},
			&cli.StringFlag{
				Name: "cache-dir",
				Usage: "Full path to the cached artifacts directory. If different from the default path, it's expected to be mounted into the volume. " +
					"If the default path does not exist, '<config-dir>/cache' is used instead",
				Destination: &CustomizeArgs.CacheDir,
				Value:       cache.DefaultDir,
			},
			&cli.BoolFlag{
				Name: "airgap",
				Usage: "Build an image that can be deployed offline with all of the necessary artifacts automatically " +
					"embedded into the image like Helm charts, Kubernetes artifacts, container images.",
				Destination: &CustomizeArgs.Airgap,
			},
		},
	}
}
