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

type SchemaVersion string

const SchemaV0 SchemaVersion = "v0"

type DependencyType string

const (
	DependencyTypeExtension DependencyType = "sysext"
	DependencyTypeHelm      DependencyType = "helm"
)

type Metadata struct {
	Name         string `yaml:"name" validate:"required"`
	Version      string `yaml:"version" validate:"required"`
	CreationDate string `yaml:"creationDate,omitempty"`
}

type Helm struct {
	Charts       []*HelmChart      `yaml:"charts" validate:"dive"`
	Repositories []*HelmRepository `yaml:"repositories" validate:"dive"`
}

type HelmChart struct {
	Name       string                `yaml:"name,omitempty"`
	Chart      string                `yaml:"chart" validate:"required"`
	Version    string                `yaml:"version" validate:"required"`
	Namespace  string                `yaml:"namespace,omitempty"`
	Repository string                `yaml:"repository,omitempty"`
	Values     map[string]any        `yaml:"values,omitempty"`
	DependsOn  []HelmChartDependency `yaml:"dependsOn,omitempty" validate:"dive"`
	Images     []HelmChartImage      `yaml:"images,omitempty"`
}

type HelmChartImage struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
}

type HelmChartDependency struct {
	Name string         `yaml:"name" validate:"required"`
	Type DependencyType `yaml:"type" validate:"required,oneof=sysext helm"`
}

type HelmRepository struct {
	Name string `yaml:"name" validate:"required"`
	URL  string `yaml:"url" validate:"required,url"`
}

type Systemd struct {
	Extensions []SystemdExtension `yaml:"extensions,omitempty" validate:"dive"`
}

type SystemdExtension struct {
	Name          string   `yaml:"name" validate:"required"`
	Image         string   `yaml:"image" validate:"required"`
	Required      bool     `yaml:"required,omitempty"`
	KernelModules []string `yaml:"kernelModules,omitempty"`
}
