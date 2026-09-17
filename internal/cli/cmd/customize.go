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
	Download   string
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

			if !cache.Mode(CustomizeArgs.Cache).IsValid() {
				return ctx, cli.Exit("Error: Unsupported --cache option.", 1)
			}

			if !cache.DownloadMode(CustomizeArgs.Download).IsValid() {
				return ctx, cli.Exit("Error: Unsupported --download option.", 1)
			}

			policy := cache.Policy{Mode: cache.Mode(CustomizeArgs.Cache), DownloadMode: cache.DownloadMode(CustomizeArgs.Download)}
			if err := policy.Validate(); err != nil {
				return ctx, cli.Exit(fmt.Sprintf("Error: %v.", err), 1)
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
				Usage: "Cache mode, 'enabled' caches downloaded artifacts for faster future builds, " +
					"'disabled' neither uses nor populates the cache, " +
					"'refresh' deletes the currently cached artifacts and downloads them again",
				Destination: &CustomizeArgs.Cache,
				Value:       string(cache.Enabled),
			},
			&cli.StringFlag{
				Name: "download",
				Usage: "Download mode, 'auto-pull' downloads any artifact missing from the cache, " +
					"'offline' never downloads and relies solely on the cache",
				Destination: &CustomizeArgs.Download,
				Value:       string(cache.AutoPull),
			},
			&cli.StringFlag{
				Name:        "cache-dir",
				Usage:       "Full path to the cached artifacts directory",
				Destination: &CustomizeArgs.CacheDir,
				Value:       cache.DefaultDir,
			},
		},
	}
}
