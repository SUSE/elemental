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
	Output  string
	Core    bool
	Product bool
	Local   bool
}

var ReleaseInfoArgs ReleaseInfoFlags

var description = `release-info takes as argument either an OCI image containing a release manifest file in it
or a local release manifest file and prints detailed information about components that make up the Core and Product manifest.`

func NewReleaseInfoCommand(appName string, releaseInfoAction, diffAction func(context.Context, *cli.Command) error) *cli.Command {
	coreFlag := &cli.BoolFlag{
		Name:        "core",
		Usage:       "Print only the Core Release Manifest information; doesn't print details pertaining to Core Release Manifest",
		Destination: &ReleaseInfoArgs.Core,
	}
	productFlag := &cli.BoolFlag{
		Name:        "product",
		Usage:       "Print only the Product Release Manifest information; doesn't print details pertaining to Core Release Manifest",
		Destination: &ReleaseInfoArgs.Product,
	}
	localFlag := &cli.BoolFlag{
		Name:        "local",
		Usage:       "Load OCI images from the local container storage instead of a remote registry",
		Destination: &ReleaseInfoArgs.Local,
	}
	return &cli.Command{
		Name:        "release-info",
		Usage:       "Prints details of components that make up a Core and Product release manifest file",
		Description: fmt.Sprintf("%s %s", appName, description),
		UsageText: fmt.Sprintf("%s release-info [flags] /path/to/manifest/file\n"+
			"%s release-info [flags] oci://registry.fqdn/repository/image", appName, appName),
		Action: releaseInfoAction,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "output",
				Aliases:     []string{"o"},
				Usage:       "Output format. One of: (json, yaml)",
				Destination: &ReleaseInfoArgs.Output,
			},
			coreFlag,
			productFlag,
			localFlag,
		},
		Commands: []*cli.Command{
			{
				Name:  "diff",
				Usage: "Print the difference between two manifest files",
				UsageText: fmt.Sprintf("%s release-info diff /path/to/manifest/file1 /path/to/manifest/file2\n"+
					"%s release-info diff oci://registry.fqdn/repository/image1 oci://registry.fqdn/repository/image2\n"+
					"%s release-info diff oci://registry.fqdn/repository/image /path/to/manifest/file\n", appName, appName, appName),
				Action: diffAction,
				Flags: []cli.Flag{
					coreFlag,
					productFlag,
					localFlag,
				},
				Before: func(ctx context.Context, _ *cli.Command) (context.Context, error) {
					if ReleaseInfoArgs.Output != "" {
						return ctx, fmt.Errorf("diff command doesn't support %q flag", "--output")
					}
					return ctx, nil
				},
			},
		},
	}
}
