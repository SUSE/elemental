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

func (h *Helm) Configure(conf *image.Configuration, rm *resolver.ResolvedManifest) ([]string, error) {
	if len(conf.Release.Components.HelmCharts) > 0 {
		var charts []string
		for _, c := range conf.Release.Components.HelmCharts {
			charts = append(charts, c.Name)
		}

		h.Logger.Info("Enabling the following Helm components: %s", strings.Join(charts, ", "))
	}

	charts, err := h.retrieveHelmCharts(rm, conf)
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

func (h *Helm) retrieveHelmCharts(rm *resolver.ResolvedManifest, conf *image.Configuration) ([]*helm.CRD, error) {
	var crds []*helm.CRD

	charts, repositories, err := enabledHelmCharts(rm, conf.Release.Components.HelmCharts, h.Logger)
	if err != nil {
		return nil, fmt.Errorf("filtering enabled helm charts: %w", err)
	}

	valueFiles := conf.Release.Components.HelmValueFiles()

	for _, chart := range charts {
		if err = h.appendHelmChart(chart, repositories, valueFiles, &crds); err != nil {
			return nil, fmt.Errorf("collecting helm charts: %w", err)
		}
	}

	if conf.Kubernetes.Helm != nil {
		repositories = conf.Kubernetes.Helm.ChartRepositories()
		valueFiles = conf.Kubernetes.Helm.ValueFiles()

		for _, chart := range conf.Kubernetes.Helm.Charts {
			if err = h.appendHelmChart(chart, repositories, valueFiles, &crds); err != nil {
				return nil, fmt.Errorf("collecting user helm charts: %w", err)
			}
		}
	}

	return crds, nil
}

func (h *Helm) appendHelmChart(chart helmChart, repositories, valueFiles map[string]string, crds *[]*helm.CRD) error {
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

	return nil
}

func enabledHelmCharts(rm *resolver.ResolvedManifest, enabled []release.HelmChart, logger log.Logger) ([]*api.HelmChart, map[string]string, error) {
	coreCharts, productCharts := map[string]*api.HelmChart{}, map[string]*api.HelmChart{}
	repositories := map[string]string{}

	if rm.CorePlatform.Components.Helm != nil {
		for _, c := range rm.CorePlatform.Components.Helm.Charts {
			coreCharts[c.Chart] = c
		}

		for _, repository := range rm.CorePlatform.Components.Helm.Repositories {
			repositories[repository.Name] = repository.URL
		}
	}

	if rm.ProductExtension != nil && rm.ProductExtension.Components.Helm != nil {
		for _, c := range rm.ProductExtension.Components.Helm.Charts {
			productCharts[c.Chart] = c
		}

		for _, repository := range rm.ProductExtension.Components.Helm.Repositories {
			repositories[repository.Name] = repository.URL
		}
	}

	var charts []*api.HelmChart
	var addChart func(name string) error

	// Add a chart and its direct dependencies, avoiding duplicates.
	// Prioritise charts from product releases over core ones.
	addChart = func(name string) error {
		source := "product"

		chart, ok := productCharts[name]
		if !ok {
			chart, ok = coreCharts[name]
			if !ok {
				return fmt.Errorf("helm chart does not exist")
			}

			source = "core"
		}

		if logger != nil {
			logger.Info("Using Helm chart %s from %s release", name, source)
		}

		if slices.ContainsFunc(charts, func(c *api.HelmChart) bool {
			return c.GetName() == name
		}) {
			return nil
		}

		// Check for dependencies and add them first.
		for _, d := range chart.DependsOn {
			if d.Type == api.DependencyTypeHelm {
				if err := addChart(d.Name); err != nil {
					return fmt.Errorf("adding dependent helm chart '%s': %w", d.Name, err)
				}
			}
		}

		// Add the main chart.
		charts = append(charts, chart)

		return nil
	}

	for _, e := range enabled {
		if err := addChart(e.Name); err != nil {
			return nil, nil, fmt.Errorf("adding helm chart '%s': %w", e.Name, err)
		}
	}

	return charts, repositories, nil
}
