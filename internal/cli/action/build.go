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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/suse/elemental/v3/internal/build"
	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/pkg/helm"
	"github.com/suse/elemental/v3/pkg/http"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/platform"
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
	if err := validateArgs(args); err != nil {
		logger.Error("Input args are invalid")
		return err
	}

	logger.Info("Reading image configuration")
	definition, err := parseImageDefinition(args)
	if err != nil {
		logger.Error("Parsing image configuration failed")
		return err
	}

	logger.Info("Validated image configuration")

	buildDir, err := createBuildDir(args.BuildDir)
	if err != nil {
		logger.Error("Creating build directory failed")
		return err
	}

	configDir := image.ConfigDir(args.ConfigDir)

	valuesResolver := &helm.ValuesResolver{
		ValuesDir: configDir.HelmValuesDir(),
		FS:        system.FS(),
	}

	builder := &build.Builder{
		System:       system,
		Helm:         build.NewHelm(system.FS(), valuesResolver, logger, buildDir.OverlaysDir()),
		DownloadFile: http.DownloadFile,
		Local:        args.Local,
	}

	logger.Info("Starting build process for %s %s image", definition.Image.Platform.String(), definition.Image.ImageType)
	if err = builder.Run(ctxCancel, definition, buildDir); err != nil {
		logger.Error("Build process failed")
		return err
	}

	logger.Info("Build process complete")
	return nil
}

func validateArgs(args *cmd.BuildFlags) error {
	_, err := os.Stat(args.ConfigDir)
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

func parseImageDefinition(args *cmd.BuildFlags) (*image.Definition, error) {
	outputPath := args.OutputPath
	if outputPath == "" {
		imageName := fmt.Sprintf("image-%s.%s", time.Now().UTC().Format("2006-01-02T15-04-05"), args.ImageType)
		outputPath = filepath.Join(args.BuildDir, imageName)
	}

	p, err := platform.Parse(args.Platform)
	if err != nil {
		return nil, fmt.Errorf("error parsing platform %s", args.Platform)
	}

	definition := &image.Definition{
		Image: image.Image{
			ImageType:       args.ImageType,
			Platform:        p,
			OutputImageName: outputPath,
		},
	}

	configDir := image.ConfigDir(args.ConfigDir)

	data, err := os.ReadFile(configDir.OSFilepath())
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err = image.ParseConfig(data, &definition.OperatingSystem); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", configDir.OSFilepath(), err)
	}

	data, err = os.ReadFile(configDir.InstallFilepath())
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err = image.ParseConfig(data, &definition.Installation); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", configDir.InstallFilepath(), err)
	}

	data, err = os.ReadFile(configDir.ReleaseFilepath())
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err = image.ParseConfig(data, &definition.Release); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", configDir.ReleaseFilepath(), err)
	}

	data, err = os.ReadFile(configDir.KubernetesFilepath())
	if err == nil {
		if err = image.ParseConfig(data, &definition.Kubernetes); err != nil {
			return nil, fmt.Errorf("parsing config file %q: %w", configDir.KubernetesFilepath(), err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err = parseKubernetesDir(configDir, &definition.Kubernetes); err != nil {
		return nil, fmt.Errorf("parsing local kubernetes directory: %w", err)
	}

	if err = parseNetworkDir(configDir, &definition.Network); err != nil {
		return nil, fmt.Errorf("parsing network directory: %w", err)
	}

	return definition, nil
}

func createBuildDir(rootBuildDir string) (image.BuildDir, error) {
	buildDirName := fmt.Sprintf("build-%s", time.Now().UTC().Format("2006-01-02T15-04-05"))
	buildDirPath := filepath.Join(rootBuildDir, buildDirName)
	return image.BuildDir(buildDirPath), os.MkdirAll(buildDirPath, 0700)
}

func parseKubernetesDir(configDir image.ConfigDir, k *kubernetes.Kubernetes) error {
	entries, err := os.ReadDir(configDir.KubernetesManifestsDir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", configDir.KubernetesManifestsDir(), err)
	}

	for _, entry := range entries {
		localManifestPath := filepath.Join(configDir.KubernetesManifestsDir(), entry.Name())
		k.LocalManifests = append(k.LocalManifests, localManifestPath)
	}

	return nil
}

func parseNetworkDir(configDir image.ConfigDir, n *image.Network) error {
	const networkCustomScriptName = "configure-network.sh"

	networkDir := configDir.NetworkDir()

	entries, err := os.ReadDir(networkDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Not configured.
			return nil
		}

		return fmt.Errorf("reading network directory: %w", err)
	}

	switch len(entries) {
	case 0:
		return fmt.Errorf("network directory is empty")
	case 1:
		if entries[0].Name() == networkCustomScriptName {
			n.CustomScript = filepath.Join(networkDir, networkCustomScriptName)
			return nil
		}
		fallthrough
	default:
		n.ConfigDir = networkDir
	}

	return nil
}
