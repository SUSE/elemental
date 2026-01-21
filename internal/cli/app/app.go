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

package app

import (
	"context"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

func Name() string {
	return filepath.Base(os.Args[0])
}

func New(usage string, globalFlags []cli.Flag, setupFunc cli.BeforeFunc, teardownFunc cli.AfterFunc, commands ...*cli.Command) *cli.Command {
	return &cli.Command{
		Flags:    globalFlags,
		Name:     Name(),
		Commands: commands,
		Usage:    usage,
		Suggest:  true,
		Before:   setupFunc,
		After:    teardownFunc,
	}
}

// ActionFunc is the type for command action functions in v3
type ActionFunc func(context.Context, *cli.Command) error
