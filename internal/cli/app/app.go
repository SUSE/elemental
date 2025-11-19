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

package app

import (
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
)

func Name() string {
	return filepath.Base(os.Args[0])
}

func New(usage string, globalFlags []cli.Flag, setupFunc cli.BeforeFunc, teardownFunc cli.AfterFunc, commands ...*cli.Command) *cli.App {
	app := cli.NewApp()

	app.Flags = globalFlags
	app.Name = Name()
	app.Commands = commands
	app.Usage = usage
	app.Suggest = true
	app.Before = setupFunc
	app.After = teardownFunc

	return app
}
