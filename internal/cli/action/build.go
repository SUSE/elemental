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
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/suse/elemental/v3/internal/build"
	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/internal/config"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/helm"
	"github.com/suse/elemental/v3/pkg/http"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/platform"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func Build(ctx *cli.Context) error {
	args := &cmd.BuildArgs

	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	system := ctx.App.Metadata["system"].(*sys.System)
	logger := system.Logger()

	ctxCancel, cancelFunc := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer cancelFunc()

	logger.Info("Validating input args")
	if err := validateArgs(system.FS(), args); err != nil {
		logger.Error("Input args are invalid")
		return err
	}

	logger.Info("Reading image configuration")
	definition, err := parseImageDefinition(system.FS(), args)
	if err != nil {
		logger.Error("Parsing image configuration failed")
		return err
	}

	logger.Info("Validated image configuration")

	buildOutput := fmt.Sprintf("build-%s", time.Now().UTC().Format("2006-01-02T15-04-05"))
	outDir, err := config.CreateOutputDir(system.FS(), args.BuildDir, buildOutput, 0700)
	if err != nil {
		logger.Error("Creating build directory failed")
		return err
	}

	defer func() {
		logger.Debug("Cleaning up build-dir %s", outDir)
		rmErr := system.FS().RemoveAll(string(outDir))
		if rmErr != nil {
			logger.Error("Cleaning up build-dir '%s' failed: %v", outDir, rmErr)
		}
	}()

	valuesResolver := &helm.ValuesResolver{
		ValuesDir: config.Dir(args.ConfigDir).HelmValuesDir(),
		FS:        system.FS(),
	}

	configManager := config.NewManager(
		system,
		config.NewHelm(system.FS(), valuesResolver, logger, outDir.OverlaysDir()),
		config.WithDownloadFunc(http.DownloadFile),
		config.WithLocal(args.Local),
	)

	builder := &build.Builder{
		System:        system,
		ConfigManager: configManager,
		Local:         args.Local,
	}

	logger.Info("Starting build process for %s %s image", definition.Image.Platform.String(), definition.Image.ImageType)
	if err = builder.Run(ctxCancel, definition, outDir); err != nil {
		logger.Error("Build process failed")
		return err
	}

	logger.Info("Build process complete")
	return nil
}

func validateArgs(fs vfs.FS, args *cmd.BuildFlags) error {
	_, err := fs.Stat(args.ConfigDir)
	if err != nil {
		return fmt.Errorf("reading config directory: %w", err)
	}

	validImageTypes := []string{image.TypeRAW}
	if !slices.Contains(validImageTypes, args.ImageType) {
		return fmt.Errorf("image type %q not supported", args.ImageType)
	}

	if _, err := platform.Parse(args.Platform); err != nil {
		return fmt.Errorf("malformed platform %q", args.Platform)
	}

	return nil
}

func parseImageDefinition(f vfs.FS, args *cmd.BuildFlags) (*image.Definition, error) {
	outputPath := args.OutputPath
	if outputPath == "" {
		imageName := fmt.Sprintf("image-%s.%s", time.Now().UTC().Format("2006-01-02T15-04-05"), args.ImageType)
		outputPath = filepath.Join(args.BuildDir, imageName)
	}

	p, err := platform.Parse(args.Platform)
	if err != nil {
		return nil, fmt.Errorf("error parsing platform %s", args.Platform)
	}

	conf, err := config.Parse(f, config.Dir(args.ConfigDir))
	if err != nil {
		return nil, fmt.Errorf("parsing configuration directory %s: %w", args.ConfigDir, err)
	}

	return &image.Definition{
		Image: image.Image{
			ImageType:       args.ImageType,
			Platform:        p,
			OutputImageName: outputPath,
		},
		Configuration: conf,
	}, nil
}
