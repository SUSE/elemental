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

//revive:disable:var-naming
package api

import (
	"github.com/suse/elemental/v3/pkg/helm"
)

type DependencyType string

const (
	DependencyTypeExtension DependencyType = "sysext"
	DependencyTypeHelm      DependencyType = "helm"
)

type Metadata struct {
	Name         string `yaml:"name"`
	Version      string `yaml:"version"`
	CreationDate string `yaml:"creationDate,omitempty"`
}

type Helm struct {
	Charts       []*HelmChart      `yaml:"charts"`
	Repositories []*HelmRepository `yaml:"repositories"`
}

type HelmChart struct {
	Name       string                `yaml:"name,omitempty"`
	Chart      string                `yaml:"chart"`
	Version    string                `yaml:"version"`
	Namespace  string                `yaml:"namespace,omitempty"`
	Repository string                `yaml:"repository,omitempty"`
	Values     map[string]any        `yaml:"values,omitempty"`
	DependsOn  []HelmChartDependency `yaml:"dependsOn,omitempty"`
	Images     []HelmChartImage      `yaml:"images,omitempty"`
}

func (c *HelmChart) GetName() string {
	return c.Chart
}

func (c *HelmChart) GetInlineValues() map[string]any {
	return c.Values
}

func (c *HelmChart) GetRepositoryName() string {
	return c.Repository
}

func (c *HelmChart) ToCRD(values []byte, repository string) *helm.CRD {
	return helm.NewCRD(c.Namespace, c.Chart, c.Version, string(values), repository)
}

func (c *HelmChart) ExtensionDependencies() []string {
	var dependencies []string

	for _, dependency := range c.DependsOn {
		if dependency.Type == DependencyTypeExtension {
			dependencies = append(dependencies, dependency.Name)
		}
	}

	return dependencies
}

type HelmChartImage struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
}

type HelmChartDependency struct {
	Name string         `yaml:"name"`
	Type DependencyType `yaml:"type"`
}

type HelmRepository struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type Systemd struct {
	Extensions []SystemdExtension `yaml:"extensions,omitempty"`
}

type SystemdExtension struct {
	Name          string   `yaml:"name"`
	Image         string   `yaml:"image"`
	Required      bool     `yaml:"required,omitempty"`
	KernelModules []string `yaml:"kernelModules,omitempty"`
}
