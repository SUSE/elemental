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

package build

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/suse/elemental/v3/internal/helm"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/manifest/extractor"
	"github.com/suse/elemental/v3/internal/template"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/install"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/manifest/source"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/upgrade"
	"gopkg.in/yaml.v3"
)

//go:embed templates/config.sh.tpl
var configScriptTpl string

//go:embed templates/k8s_res_deploy.sh.tpl
var k8sResDeployScriptTpl string

func Run(ctx context.Context, d *image.Definition, buildDir string, system *sys.System) error {
	logger := system.Logger()
	runner := system.Runner()
	overlaysPath := filepath.Join(buildDir, "overlays")

	logger.Info("Resolving release manifest: %s", d.Release.ManifestURI)
	m, err := resolveManifest(d.Release.ManifestURI, buildDir)
	if err != nil {
		logger.Error("Resolving release manifest failed")
		return err
	}

	var runtimeHelmCharts []string
	relativeK8sPath := filepath.Join("var", "lib", "elemental", "kubernetes")
	if needsHelmChartsSetup(&d.Kubernetes, m) {
		relativeHelmPath := filepath.Join(relativeK8sPath, "helm")
		if runtimeHelmCharts, err = setupHelmCharts(d, m, overlaysPath, relativeHelmPath); err != nil {
			logger.Error("Setting up HelmChart resoruces")
			return err
		}
	}

	var runtimeManifestsDir string
	if needsManifestsSetup(&d.Kubernetes) {
		relativeManfiestsPath := filepath.Join(relativeK8sPath, "manifests")
		manifestsOverlayPath := filepath.Join(overlaysPath, relativeManfiestsPath)
		if err = setupManifests(ctx, system.FS(), &d.Kubernetes, manifestsOverlayPath); err != nil {
			logger.Error("Setting up Kubernetes manifests")
			return err
		}

		runtimeManifestsDir = filepath.Join(string(os.PathSeparator), relativeManfiestsPath)
	}

	var runtimeK8sResDeployScript string
	if len(runtimeHelmCharts) > 0 || runtimeManifestsDir != "" {
		kubernetesOverlayPath := filepath.Join(overlaysPath, relativeK8sPath)
		scriptInOverlay, err := writeK8sResDeployScript(kubernetesOverlayPath, runtimeManifestsDir, runtimeHelmCharts)
		if err != nil {
			logger.Error("Writing Kubernetes resource deployment script")
			return err
		}
		runtimeK8sResDeployScript = filepath.Join(string(os.PathSeparator), relativeK8sPath, filepath.Base(scriptInOverlay))
	}

	logger.Info("Downloading RKE2 extension")
	extensionsPath := filepath.Join(overlaysPath, "var", "lib", "extensions")
	if err = downloadExtension(ctx, m.CorePlatform.Components.Kubernetes.RKE2.Image, extensionsPath); err != nil {
		logger.Error("Downloading RKE2 extension failed")
		return err
	}

	logger.Info("Preparing configuration script")
	configScript, err := writeConfigScript(d, buildDir, runtimeK8sResDeployScript)
	if err != nil {
		logger.Error("Preparing configuration script failed")
		return err
	}

	logger.Info("Creating RAW disk image")
	if err = createDisk(runner, d.Image, d.OperatingSystem.DiskSize); err != nil {
		logger.Error("Creating RAW disk image failed")
		return err
	}

	logger.Info("Attaching loop device to RAW disk image")
	device, err := attachDevice(runner, d.Image)
	if err != nil {
		logger.Error("Attaching loop device failed")
		return err
	}
	defer func() {
		if dErr := detachDevice(runner, device); dErr != nil {
			logger.Error("Detaching loop device failed: %v", dErr)
		}
	}()

	logger.Info("Preparing installation setup")
	dep, err := newDeployment(
		system,
		device,
		d.Installation.Bootloader,
		d.Installation.KernelCmdLine,
		m.CorePlatform.Components.OperatingSystem.Image,
		configScript,
		overlaysPath,
	)
	if err != nil {
		logger.Error("Preparing installation setup failed")
		return err
	}

	boot, err := bootloader.New(dep.BootConfig.Bootloader, system)
	if err != nil {
		logger.Error("Parsing boot config failed")
		return err
	}

	manager := firmware.NewEfiBootManager(system)
	upgrader := upgrade.New(ctx, system, upgrade.WithBootManager(manager), upgrade.WithBootloader(boot))
	installer := install.New(ctx, system, install.WithUpgrader(upgrader))

	logger.Info("Installing OS")
	if err = installer.Install(dep); err != nil {
		logger.Error("Installation failed")
		return err
	}

	logger.Info("Installation complete")

	return nil
}

