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

package core

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/pkg/manifest/api"
)

type ReleaseManifest struct {
	Metadata   *api.Metadata `yaml:"metadata"`
	Components Components    `yaml:"components"`
}

type Components struct {
	OperatingSystem *OperatingSystem `yaml:"operatingSystem"`
	Kubernetes      Kubernetes       `yaml:"kubernetes"`
	Systemd         api.Systemd      `yaml:"systemd,omitempty"`
	Helm            *api.Helm        `yaml:"helm,omitempty"`
}

type OperatingSystem struct {
	Version string `yaml:"version"`
	Image   string `yaml:"image"`
}

type Kubernetes struct {
	RKE2 *RKE2 `yaml:"rke2"`
}

type RKE2 struct {
	Version string `yaml:"version"`
	Image   string `yaml:"image"`
}

func Parse(data []byte) (*ReleaseManifest, error) {
	rm := &ReleaseManifest{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(rm); err != nil {
		return nil, fmt.Errorf("unmarshaling 'core' release manifest: %w", err)
	}

	return rm, nil
}
