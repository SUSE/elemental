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

package config

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/template"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"go.yaml.in/yaml/v3"
)

const (
	registryScriptName   = "elemental_registry.sh"
	registryUnitName     = "elemental-registry.service"
	registryPort         = "6545"
	registryArchiveSufix = "registry.tar.zst"
	haulerBinaryName     = "hauler"
	dockerHubHost        = "docker.io"
)

var (
	//go:embed templates/elemental-registry.service.tpl
	registryUnitTpl string

	//go:embed templates/elemental_registry.sh.tpl
	registryScriptTpl string
)

type imageStore interface {
	Login(ctx context.Context, registry, username, password string) error
	SaveImage(ctx context.Context, image, platform, archive string) error
	BinaryPath() (string, error)
}


func (m *Manager) configureElementalRegistry(ctx context.Context, conf *image.Configuration, rm *resolver.ResolvedManifest, output Output, butaneCfg *butane.Config) ([]string, error) {
	logger := m.system.Logger()
	fs := m.system.FS()

	if host := m.system.Platform().String(); m.platform != host {
		return nil, fmt.Errorf("embedding a registry for platform %s is not supported on a %s host", m.platform, host)
	}

	images, err := m.collectContainerImages(rm, conf, output)
	if err != nil {
		return nil, fmt.Errorf("collecting container images: %w", err)
	}

	if len(images) == 0 {
		logger.Info("Skipping embedded artifact registry, no container images to embed")
		return nil, nil
	}

	registryDir := filepath.Join(output.OverlaysDir(), image.RegistryPath())
	if err = vfs.MkdirAll(fs, registryDir, vfs.DirPerm); err != nil {
		return nil, fmt.Errorf("creating registry directory: %w", err)
	}

	for _, registry := range conf.Kubernetes.OCIRegistry.Registries {
		if registry.Credentials == nil {
			continue
		}

		if err = m.imageStore.Login(ctx, registry.URI, registry.Credentials.Username, registry.Credentials.Password); err != nil {
			return nil, err
		}
	}

	platform := m.platform
	logger.Debug("Embedding %d container images for platform %s", len(images), platform)

	for _, img := range images {
		if err = m.embedImage(ctx, img, platform, registryDir); err != nil {
			return nil, fmt.Errorf("embedding image %s: %w", img, err)
		}
	}

	haulerBinary, err := m.imageStore.BinaryPath()
	if err != nil {
		return nil, err
	}

	haulerTarget := filepath.Join(registryDir, haulerBinaryName)
	if err = vfs.CopyFile(fs, haulerBinary, haulerTarget); err != nil {
		return nil, fmt.Errorf("copying hauler binary: %w", err)
	}

	if err = fs.Chmod(haulerTarget, 0o755); err != nil {
		return nil, fmt.Errorf("setting hauler binary permissions: %w", err)
	}

	if err = appendRegistryUnit(butaneCfg); err != nil {
		return nil, err
	}

	hosts, err := imageHostnames(images)
	if err != nil {
		return nil, fmt.Errorf("resolving image registries: %w", err)
	}

	return hosts, nil
}

func (m *Manager) embedImage(ctx context.Context, img, platform, registryDir string) error {
	fs := m.system.FS()

	archive := filepath.Join(registryDir, registryArchiveName(img))
	key := fmt.Sprintf("%s/%s/%s", haulerBinaryName, platform, img)

	m.system.Logger().Debug("Embedding container image %s", img)

	return m.cache.File(ctx, key, archive, func(ctx context.Context) (io.ReadCloser, error) {
		tempDir, err := vfs.TempDir(fs, "", "hauler-archive-")
		if err != nil {
			return nil, fmt.Errorf("creating hauler archive directory: %w", err)
		}

		tempArchive := filepath.Join(tempDir, registryArchiveName(img))
		if err = m.imageStore.SaveImage(ctx, img, platform, tempArchive); err != nil {
			_ = fs.RemoveAll(tempDir)
			return nil, err
		}

		file, err := fs.Open(tempArchive)
		if err != nil {
			_ = fs.RemoveAll(tempDir)
			return nil, fmt.Errorf("opening hauler archive: %w", err)
		}

		return &tempFile{File: file, fs: fs, dir: tempDir}, nil
	})
}

