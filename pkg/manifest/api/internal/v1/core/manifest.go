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

package core

import (
	core "github.com/suse/elemental/v3/pkg/manifest/api/internal/v0/core"
	v1 "github.com/suse/elemental/v3/pkg/manifest/api/internal/v1"
)

type Components = core.Components
type OperatingSystem = core.OperatingSystem
type Kubernetes = core.Kubernetes
type Image = core.Image

type ReleaseManifest struct {
	Schema     v1.SchemaVersion `yaml:"schema,omitempty"`
	Metadata   *Metadata        `yaml:"metadata,omitempty"`
	Components Components       `yaml:"components" validate:"required"`
}

type Metadata struct {
	v1.Metadata `yaml:",inline"`
	Elemental   *Elemental `yaml:"elemental,omitempty"`
}

type Elemental struct {
	Version string `yaml:"version" validate:"required"`
	Image   string `yaml:"image" validate:"required"`
}
