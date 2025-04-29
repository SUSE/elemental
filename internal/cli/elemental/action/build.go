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

package action

import (
	"fmt"
	"os"
	"slices"

	"github.com/suse/elemental/v3/internal/cli/elemental/cmd"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/urfave/cli/v2"
)

func Build(ctx *cli.Context) error {
	args := &cmd.BuildArgs

	if ctx.App.Metadata == nil || ctx.App.Metadata["logger"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	logger := ctx.App.Metadata["logger"].(log.Logger)

	logger.Info("Validating input args")

	if err := validateArgs(args); err != nil {
		logger.Error("Input args are invalid")
		return err
	}

	logger.Info("Reading image configuration")

	// Perform initial setup and branch off to the actual business logic

	return nil
}

func validateArgs(args *cmd.BuildFlags) error {
	_, err := os.Stat(args.ConfigDir)
	if err != nil {
		return fmt.Errorf("reading config directory: %w", err)
	}

	validImageTypes := []string{image.TypeRAW}
	validImageArchs := []image.Arch{image.ArchTypeARM, image.ArchTypeX86}

	if !slices.Contains(validImageTypes, args.ImageType) {
		return fmt.Errorf("image type %q not supported", args.ImageType)
	}

	if !slices.Contains(validImageArchs, image.Arch(args.Architecture)) {
		return fmt.Errorf("image arch %q not supported", args.Architecture)
	}

	return nil
}
