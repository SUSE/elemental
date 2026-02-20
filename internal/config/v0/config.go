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

package v0

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/manifest/source"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type Dir string

func (dir Dir) InstallFilepath() string {
	return filepath.Join(string(dir), "install.yaml")
}

func (dir Dir) ReleaseFilepath() string {
	return filepath.Join(string(dir), "release.yaml")
}

func (dir Dir) ClusterFilepath() string {
	return filepath.Join(dir.kubernetesDir(), "cluster.yaml")
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

func (dir Dir) KubernetesAgentFilepath() string {
	return filepath.Join(dir.KubernetesConfigDir(), "agent.yaml")
}

func (dir Dir) KubernetesServerFilepath() string {
	return filepath.Join(dir.KubernetesConfigDir(), "server.yaml")
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

func (dir Dir) CustomDir() string {
	return filepath.Join(string(dir), "custom")
}

func Parse(f vfs.FS, configDir Dir) (conf *image.Configuration, err error) {
	conf = &image.Configuration{}

	data, err := f.ReadFile(configDir.InstallFilepath())
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err = ParseAny(data, &conf.Installation); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", configDir.InstallFilepath(), err)
	}

	data, err = f.ReadFile(configDir.ReleaseFilepath())
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err = ParseAny(data, &conf.Release); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", configDir.ReleaseFilepath(), err)
	}

	if err = sanitizeManifestURI(&conf.Release, string(configDir)); err != nil {
		return nil, fmt.Errorf("updating manifest URI: %w", err)
	}

	if err = parseKubernetesDir(f, configDir, &conf.Kubernetes, &conf.Release); err != nil {
		return nil, fmt.Errorf("parsing kubernetes configuration: %w", err)
	}

	if err = parseNetworkDir(f, configDir, &conf.Network); err != nil {
		return nil, fmt.Errorf("parsing network directory: %w", err)
	}

	if err = parseCustomDir(f, configDir, &conf.Custom); err != nil {
		return nil, fmt.Errorf("parsing custom directory: %w", err)
	}

	data, err = f.ReadFile(configDir.ButaneFilepath())
	if err == nil {
		if err = ParseAny(data, &conf.ButaneConfig); err != nil {
			return nil, fmt.Errorf("parsing config file %q: %w", configDir.ButaneFilepath(), err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err = Validate(conf); err != nil {
		return nil, fmt.Errorf("validating configuration: %w", err)
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

	data, err := f.ReadFile(configDir.ClusterFilepath())
	if err == nil {
		if err = ParseAny(data, k); err != nil {
			return fmt.Errorf("parsing config file %q: %w", configDir.ClusterFilepath(), err)
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

	return nil
}

func parseKubernetesDir(f vfs.FS, configDir Dir, k *kubernetes.Kubernetes, r *release.Release) error {
	entries, err := f.ReadDir(configDir.KubernetesManifestsDir())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading %s: %w", configDir.KubernetesManifestsDir(), err)
	}

	for _, entry := range entries {
		localManifestPath := filepath.Join(configDir.KubernetesManifestsDir(), entry.Name())
		k.LocalManifests = append(k.LocalManifests, localManifestPath)
	}

	k.Config = kubernetes.Config{}

	serverYamlPath := configDir.KubernetesServerFilepath()
	if exists, _ := vfs.Exists(f, serverYamlPath); exists {
		k.Config.ServerFilePath = serverYamlPath
	}

	agentYamlPath := configDir.KubernetesAgentFilepath()
	if exists, _ := vfs.Exists(f, agentYamlPath); exists {
		k.Config.AgentFilePath = agentYamlPath
	}

	return parseKubernetes(f, configDir, k, r)
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

func parseCustomDir(f vfs.FS, configDir Dir, c *image.Custom) error {
	const (
		scriptsPath = "scripts"
		filesPath   = "files"
	)

	validateDir := func(path string) error {
		entries, err := f.ReadDir(path)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			return fmt.Errorf("directory %q is empty", path)
		}

		return nil
	}

	customDir := configDir.CustomDir()
	if err := validateDir(customDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Not configured.
			return nil
		}

		return err
	}

	scriptsDir := filepath.Join(customDir, scriptsPath)
	if err := validateDir(scriptsDir); err != nil {
		return err
	}
	c.ScriptsDir = scriptsDir

	filesDir := filepath.Join(customDir, filesPath)
	if err := validateDir(filesDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Not configured.
			return nil
		}
		return err
	}
	c.FilesDir = filesDir

	return nil
}

func ParseAny(data []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	return decoder.Decode(target)
}
