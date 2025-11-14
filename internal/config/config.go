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

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/manifest/source"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"go.yaml.in/yaml/v3"
)

type Dir string

func (dir Dir) InstallFilepath() string {
	return filepath.Join(string(dir), "install.yaml")
}

func (dir Dir) ReleaseFilepath() string {
	return filepath.Join(string(dir), "release.yaml")
}

func (dir Dir) KubernetesFilepath() string {
	return filepath.Join(string(dir), "kubernetes.yaml")
}

func (dir Dir) ButaneFilepath() string {
	return filepath.Join(string(dir), "butane.yaml")
}

func (dir Dir) kubernetesDir() string {
	return filepath.Join(string(dir), "kubernetes")
}

func (dir Dir) KubernetesConfigDir() string {
	return filepath.Join(dir.kubernetesDir(), "config")
}

func (dir Dir) KubernetesManifestsDir() string {
	return filepath.Join(dir.kubernetesDir(), "manifests")
}

func (dir Dir) HelmValuesDir() string {
	return filepath.Join(dir.kubernetesDir(), "helm", "values")
}

func (dir Dir) NetworkDir() string {
	return filepath.Join(string(dir), "network")
}

type OutputDir string

func (dir OutputDir) OverlaysDir() string {
	return filepath.Join(string(dir), "overlays")
}

func (dir OutputDir) FirstbootConfigDir() string {
	return filepath.Join(dir.OverlaysDir(), deployment.ConfigMnt)
}

func (dir OutputDir) CatalystConfigDir() string {
	return filepath.Join(dir.OverlaysDir(), deployment.ConfigMnt, "catalyst")
}

func (dir OutputDir) ReleaseManifestsDir() string {
	return filepath.Join(string(dir), "release-manifests")
}

func Parse(f vfs.FS, configDir Dir) (conf *image.Configuration, err error) {
	conf = &image.Configuration{}

	data, err := f.ReadFile(configDir.InstallFilepath())
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err = parseAny(data, &conf.Installation); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", configDir.InstallFilepath(), err)
	}

	data, err = f.ReadFile(configDir.ReleaseFilepath())
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err = parseAny(data, &conf.Release); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", configDir.ReleaseFilepath(), err)
	}

	if err = sanitizeManifestURI(&conf.Release, string(configDir)); err != nil {
		return nil, fmt.Errorf("updating manifest URI: %w", err)
	}

	if err = parseKubernetes(f, configDir, &conf.Kubernetes, &conf.Release); err != nil {
		return nil, fmt.Errorf("parsing kubernetes configuration: %w", err)
	}

	if err = parseNetworkDir(f, configDir, &conf.Network); err != nil {
		return nil, fmt.Errorf("parsing network directory: %w", err)
	}

	data, err = f.ReadFile(configDir.ButaneFilepath())
	if err == nil {
		if err = parseAny(data, &conf.ButaneConfig); err != nil {
			return nil, fmt.Errorf("parsing config file %q: %w", configDir.ButaneFilepath(), err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	return conf, nil
}

func sanitizeManifestURI(r *release.Release, configDir string) error {
	fileSource := fmt.Sprintf("%s://", source.File.String())
	if !strings.HasPrefix(r.ManifestURI, fileSource) {
		return nil
	}

	absConfDir, err := filepath.Abs(configDir)
	if err != nil {
		return fmt.Errorf("calculate absolute directory: %w", err)
	}

	r.ManifestURI = filepath.Join(fileSource, absConfDir, strings.TrimPrefix(r.ManifestURI, fileSource))
	return nil
}

func parseKubernetes(f vfs.FS, configDir Dir, k *kubernetes.Kubernetes, r *release.Release) error {
	const (
		MetalLB                = "metallb"
		EndpointCopierOperator = "endpoint-copier-operator"
	)

	data, err := f.ReadFile(configDir.KubernetesFilepath())
	if err == nil {
		if err = parseAny(data, k); err != nil {
			return fmt.Errorf("parsing config file %q: %w", configDir.KubernetesFilepath(), err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading config file: %w", err)
	}

	if k.Network.APIVIP4 != "" || k.Network.APIVIP6 != "" {
		containsChart := func(name string) bool {
			return slices.ContainsFunc(r.Components.HelmCharts, func(c release.HelmChart) bool {
				return c.Name == name
			})
		}

		if !containsChart(MetalLB) {
			r.Components.HelmCharts = append(r.Components.HelmCharts, release.HelmChart{Name: MetalLB})
		}

		if !containsChart(EndpointCopierOperator) {
			r.Components.HelmCharts = append(r.Components.HelmCharts, release.HelmChart{Name: EndpointCopierOperator})
		}
	}

	return parseKubernetesDir(f, configDir, k)
}

func parseKubernetesDir(f vfs.FS, configDir Dir, k *kubernetes.Kubernetes) error {
	entries, err := f.ReadDir(configDir.KubernetesManifestsDir())
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

	k.Config = kubernetes.Config{}

	serverYamlPath := filepath.Join(configDir.KubernetesConfigDir(), "server.yaml")
	if exists, _ := vfs.Exists(f, serverYamlPath); exists {
		k.Config.ServerFilePath = serverYamlPath
	}

	agentYamlPath := filepath.Join(configDir.KubernetesConfigDir(), "agent.yaml")
	if exists, _ := vfs.Exists(f, agentYamlPath); exists {
		k.Config.AgentFilePath = agentYamlPath
	}

	return nil
}

func parseNetworkDir(f vfs.FS, configDir Dir, n *image.Network) error {
	const networkCustomScriptName = "configure-network.sh"

	networkDir := configDir.NetworkDir()

	entries, err := f.ReadDir(networkDir)
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

func parseAny(data []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	return decoder.Decode(target)
}
