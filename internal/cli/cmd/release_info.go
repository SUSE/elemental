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

type ReleaseInfoFlags struct {
	Local    bool
	Markdown bool
}

var ReleaseInfoArgs ReleaseInfoFlags

var description = `release-info takes as argument either an OCI image containing a release manifest file in it
or a local release manifest file and prints detailed information about components that make up the Core and Product manifest.`

func NewReleaseInfoCommand(appName string, releaseInfoAction func(context.Context, *cli.Command) error) *cli.Command {
	return &cli.Command{
		Name:        "release-info",
		Usage:       "Prints details of components that make up a Core and Product release manifest file",
		Description: fmt.Sprintf("%s %s", appName, description),
		UsageText:   fmt.Sprintf("%s release-info [options] <manifest-file>\n%s release-info [options] oci://registry.com/repo/manifest", appName, appName),
		Action:      releaseInfoAction,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "local",
				Usage:       "Load OCI images from the local container storage instead of a remote registry",
				Destination: &ReleaseInfoArgs.Local,
			},
			&cli.BoolFlag{
				Name:        "markdown",
				Aliases:     []string{"m"},
				Usage:       "Generate markdown output that can be copy-paste into a markdown editor",
				Destination: &ReleaseInfoArgs.Markdown,
			},
		},
	}
}
