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

package api

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"

	"go.yaml.in/yaml/v3"

	"github.com/go-playground/validator/v10"
	v0 "github.com/suse/elemental/v3/pkg/manifest/api/internal/v0"
	v1 "github.com/suse/elemental/v3/pkg/manifest/api/internal/v1"
)

type SchemaVersion = v1.SchemaVersion
type DependencyType = v1.DependencyType
type Metadata = v1.Metadata
type Helm = v1.Helm
type HelmChart = v1.HelmChart
type HelmChartImage = v1.HelmChartImage
type HelmChartDependency = v1.HelmChartDependency
type HelmRepository = v1.HelmRepository
type Systemd = v1.Systemd
type SystemdExtension = v1.SystemdExtension

type schemaHeader struct {
	SchemaVersion SchemaVersion `yaml:"schema"`
}

func LoadSchemaVersion(data []byte) (SchemaVersion, error) {
	var header schemaHeader

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(false)

	if err := decoder.Decode(&header); err != nil {
		return "", fmt.Errorf("extracting schema version: %w", err)
	}

	// TODO: remove default once we have added schema to the official manifests.
	if header.SchemaVersion == "" {
		return SchemaV0, nil
	}

	switch header.SchemaVersion {
	case SchemaV0:
		return SchemaV0, nil
	case SchemaV1:
		return SchemaV1, nil
	default:
		return "", fmt.Errorf("unsupported manifest schema version: %q", header.SchemaVersion)
	}
}

func Parse[R any](data []byte) (*R, error) {
	typeR := reflect.TypeFor[R]().String()

	if _, err := LoadSchemaVersion(data); err != nil {
		return nil, fmt.Errorf("parsing %q release manifest: %w", typeR, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	rm := new(R)
	if err := decoder.Decode(rm); err != nil {
		return nil, fmt.Errorf("unmarshaling %q release manifest: %w", typeR, err)
	}

	if err := NewValidator(WithYAMLFieldNames()).Struct(rm); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			err = FormatErrors(validationErrors)
		}

		return nil, fmt.Errorf("validating %q release manifest: %w", typeR, err)
	}

	return rm, nil
}

const (
	SchemaV0 SchemaVersion = v0.SchemaV0
	SchemaV1 SchemaVersion = v1.SchemaV1
)

const (
	DependencyTypeExtension DependencyType = v0.DependencyTypeExtension
	DependencyTypeHelm      DependencyType = v0.DependencyTypeHelm
)