func appendRegistryUnit(butaneCfg *butane.Config) error {
	registryDir := filepath.Join("/", image.RegistryPath())
	script := filepath.Join(registryDir, registryScriptName)

	scriptValues := struct {
		RegistryDir   string
		ArchiveSuffix string
		Port          string
	}{
		RegistryDir:   registryDir,
		ArchiveSuffix: registryArchiveSufix,
		Port:          registryPort,
	}

	scriptData, err := template.Parse(registryScriptName, registryScriptTpl, &scriptValues)
	if err != nil {
		return fmt.Errorf("parsing registry script template: %w", err)
	}

	butaneCfg.AddFileInline(script, &scriptData, 0o744)

	unitValues := struct {
		RegistryDir string
		Script      string
	}{
		RegistryDir: registryDir,
		Script:      script,
	}

	unitData, err := template.Parse(registryUnitName, registryUnitTpl, &unitValues)
	if err != nil {
		return fmt.Errorf("parsing registry unit template: %w", err)
	}

	butaneCfg.AddSystemdUnit(registryUnitName, unitData, true)

	return nil
}

// collectContainerImages gathers, deduplicates, and organizes the images declared by the enabled
// release/user charts, remote/local manifests.
func (m *Manager) collectContainerImages(rm *resolver.ResolvedManifest, conf *image.Configuration, output Output) ([]string, error) {
	fs := m.system.FS()
	images := map[string]bool{}

	charts, _, err := enabledHelmCharts(rm, conf.Release.Components.HelmCharts, nil)
	if err != nil {
		return nil, fmt.Errorf("filtering enabled helm charts: %w", err)
	}

	for _, chart := range charts {
		for _, img := range chart.Images {
			images[img.Image] = true
		}
	}

	if m.helm != nil {
		for _, img := range m.helm.ContainerImages() {
			images[img] = true
		}
	}

	manifests := slices.Clone(conf.Kubernetes.LocalManifests)
	remote, err := vfs.FindFiles(fs, output.RemoteManifestsStoreDir(), "*")
	if err != nil {
		return nil, fmt.Errorf("listing downloaded manifests: %w", err)
	}
	manifests = append(manifests, remote...)

	for _, manifest := range manifests {
		resources, err := readManifest(fs, manifest)
		if err != nil {
			return nil, fmt.Errorf("reading manifest '%s': %w", manifest, err)
		}

		for _, resource := range resources {
			extractManifestImages(resource, images)
		}
	}

	for _, img := range conf.Kubernetes.OCIRegistry.ContainerImages {
		images[img.Name] = true
	}

	return slices.Sorted(maps.Keys(images)), nil
}

func readManifest(fs vfs.FS, path string) ([]map[string]any, error) {
	file, err := fs.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening manifest: %w", err)
	}
	defer func() { _ = file.Close() }()

	return parseManifests(file)
}

// parseManifests decodes a stream of YAML documents into generic resources, skipping empty ones.
func parseManifests(r io.Reader) ([]map[string]any, error) {
	var resources []map[string]any

	decoder := yaml.NewDecoder(r)
	for {
		var resource map[string]any

		if err := decoder.Decode(&resource); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, fmt.Errorf("unmarshalling manifest: %w", err)
		}

		if resource != nil {
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// extractManifestImages collects every "image" value found in workload resources.
func extractManifestImages(resource map[string]any, images map[string]bool) {
	workloadKinds := []string{"Pod", "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job", "CronJob"}

	kind, _ := resource["kind"].(string)
	if !slices.Contains(workloadKinds, kind) {
		return
	}

	var findImages func(data any)

	findImages = func(data any) {
		switch t := data.(type) {
		case map[string]any:
			for k, v := range t {
				if k == "image" {
					if img, ok := v.(string); ok {
						images[img] = true
					}
				}
				findImages(v)
			}
		case []any:
			for _, v := range t {
				findImages(v)
			}
		}
	}

	findImages(resource)
}

// imageHostnames returns the sorted set of registries the images are served from. Docker Hub
// is always included since unqualified references resolve to it.
func imageHostnames(images []string) ([]string, error) {
	hosts := map[string]bool{dockerHubHost: true}

	for _, img := range images {
		ref, err := name.ParseReference(img)
		if err != nil {
			return nil, fmt.Errorf("parsing image reference '%s': %w", img, err)
		}

		host := ref.Context().RegistryStr()
		if host == name.DefaultRegistry {
			continue
		}

		hosts[host] = true
	}

	return slices.Sorted(maps.Keys(hosts)), nil
}

func registryArchiveName(img string) string {
	return fmt.Sprintf("%s-%s", strings.ReplaceAll(img, "/", "_"), registryArchiveSufix)
}
