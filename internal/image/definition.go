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

//revive:disable:var-naming
package image

import (
	"path/filepath"

	"github.com/suse/elemental/v3/internal/image/install"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/sys/platform"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	TypeRAW = "raw"
)

type Definition struct {
	Image         Image
	Configuration *Configuration
}

type Configuration struct {
	Installation install.Installation  `validate:"required"`
	Release      release.Release       `validate:"required"`
	Kubernetes   kubernetes.Kubernetes `validate:"omitempty"`
	Network      Network               `validate:"omitempty"`
	Custom       Custom                `validate:"omitempty"`
	ButaneConfig map[string]any        `validate:"omitempty"`
}

type Image struct {
	ImageType       string
	Platform        *platform.Platform
	OutputImageName string
}

type Network struct {
	CustomScript string
	ConfigDir    string
}

type Custom struct {
	ScriptsDir string
	FilesDir   string
}

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
