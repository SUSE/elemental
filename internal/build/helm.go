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
	"path/filepath"
	"slices"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/helm"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	yaml "sigs.k8s.io/yaml/goyaml.v3"
)

type helmValuesResolver interface {
	Resolve(*helm.ValueSource) ([]byte, error)
}

type helmChart interface {
	GetName() string
	GetInlineValues() map[string]any
	GetRepositoryName() string
	ToCRD(values []byte, repository string) *helm.CRD
}

func needsHelmChartsSetup(def *image.Definition) bool {
	return len(def.Release.Core.Helm) > 0 || len(def.Release.Product.Helm) > 0 || def.Kubernetes.Helm != nil
}

func setupHelmCharts(fs vfs.FS, def *image.Definition, rm *resolver.ResolvedManifest, overlaysPath, helmPath string, valuesResolver helmValuesResolver) (runtimeHelmCharts []string, err error) {
	charts, err := retrieveHelmCharts(rm, def, valuesResolver)
	if err != nil {
		return nil, fmt.Errorf("retrieving helm charts: %w", err)
	}

	helmDestPath := filepath.Join(overlaysPath, helmPath)

	chartNames, err := writeHelmCharts(fs, charts, helmDestPath)
	if err != nil {
		return nil, fmt.Errorf("writing helm chart resources: %w", err)
	}

	for _, chartName := range chartNames {
		runtimeHelmCharts = append(runtimeHelmCharts, filepath.Join("/", helmPath, chartName))
	}

	return runtimeHelmCharts, nil
}

func writeHelmCharts(fs vfs.FS, crds []*helm.CRD, destDir string) (names []string, err error) {
	if err = vfs.MkdirAll(fs, destDir, vfs.DirPerm); err != nil {
		return nil, fmt.Errorf("creating directory: %w", err)
	}

	for _, crd := range crds {
		data, err := yaml.Marshal(crd)
		if err != nil {
			return nil, fmt.Errorf("marshaling helm chart %s: %w", crd.Metadata.Name, err)
		}

		chartName := fmt.Sprintf("%s.yaml", crd.Metadata.Name)
		chartPath := filepath.Join(destDir, chartName)
		if err = fs.WriteFile(chartPath, data, 0o644); err != nil {
			return nil, fmt.Errorf("writing helm chart: %w", err)
		}

		names = append(names, chartName)
	}

	return names, nil
}

func retrieveHelmCharts(rm *resolver.ResolvedManifest, def *image.Definition, valuesResolver helmValuesResolver) ([]*helm.CRD, error) {
	var crds []*helm.CRD

	if rm.CorePlatform != nil && rm.CorePlatform.Components.Helm != nil && len(def.Release.Core.Helm) > 0 {
		charts, err := enabledHelmCharts(rm.CorePlatform.Components.Helm, &def.Release.Core)
		if err != nil {
			return nil, fmt.Errorf("filtering enabled core helm charts: %w", err)
		}

		if err := collectHelmCharts(charts, rm.CorePlatform.Components.Helm.ChartRepositories(), def.Release.Core.HelmValueFiles(), valuesResolver, &crds); err != nil {
			return nil, fmt.Errorf("collecting core helm charts: %w", err)
		}
	}

	if rm.ProductExtension != nil && rm.ProductExtension.Components.Helm != nil && len(def.Release.Product.Helm) > 0 {
		charts, err := enabledHelmCharts(rm.ProductExtension.Components.Helm, &def.Release.Product)
		if err != nil {
			return nil, fmt.Errorf("filtering enabled product helm charts: %w", err)
		}

		if err = collectHelmCharts(charts, rm.ProductExtension.Components.Helm.ChartRepositories(), def.Release.Product.HelmValueFiles(), valuesResolver, &crds); err != nil {
			return nil, fmt.Errorf("collecting product helm charts: %w", err)
		}
	}

	if def.Kubernetes.Helm != nil {
		var charts []helmChart
		for _, chart := range def.Kubernetes.Helm.Charts {
			charts = append(charts, chart)
		}

		if err := collectHelmCharts(charts, def.Kubernetes.Helm.ChartRepositories(), def.Kubernetes.Helm.ValueFiles(), valuesResolver, &crds); err != nil {
			return nil, fmt.Errorf("collecting user helm charts: %w", err)
		}
	}

	return crds, nil
}

func collectHelmCharts(charts []helmChart, repositories, valueFiles map[string]string, valuesResolver helmValuesResolver, crds *[]*helm.CRD) error {
	for _, chart := range charts {
		name := chart.GetName()
		repository, ok := repositories[chart.GetRepositoryName()]
		if !ok {
			return fmt.Errorf("repository not found for chart: %s", name)
		}

		source := &helm.ValueSource{Inline: chart.GetInlineValues(), File: valueFiles[name]}
		values, err := valuesResolver.Resolve(source)
		if err != nil {
			return fmt.Errorf("resolving values for chart %s: %w", name, err)
		}

		crd := chart.ToCRD(values, repository)
		*crds = append(*crds, crd)
	}

	return nil
}

func enabledHelmCharts(helm *api.Helm, enabled *release.Components) ([]helmChart, error) {
	var charts []helmChart

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

		if slices.ContainsFunc(charts, func(c helmChart) bool {
			return c.GetName() == name
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
		charts = append(charts, chart)

		return nil
	}

	for _, e := range enabled.Helm {
		if err := addChart(e.Name); err != nil {
			return nil, fmt.Errorf("adding helm chart '%s': %w", e.Name, err)
		}
	}

	return charts, nil
}
