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
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/suse/elemental/v3/internal/helm"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"gopkg.in/yaml.v3"
)

func needsHelmChartsSetup(def *image.Definition, rm *resolver.ResolvedManifest) bool {
	return (rm.CorePlatform != nil && rm.CorePlatform.Components.Helm != nil) ||
		(rm.ProductExtension != nil && rm.ProductExtension.Components.Helm != nil && len(def.Release.Enable) != 0) ||
		def.Kubernetes.Helm != nil
}

func setupHelmCharts(d *image.Definition, rm *resolver.ResolvedManifest, overlaysPath, relativeHelmPath string) (runtimeHelmCharts []string, err error) {
	pathInOverlays := filepath.Join(overlaysPath, relativeHelmPath)
	runtimePath := filepath.Join(string(os.PathSeparator), relativeHelmPath)

	configs, err := getPrioritisedHelmConfigs(d, rm)
	if err != nil {
		return nil, fmt.Errorf("prioritizing helm charts: %w", err)
	}

	chartNames, err := writeHelmCharts(pathInOverlays, configs)
	if err != nil {
		return nil, fmt.Errorf("writing helm chart resources to %s: %w", pathInOverlays, err)
	}

	for _, chartName := range chartNames {
		runtimeHelmCharts = append(runtimeHelmCharts, filepath.Join(runtimePath, chartName))
	}

	return runtimeHelmCharts, nil
}

func writeHelmCharts(dest string, configs []*api.Helm) (names []string, err error) {
	if err = os.MkdirAll(dest, os.ModeDir); err != nil {
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

func getPrioritisedHelmConfigs(def *image.Definition, rm *resolver.ResolvedManifest) ([]*api.Helm, error) {
	var configs []*api.Helm

	if rm.CorePlatform != nil && rm.CorePlatform.Components.Helm != nil {
		configs = append(configs, rm.CorePlatform.Components.Helm)
	}

	if rm.ProductExtension != nil && rm.ProductExtension.Components.Helm != nil && len(def.Release.Enable) != 0 {
		enabledCharts, err := enabledHelmCharts(rm.ProductExtension.Components.Helm, def.Release.Enable)
		if err != nil {
			return nil, fmt.Errorf("filtering enabled helm charts: %w", err)
		}
		configs = append(configs, enabledCharts)
	}

	if def.Kubernetes.Helm != nil {
		configs = append(configs, def.Kubernetes.Helm)
	}

	return configs, nil
}

func enabledHelmCharts(helm *api.Helm, enabled []string) (*api.Helm, error) {
	h := &api.Helm{Repositories: helm.Repositories}

	allCharts := map[string]*api.HelmChart{}
	for _, c := range helm.Charts {
		allCharts[c.Chart] = c
	}

	var addChart func(name string) error

	// Add a chart and its direct dependencies, avoiding duplicates.
	addChart = func(name string) error {
		chart, ok := allCharts[name]
		if !ok {
			return fmt.Errorf("helm chart does not exist")
		}

		if slices.ContainsFunc(h.Charts, func(c *api.HelmChart) bool {
			return c.Chart == name
		}) {
			return nil
		}

		// Check for dependencies and add them first.
		for _, d := range chart.DependsOn {
			if err := addChart(d); err != nil {
				return fmt.Errorf("adding dependent helm chart '%s': %w", d, err)
			}
		}

		// Add the main chart.
		h.Charts = append(h.Charts, chart)

		return nil
	}

	for _, e := range enabled {
		if err := addChart(e); err != nil {
			return nil, fmt.Errorf("adding helm chart '%s': %w", e, err)
		}
	}

	return h, nil
}
