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

package solution

import (
	"github.com/suse/elemental/v3/pkg/manifest/api/internal/v0/solution"
	v1 "github.com/suse/elemental/v3/pkg/manifest/api/internal/v1"
)

type CorePlatform = solution.CorePlatform
type Components = solution.Components

type ReleaseManifest struct {
	Schema       v1.SchemaVersion `yaml:"schema,omitempty"`
	Metadata     *v1.Metadata     `yaml:"metadata,omitempty"`
	CorePlatform *CorePlatform    `yaml:"corePlatform" validate:"required"`
	Components   Components       `yaml:"components,omitempty"`
}
