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
	v0 "github.com/suse/elemental/v3/pkg/manifest/api/internal/v0"
)

type ReleaseManifest struct {
	Schema     v0.SchemaVersion `yaml:"schema,omitempty"`
	Metadata   *v0.Metadata     `yaml:"metadata,omitempty"`
	Components Components       `yaml:"components" validate:"required"`
}

type Components struct {
	OperatingSystem *OperatingSystem `yaml:"operatingSystem" validate:"required"`
	Kubernetes      *Kubernetes      `yaml:"kubernetes"`
	Systemd         v0.Systemd       `yaml:"systemd,omitempty"`
	Helm            *v0.Helm         `yaml:"helm,omitempty"`
}

type OperatingSystem struct {
	Image Image `yaml:"image" validate:"required"`
}

type Kubernetes struct {
	Version string `yaml:"version" validate:"required"`
	Image   string `yaml:"image" validate:"required"`
}

type Image struct {
	Base string `yaml:"base" validate:"required"`
	ISO  string `yaml:"iso" validate:"required"`
}
