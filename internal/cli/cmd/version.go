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

var (
	version = "v0.0.1"
	// gitCommit is the git sha1
	gitCommit = ""
)

func NewVersionCommand(appName string) *cli.Command {
	return &cli.Command{
		Name:      "version",
		Aliases:   []string{"v"},
		Usage:     "Inspect program version",
		UsageText: fmt.Sprintf("%s version", appName),
		Action: func(_ context.Context, _ *cli.Command) error {
			commit := gitCommit
			if len(commit) > 7 {
				commit = gitCommit[:7]
			}

			fmt.Printf("%s+g%s\n", version, commit)

			return nil
		},
	}
}
