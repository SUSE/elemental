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

package config

import (
	"context"

	"github.com/suse/elemental/v3/internal/image"

	"github.com/suse/elemental/v3/pkg/http"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type downloadFunc func(ctx context.Context, fs vfs.FS, url, path string) error

type helmConfigurator interface {
	Configure(conf *image.Configuration, manifest *resolver.ResolvedManifest) ([]string, error)
}

type releaseManifestResolver interface {
	Resolve(uri string) (*resolver.ResolvedManifest, error)
}

type Manager struct {
	system *sys.System
	local  bool

	rmResolver   releaseManifestResolver
	downloadFile downloadFunc
	helm         helmConfigurator
}

type Opts func(m *Manager)

func WithManifestResolver(r releaseManifestResolver) Opts {
	return func(m *Manager) {
		m.rmResolver = r
	}
}

func WithDownloadFunc(d downloadFunc) Opts {
	return func(m *Manager) {
		m.downloadFile = d
	}
}

func WithLocal(local bool) Opts {
	return func(m *Manager) {
		m.local = local
	}
}

func NewManager(sys *sys.System, helm helmConfigurator, opts ...Opts) *Manager {
	m := &Manager{
		system: sys,
		helm:   helm,
	}

	for _, o := range opts {
		o(m)
	}

	if m.downloadFile == nil {
		m.downloadFile = http.DownloadFile
	}

	return m
}
