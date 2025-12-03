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
	"context"
	"errors"
	"fmt"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/internal/config"
	"github.com/suse/elemental/v3/internal/customize"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/extractor"
	"github.com/suse/elemental/v3/pkg/helm"
	"github.com/suse/elemental/v3/pkg/http"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/platform"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func Customize(ctx *cli.Context) (err error) {
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	system := ctx.App.Metadata["system"].(*sys.System)
	logger := system.Logger()
	args := &cmd.CustomizeArgs

	ctxCancel, cancelFunc := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer cancelFunc()

	logger.Info("Customizing image")

	customizeDir := fmt.Sprintf("customize-%s", time.Now().UTC().Format("2006-01-02T15-04-05"))
	outDir, err := config.CreateOutputDir(system.FS(), args.CustomizeOutput, customizeDir, 0700)
	if err != nil {
		logger.Error("Creating customize directory '%s' failed", outDir)
		return err
	}

	defer func() {
		logger.Debug("Cleaning up customize-dir %s", outDir)
		rmErr := system.FS().RemoveAll(string(outDir))
		if rmErr != nil {
			logger.Error("Cleaning up customize-dir %s", outDir)
			err = errors.Join(err, rmErr)
		}
	}()

	def, err := digestCustomizeDefinition(system.FS(), args)
	if err != nil {
		logger.Error("Digesting image definition from customize flags")
		return err
	}

	customizeRunner, err := setupCustomizeRunner(ctxCancel, system, args, outDir)
	if err != nil {
		logger.Error("Setting up customization runner")
		return err
	}

	if err = customizeRunner.Run(ctxCancel, def, outDir, args.Local); err != nil {
		logger.Error("Failed customizing installer media")
		return err
	}

	return nil
}

func setupCustomizeRunner(
	ctx context.Context,
	s *sys.System,
	args *cmd.CustomizeFlags,
	outDir config.OutputDir,
) (r *customize.Runner, err error) {
	extr, err := setupFileExtractor(ctx, s, outDir)
	if err != nil {
		return nil, fmt.Errorf("setting up file extractor: %w", err)
	}

	return &customize.Runner{
		System:        s,
		ConfigManager: setupConfigManager(s, args.ConfigDir, outDir, args.Local),
		FileExtractor: extr,
	}, nil
}

func setupConfigManager(s *sys.System, configDir string, outDir config.OutputDir, local bool) *config.Manager {
	valuesResolver := &helm.ValuesResolver{
		ValuesDir: config.Dir(configDir).HelmValuesDir(),
		FS:        s.FS(),
	}

	return config.NewManager(
		s,
		config.NewHelm(s.FS(), valuesResolver, s.Logger(), outDir.OverlaysDir()),
		config.WithDownloadFunc(http.DownloadFile),
		config.WithLocal(local),
	)
}
func setupFileExtractor(ctx context.Context, s *sys.System, outDir config.OutputDir) (extr *extractor.OCIFileExtractor, err error) {
	const isoSearchGlob = "/iso/uc-base-kernel-default-iso*.iso"

	if err := vfs.MkdirAll(s.FS(), outDir.ISOStoreDir(), vfs.DirPerm); err != nil {
		return nil, fmt.Errorf("creating ISO store directory: %w", err)
	}

	return extractor.New(
		[]string{isoSearchGlob},
		extractor.WithStore(outDir.ISOStoreDir()),
		extractor.WithFS(s.FS()),
		extractor.WithContext(ctx),
	)
}

func digestCustomizeDefinition(f vfs.FS, args *cmd.CustomizeFlags) (def *image.Definition, err error) {
	outputPath := args.OutputPath
	if outputPath == "" {
		imageName := fmt.Sprintf("image-%s.%s", time.Now().UTC().Format("2006-01-02T15-04-05"), args.MediaType)
		outputPath = filepath.Join(args.CustomizeOutput, imageName)
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
			ImageType:       args.MediaType,
			Platform:        p,
			OutputImageName: outputPath,
		},
		Configuration: conf,
	}, nil
}
