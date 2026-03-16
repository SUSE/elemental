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

package config

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/go-playground/validator/v10"

	v0 "github.com/suse/elemental/v3/internal/config/v0"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type SchemaVersion string

const SchemaV0 SchemaVersion = "v0"

type Output struct {
	RootPath string

	// ConfigPath is only populated if configuration (incl. network, catalyst and custom scripts)
	// is requested separately. Note that extensions are *always* part of the RootPath instead.
	ConfigPath string
}

func NewOutput(fs vfs.FS, rootPath, configPath string) (Output, error) {
	if rootPath == "" {
		dir, err := vfs.TempDir(fs, "", "work-")
		if err != nil {
			return Output{}, err
		}

		rootPath = dir
	} else if err := vfs.MkdirAll(fs, rootPath, vfs.DirPerm); err != nil {
		return Output{}, err
	}

	if configPath != "" {
		if err := vfs.MkdirAll(fs, configPath, vfs.DirPerm); err != nil {
			return Output{}, err
		}
	}

	return Output{
		RootPath:   rootPath,
		ConfigPath: configPath,
	}, nil
}

func (o Output) OverlaysDir() string {
	return filepath.Join(o.RootPath, "overlays")
}

func (o Output) FirstbootConfigDir() string {
	if o.ConfigPath != "" {
		return o.ConfigPath
	}

	return filepath.Join(o.OverlaysDir(), deployment.ConfigMnt)
}

func (o Output) CatalystConfigDir() string {
	return filepath.Join(o.FirstbootConfigDir(), "catalyst")
}

func (o Output) ExtractedFilesStoreDir() string {
	return filepath.Join(o.RootPath, "store")
}

func (o Output) ReleaseManifestsStoreDir() string {
	return filepath.Join(o.ExtractedFilesStoreDir(), "release-manifests")
}

func (o Output) ISOStoreDir() string {
	return filepath.Join(o.ExtractedFilesStoreDir(), "ISOs")
}

func (o Output) Cleanup(fs vfs.FS) error {
	return fs.RemoveAll(o.RootPath)
}

func Write(f vfs.FS, configDir string, conf *image.Configuration) error {
	return v0.Write(f, v0.Dir(configDir), conf)
}

func Parse(f vfs.FS, configDir string) (conf *image.Configuration, err error) {
	schemaVersion, err := LoadSchemaVersion(f, configDir)
	if err != nil {
		return nil, fmt.Errorf("failed parsing schema version: %w", err)
	}

	switch schemaVersion {
	case SchemaV0:
		return v0.Parse(f, v0.Dir(configDir))
	default:
		return nil, fmt.Errorf("unknown schema version: '%s'", schemaVersion)
	}
}

type releaseSchema struct {
	SchemaVersion SchemaVersion `yaml:"schema" validate:"required,oneof=v0"`
}

func LoadSchemaVersion(f vfs.FS, configDir string) (SchemaVersion, error) {
	installFilepath := filepath.Join(configDir, "install.yaml")

	data, err := f.ReadFile(installFilepath)
	if err != nil {
		return "", fmt.Errorf("reading config file '%s': %w", installFilepath, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(false)

	var r releaseSchema
	if err := decoder.Decode(&r); err != nil {
		return "", fmt.Errorf("failed decoding struct: %w", err)
	}

	if err := validator.New().Struct(r); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			err = api.FormatErrors(validationErrors)
		}

		return "", fmt.Errorf("validating schema version: %w", err)
	}

	return r.SchemaVersion, nil
}