func newDeployment(system *sys.System, installationDevice, bootloader, kernelCmdLine, osImage, configScript, overlaysPath string) (*deployment.Deployment, error) {
	d := deployment.DefaultDeployment()
	d.Disks[0].Device = installationDevice
	d.BootConfig.Bootloader = bootloader
	d.BootConfig.KernelCmdline = kernelCmdLine
	d.CfgScript = configScript

	osURI := fmt.Sprintf("%s://%s", deployment.OCI, osImage)
	osSource, err := deployment.NewSrcFromURI(osURI)
	if err != nil {
		return nil, fmt.Errorf("parsing OS source URI %q: %w", osURI, err)
	}
	d.SourceOS = osSource

	overlaysURI := fmt.Sprintf("%s://%s", deployment.Dir, overlaysPath)
	overlaySource, err := deployment.NewSrcFromURI(overlaysURI)
	if err != nil {
		return nil, fmt.Errorf("parsing overlay source URI %q: %w", overlaysURI, err)
	}
	d.OverlayTree = overlaySource

	if err = d.Sanitize(system); err != nil {
		return nil, fmt.Errorf("sanitizing deployment: %w", err)
	}

	return d, nil
}

func resolveManifest(manifestURI, storeDir string) (*resolver.ResolvedManifest, error) {
	manifestStore := filepath.Join(storeDir, "release-manifests")
	if err := os.MkdirAll(manifestStore, 0700); err != nil {
		return nil, fmt.Errorf("creating release manifest store '%s': %w", manifestStore, err)
	}

	extr, err := extractor.New(extractor.WithStore(manifestStore))
	if err != nil {
		return nil, fmt.Errorf("initialising OCI release manifest extractor: %w", err)
	}

	res := resolver.New(source.NewReader(extr))
	m, err := res.Resolve(manifestURI)
	if err != nil {
		return nil, fmt.Errorf("resolving manifest at uri '%s': %w", manifestURI, err)
	}

	return m, nil
}

func writeConfigScript(d *image.Definition, dest, runtimeK8sResDeployScript string) (string, error) {
	const configScriptName = "config.sh"

	values := struct {
		Users                []image.User
		KubernetesDir        string
		ManifestDeployScript string
	}{
		Users: d.OperatingSystem.Users,
	}

	if runtimeK8sResDeployScript != "" {
		values.KubernetesDir = filepath.Dir(runtimeK8sResDeployScript)
		values.ManifestDeployScript = runtimeK8sResDeployScript
	}

	data, err := template.Parse(configScriptName, configScriptTpl, &values)
	if err != nil {
		return "", fmt.Errorf("parsing config script template: %w", err)
	}

	filename := filepath.Join(dest, configScriptName)
	if err = os.WriteFile(filename, []byte(data), os.FileMode(0o744)); err != nil {
		return "", fmt.Errorf("writing config script: %w", err)
	}
	return filename, nil
}

func createDisk(runner sys.Runner, img image.Image, diskSize image.DiskSize) error {
	const defaultSize = "10G"

	if diskSize == "" {
		diskSize = defaultSize
	} else if !diskSize.IsValid() {
		return fmt.Errorf("invalid disk size definition '%s'", diskSize)
	}

	_, err := runner.Run("qemu-img", "create", "-f", "raw", img.OutputImageName, string(diskSize))
	return err
}

func attachDevice(runner sys.Runner, img image.Image) (string, error) {
	out, err := runner.Run("losetup", "-f", "--show", img.OutputImageName)
	if err != nil {
		return "", err
	}

	device := strings.TrimSpace(string(out))
	return device, nil
}

func detachDevice(runner sys.Runner, device string) error {
	_, err := runner.Run("losetup", "-d", device)
	return err
}

func downloadExtension(ctx context.Context, downloadURL, extensionsPath string) error {
	if err := os.MkdirAll(extensionsPath, 0700); err != nil {
		return fmt.Errorf("setting up extensions directory '%s': %w", extensionsPath, err)
	}

	parsedURL, err := url.Parse(downloadURL)
	if err != nil {
		return fmt.Errorf("invalid url '%s': %w", downloadURL, err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("creating HTTP request: %w", err)
	}

	httpClient := &http.Client{Timeout: 90 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading file returned unexpected status code: %d", resp.StatusCode)
	}

	fileName := filepath.Base(parsedURL.Path)
	output := filepath.Join(extensionsPath, fileName)

	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("creating file %q: %w", output, err)
	}

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("copying file contents: %w", err)
	}

	if err = file.Close(); err != nil {
		return fmt.Errorf("closing file: %w", err)
	}

	return nil
}

