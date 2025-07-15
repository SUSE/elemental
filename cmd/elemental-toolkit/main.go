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

package main

import (
	"log"
	"os"

	"github.com/suse/elemental/v3/internal/cli/app"
	"github.com/suse/elemental/v3/internal/cli/elemental-toolkit/action"
	"github.com/suse/elemental/v3/internal/cli/elemental-toolkit/cmd"
	"github.com/suse/elemental/v3/internal/cli/version"
)

func main() {
	appName := app.Name()
	application := app.New(
		cmd.Usage,
		cmd.GlobalFlags(),
		cmd.Setup,
		cmd.NewInstallCommand(appName, action.Install),
		cmd.NewUpgradeCommand(appName, action.Upgrade),
		cmd.NewUnpackImageCommand(appName, action.Unpack),
		cmd.NewBuildISOCommand(appName, action.BuildInstaller),
		version.NewVersionCommand(appName))

	if err := application.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
