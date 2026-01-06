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
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const Usage = "Install and upgrade immutable operating systems"

var (
	logFile *os.File
)

func GlobalFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "Set logging at debug level",
		},
		&cli.StringFlag{
			Name:  "log-file",
			Usage: "Save logs to file, accepts path to file or stdout/stderr",
		},
	}
}

func Setup(ctx *cli.Context) error {
	s, err := sys.NewSystem()
	if err != nil {
		return err
	}

	if ctx.Bool("debug") {
		s.Logger().SetLevel(log.DebugLevel())
	}

	if err = SetLoggerTarget(s, ctx); err != nil {
		return err
	}

	if ctx.App.Metadata == nil {
		ctx.App.Metadata = map[string]any{}
	}
	ctx.App.Metadata["system"] = s
	return nil
}

func Teardown(_ *cli.Context) error {
	if logFile != nil {
		return logFile.Close()
	}

	return nil
}

func SetLoggerTarget(s *sys.System, ctx *cli.Context) error {
	logPath := ctx.String("log-file")
	switch logPath {
	case "":
		break
	case "-":
	case "stdout":
		s.Logger().SetOutput(os.Stdout)
	case "stderr":
		s.Logger().SetOutput(os.Stderr)
	default:
		var err error
		logFile, err = s.FS().OpenFile(logPath, os.O_WRONLY|os.O_CREATE, vfs.FilePerm)
		if err != nil {
			return fmt.Errorf("opening log file '%s': %w", logPath, err)
		}
		s.Logger().SetOutput(logFile)
	}

	return nil
}
