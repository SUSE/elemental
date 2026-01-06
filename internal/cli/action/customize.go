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

package action

import (
	"context"
	"fmt"
	"os/signal"
	"path/filepath"
	"strings"
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

func Customize(ctx *cli.Context) error {
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	system := ctx.App.Metadata["system"].(*sys.System)
	logger := system.Logger()
	fs := system.FS()
	args := &cmd.CustomizeArgs

	logger.Info("Customizing image started")

	imagePath, configPath := resolveOutputPaths(args)

	output, err := config.NewOutput(fs, "", configPath)
	if err != nil {
		logger.Error("Creating working directory failed")
		return err
	}

	defer func() {
		logger.Debug("Cleaning up working directory")
		if rmErr := output.Cleanup(fs); rmErr != nil {
			logger.Error("Cleaning up working directory failed: %v", rmErr)
		}
	}()

	def, err := digestCustomizeDefinition(fs, args, imagePath)
	if err != nil {
		logger.Error("Digesting image definition from customize flags failed")
		return err
	}

	ctxCancel, cancelFunc := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer cancelFunc()

	customizeRunner, err := setupCustomizeRunner(ctxCancel, system, args, output)
	if err != nil {
		logger.Error("Setting up customization runner failed")
		return err
	}

	if err = customizeRunner.Run(ctxCancel, def, output, args.Local); err != nil {
		logger.Error("Customizing installer media failed")
		return err
	}

	return nil
}

func resolveOutputPaths(args *cmd.CustomizeFlags) (imagePath, configPath string) {
	imagePath = args.OutputPath
	if imagePath == "" {
		timestamp := time.Now().UTC().Format("2006-01-02T15-04-05")
		imageName := fmt.Sprintf("image-%s.%s", timestamp, args.MediaType)

		imagePath = filepath.Join(args.ConfigDir, imageName)
	}

	if args.Mode == "split" {
		outputDir := filepath.Dir(imagePath)
		filename := filepath.Base(imagePath)
		baseName := strings.TrimSuffix(filename, filepath.Ext(filename))

		configPath = filepath.Join(outputDir, baseName+"-config")
	}

	return imagePath, configPath
}

func setupCustomizeRunner(
	ctx context.Context,
	s *sys.System,
	args *cmd.CustomizeFlags,
	output config.Output,
) (*customize.Runner, error) {
	extr, err := setupFileExtractor(ctx, s, output)
	if err != nil {
		return nil, fmt.Errorf("setting up file extractor: %w", err)
	}

	return &customize.Runner{
		System:        s,
		ConfigManager: setupConfigManager(s, args.ConfigDir, output, args.Local),
		FileExtractor: extr,
	}, nil
}

func setupConfigManager(s *sys.System, configDir string, output config.Output, local bool) *config.Manager {
	valuesResolver := &helm.ValuesResolver{
		ValuesDir: config.Dir(configDir).HelmValuesDir(),
		FS:        s.FS(),
	}

	return config.NewManager(
		s,
		config.NewHelm(s.FS(), valuesResolver, s.Logger(), output.OverlaysDir()),
		config.WithDownloadFunc(http.DownloadFile),
		config.WithLocal(local),
	)
}
func setupFileExtractor(ctx context.Context, s *sys.System, outDir config.Output) (extr *extractor.OCIFileExtractor, err error) {
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

func digestCustomizeDefinition(f vfs.FS, args *cmd.CustomizeFlags, imagePath string) (*image.Definition, error) {
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
			OutputImageName: imagePath,
		},
		Configuration: conf,
	}, nil
}
