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
	v0 "github.com/suse/elemental/v3/pkg/manifest/api/internal/v0"
)

type SchemaVersion = v0.SchemaVersion
type DependencyType = v0.DependencyType
type Helm = v0.Helm
type HelmChart = v0.HelmChart
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