func needsHelmChartsSetup(k *image.Kubernetes, rm *resolver.ResolvedManifest) bool {
	return (rm.CorePlatform != nil && rm.CorePlatform.Components.Helm != nil) ||
		(rm.ProductExtension != nil && rm.ProductExtension.Components.Helm != nil) || k.Helm != nil
}

func needsManifestsSetup(k *image.Kubernetes) bool {
	return len(k.RemoteManifests) > 0 || len(k.LocalManifest) > 0
}

func setupHelmCharts(d *image.Definition, rm *resolver.ResolvedManifest, overlaysPath, relativeHelmPath string) (runtimeHelmCharts []string, err error) {
	pathInOverlays := filepath.Join(overlaysPath, relativeHelmPath)
	runtimePath := filepath.Join(string(os.PathSeparator), relativeHelmPath)
	chartNames, err := writeHelmCharts(pathInOverlays, getPrioritisedHelmConfigs(&d.Kubernetes, rm))
	if err != nil {
		return nil, fmt.Errorf("writing helm chart resources to %s: %w", pathInOverlays, err)
	}

	for _, chartName := range chartNames {
		runtimeHelmCharts = append(runtimeHelmCharts, filepath.Join(runtimePath, chartName))
	}

	return runtimeHelmCharts, nil
}

func getPrioritisedHelmConfigs(k *image.Kubernetes, rm *resolver.ResolvedManifest) []*api.Helm {
	configs := []*api.Helm{}
	if rm.CorePlatform != nil && rm.CorePlatform.Components.Helm != nil {
		configs = append(configs, rm.CorePlatform.Components.Helm)
	}

	if rm.ProductExtension != nil && rm.ProductExtension.Components.Helm != nil {
		configs = append(configs, rm.ProductExtension.Components.Helm)
	}

	if k.Helm != nil {
		configs = append(configs, k.Helm)
	}

	return configs
}

func writeHelmCharts(dest string, configs []*api.Helm) (names []string, err error) {
	if err := os.MkdirAll(dest, os.ModeDir); err != nil {
		return nil, fmt.Errorf("setting up HelmChart destination directory '%s': %w", dest, err)
	}

	for _, config := range configs {
		for _, helmCRD := range helm.ProduceCRDs(config) {
			data, err := yaml.Marshal(helmCRD)
			if err != nil {
				return nil, fmt.Errorf("marshaling helm chart: %w", err)
			}

			chartName := fmt.Sprintf("%s.yaml", helmCRD.Metadata.Name)
			chartPath := filepath.Join(dest, chartName)
			if err = os.WriteFile(chartPath, data, os.FileMode(0o644)); err != nil {
				return nil, fmt.Errorf("writing helm chart: %w", err)
			}

			names = append(names, chartName)
		}
	}

	return names, nil
}

func setupManifests(ctx context.Context, fs vfs.FS, k *image.Kubernetes, manifestsDir string) error {
	if err := os.MkdirAll(manifestsDir, os.ModeDir); err != nil {
		return fmt.Errorf("setting up manifests directory '%s': %w", manifestsDir, err)
	}

	for _, manifest := range k.RemoteManifests {
		if err := downloadExtension(ctx, manifest, manifestsDir); err != nil {
			return fmt.Errorf("downloading remote Kubernetes manfiest '%s': %w", manifest, err)
		}
	}

	for _, manifest := range k.LocalManifest {
		overlayPath := filepath.Join(manifestsDir, filepath.Base(manifest))
		if err := vfs.CopyFile(fs, manifest, overlayPath); err != nil {
			return fmt.Errorf("copying local manifest '%s' to '%s': %w", manifest, overlayPath, err)
		}
	}

	return nil
}

func writeK8sResDeployScript(dest, runtimeManifestsDir string, runtimeHelmCharts []string) (path string, err error) {
	const k8sResDeployScriptName = "k8s_res_deploy.sh"

	values := struct {
		HelmCharts   []string
		ManifestsDir string
	}{
		HelmCharts:   runtimeHelmCharts,
		ManifestsDir: runtimeManifestsDir,
	}

	data, err := template.Parse(k8sResDeployScriptName, k8sResDeployScriptTpl, &values)
	if err != nil {
		return "", fmt.Errorf("parsing template for %s: %w", k8sResDeployScriptName, err)
	}

	filename := filepath.Join(dest, k8sResDeployScriptName)
	if err := os.WriteFile(filename, []byte(data), os.FileMode(0o744)); err != nil {
		return "", fmt.Errorf("writing %s: %w", filename, err)
	}
	return filename, nil
}
