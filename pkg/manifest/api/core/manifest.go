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
	"bytes"
	"errors"
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/go-playground/validator/v10"
	"github.com/suse/elemental/v3/pkg/manifest/api"
)

type ReleaseManifest struct {
	Metadata   *api.Metadata `yaml:"metadata,omitempty"`
	Components Components    `yaml:"components" validate:"required"`
}

type Components struct {
	OperatingSystem *OperatingSystem `yaml:"operatingSystem" validate:"required"`
	Systemd         api.Systemd      `yaml:"systemd,omitempty"`
	Helm            *api.Helm        `yaml:"helm,omitempty"`
}

type OperatingSystem struct {
	Image Image `yaml:"image" validate:"required"`
}

type Image struct {
	Base string `yaml:"base" validate:"required"`
	ISO  string `yaml:"iso" validate:"required"`
}

func Parse(data []byte) (*ReleaseManifest, error) {
	rm := &ReleaseManifest{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(rm); err != nil {
		return nil, fmt.Errorf("unmarshaling 'core' release manifest: %w", err)
	}

	if err := api.NewValidator(api.WithYAMLFieldNames()).Struct(rm); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			err = api.FormatErrors(validationErrors)
		}

		return nil, fmt.Errorf("validating 'core' release manifest: %w", err)
	}

	return rm, nil
}
