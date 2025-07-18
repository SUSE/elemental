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
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/helm"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
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

type Helm struct {
	FS             vfs.FS
	RelativePath   string
	DestinationDir string
	ValuesResolver helmValuesResolver
	Logger         log.Logger
}

func NewHelm(fs vfs.FS, valuesResolver helmValuesResolver, logger log.Logger, destinationDir string) *Helm {
	return &Helm{
		FS:             fs,
		RelativePath:   image.HelmPath(),
		DestinationDir: destinationDir,
		ValuesResolver: valuesResolver,
		Logger:         logger,
	}
}

func needsHelmChartsSetup(def *image.Definition) bool {
	return len(def.Release.Core.Helm) > 0 || len(def.Release.Product.Helm) > 0 || def.Kubernetes.Helm != nil
}

func (h *Helm) Configure(def *image.Definition, rm *resolver.ResolvedManifest) ([]string, error) {
	if len(def.Release.Core.Helm) > 0 {
		var charts []string
		for _, c := range def.Release.Core.Helm {
			charts = append(charts, c.Name)
		}

		h.Logger.Info("Enabling the following core components: %s", strings.Join(charts, ", "))
	}

	if len(def.Release.Product.Helm) > 0 {
		var charts []string
		for _, c := range def.Release.Product.Helm {
			charts = append(charts, c.Name)
		}

		h.Logger.Info("Enabling the following product extensions: %s", strings.Join(charts, ", "))
	}

	charts, err := h.retrieveHelmCharts(rm, def)
	if err != nil {
		return nil, fmt.Errorf("retrieving helm charts: %w", err)
	}

	chartFiles, err := h.writeHelmCharts(charts)
	if err != nil {
		return nil, fmt.Errorf("writing helm chart resources: %w", err)
	}

	return chartFiles, nil
}

func (h *Helm) writeHelmCharts(crds []*helm.CRD) ([]string, error) {
	if err := vfs.MkdirAll(h.FS, filepath.Join(h.DestinationDir, h.RelativePath), vfs.DirPerm); err != nil {
		return nil, fmt.Errorf("creating directory: %w", err)
	}

	var charts []string

	for _, crd := range crds {
		data, err := yaml.Marshal(crd)
		if err != nil {
			return nil, fmt.Errorf("marshaling helm chart %s: %w", crd.Metadata.Name, err)
		}

		chartName := fmt.Sprintf("%s.yaml", crd.Metadata.Name)
		relativePath := filepath.Join("/", h.RelativePath, chartName)
		fullPath := filepath.Join(h.DestinationDir, relativePath)
		if err = h.FS.WriteFile(fullPath, data, 0o644); err != nil {
			return nil, fmt.Errorf("writing helm chart: %w", err)
		}

		charts = append(charts, relativePath)
	}

	return charts, nil
}

func (h *Helm) retrieveHelmCharts(rm *resolver.ResolvedManifest, def *image.Definition) ([]*helm.CRD, error) {
	var crds []*helm.CRD

	if rm.CorePlatform != nil && rm.CorePlatform.Components.Helm != nil && len(def.Release.Core.Helm) > 0 {
		charts, err := enabledHelmCharts(rm.CorePlatform.Components.Helm, &def.Release.Core)
		if err != nil {
			return nil, fmt.Errorf("filtering enabled core helm charts: %w", err)
		}

		if err = h.collectHelmCharts(charts, rm.CorePlatform.Components.Helm.ChartRepositories(), def.Release.Core.HelmValueFiles(), &crds); err != nil {
			return nil, fmt.Errorf("collecting core helm charts: %w", err)
		}
	}

	if rm.ProductExtension != nil && rm.ProductExtension.Components.Helm != nil && len(def.Release.Product.Helm) > 0 {
		charts, err := enabledHelmCharts(rm.ProductExtension.Components.Helm, &def.Release.Product)
		if err != nil {
			return nil, fmt.Errorf("filtering enabled product helm charts: %w", err)
		}

		if err = h.collectHelmCharts(charts, rm.ProductExtension.Components.Helm.ChartRepositories(), def.Release.Product.HelmValueFiles(), &crds); err != nil {
			return nil, fmt.Errorf("collecting product helm charts: %w", err)
		}
	}

	if def.Kubernetes.Helm != nil {
		var charts []helmChart
		for _, chart := range def.Kubernetes.Helm.Charts {
			charts = append(charts, chart)
		}

		if err := h.collectHelmCharts(charts, def.Kubernetes.Helm.ChartRepositories(), def.Kubernetes.Helm.ValueFiles(), &crds); err != nil {
			return nil, fmt.Errorf("collecting user helm charts: %w", err)
		}
	}

	return crds, nil
}

func (h *Helm) collectHelmCharts(charts []helmChart, repositories, valueFiles map[string]string, crds *[]*helm.CRD) error {
	for _, chart := range charts {
		name := chart.GetName()
		repository, ok := repositories[chart.GetRepositoryName()]
		if !ok {
			return fmt.Errorf("repository not found for chart: %s", name)
		}

		source := &helm.ValueSource{Inline: chart.GetInlineValues(), File: valueFiles[name]}
		values, err := h.ValuesResolver.Resolve(source)
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
