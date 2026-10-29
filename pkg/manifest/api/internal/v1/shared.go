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

package v1

import (
	"github.com/suse/elemental/v3/pkg/helm"
	v0 "github.com/suse/elemental/v3/pkg/manifest/api/internal/v0"
)

type SchemaVersion = v0.SchemaVersion
type DependencyType = v0.DependencyType
type HelmChartImage = v0.HelmChartImage
type HelmChartDependency = v0.HelmChartDependency
type HelmRepository = v0.HelmRepository
type Systemd = v0.Systemd
type SystemdExtension = v0.SystemdExtension

const SchemaV1 SchemaVersion = "v1"

type Metadata struct {
	Name         string `yaml:"name" validate:"required"`
	CreationDate string `yaml:"creationDate,omitempty"`
}

type Helm struct {
	Charts       []*HelmChart      `yaml:"charts" validate:"dive"`
	Repositories []*HelmRepository `yaml:"repositories" validate:"dive"`
}

type HelmChart struct {
	Name            string                `yaml:"name,omitempty"`
	Chart           string                `yaml:"chart" validate:"required"`
	Version         string                `yaml:"version" validate:"required"`
	Namespace       string                `yaml:"namespace,omitempty"`
	Repository      string                `yaml:"repository,omitempty"`
	PriorityUpgrade bool                  `yaml:"priorityUpgrade,omitempty"`
	Values          map[string]any        `yaml:"values,omitempty"`
	DependsOn       []HelmChartDependency `yaml:"dependsOn,omitempty" validate:"dive"`
	Images          []HelmChartImage      `yaml:"images,omitempty"`
}

func (c *HelmChart) GetName() string {
	return c.Chart
}

func (c *HelmChart) GetVersion() string {
	return c.Version
}

func (c *HelmChart) GetAPIVersions() []string {
	return nil
}

func (c *HelmChart) GetInlineValues() map[string]any {
	return c.Values
}

func (c *HelmChart) GetRepositoryName() string {
	return c.Repository
}

func (c *HelmChart) ToCRD(values []byte, repository string, hasAuth bool, skipTLSVerify bool) (*helm.CRD, error) {
	return helm.NewCRD(c.Namespace, c.Chart, c.Version, string(values), repository, hasAuth, skipTLSVerify)
}

func (c *HelmChart) ExtensionDependencies() []string {
	var dependencies []string

	for _, dependency := range c.DependsOn {
		if dependency.Type == v0.DependencyTypeExtension {
			dependencies = append(dependencies, dependency.Name)
		}
	}

	return dependencies
}

func MigrateHelmV0(old *v0.Helm) *Helm {
	if old == nil {
		return nil
	}

	migrated := &Helm{Repositories: old.Repositories}
	for _, c := range old.Charts {
		migrated.Charts = append(migrated.Charts, &HelmChart{
			Name:       c.Name,
			Chart:      c.Chart,
			Version:    c.Version,
			Namespace:  c.Namespace,
			Repository: c.Repository,
			Values:     c.Values,
			DependsOn:  c.DependsOn,
			Images:     c.Images,
		})
	}

	return migrated
}
