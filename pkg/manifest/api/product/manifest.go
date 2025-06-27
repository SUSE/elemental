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

package product

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/pkg/manifest/api"
)

type ReleaseManifest struct {
	MetaData     *api.MetaData `yaml:"metadata,omitempty"`
	CorePlatform *CorePlatform `yaml:"corePlatform"`
	Components   Components    `yaml:"components,omitempty"`
}

type CorePlatform struct {
	Image   string `yaml:"image"`
	Version string `yaml:"version"`
}

type Components struct {
	Helm *api.Helm `yaml:"helm,omitempty"`
}

func Parse(data []byte) (*ReleaseManifest, error) {
	rm := &ReleaseManifest{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(rm); err != nil {
		return nil, fmt.Errorf("unmarshaling 'product' release manifest: %w", err)
	}

	if rm.CorePlatform == nil {
		return nil, fmt.Errorf("missing 'corePlatform' field")
	}

	return rm, nil
}
