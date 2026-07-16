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
	"fmt"

	"github.com/suse/elemental/v3/pkg/manifest/api"
	corev0 "github.com/suse/elemental/v3/pkg/manifest/api/internal/v0/core"
)

type ReleaseManifest = corev0.ReleaseManifest
type Components = corev0.Components
type OperatingSystem = corev0.OperatingSystem
type Kubernetes = corev0.Kubernetes
type Image = corev0.Image

func Parse(data []byte) (*ReleaseManifest, error) {
	version, err := api.LoadSchemaVersion(data)
	if err != nil {
		return nil, fmt.Errorf("parsing 'core' release manifest: %w", err)
	}

	switch version {
	case api.SchemaV0:
		return parseV0(data)
	default:
		return nil, fmt.Errorf("unknown release manifest version %q", version)
	}
}

func parseV0(data []byte) (*ReleaseManifest, error) {
	return api.Parse[corev0.ReleaseManifest](data)
}
