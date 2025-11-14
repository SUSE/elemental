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
	"fmt"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/manifest/extractor"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/http"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/manifest/source"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type ReportData struct {
	ResolvedManifest *resolver.ResolvedManifest
	CustomPartitions []*deployment.Partition
}

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

// ConfigureComponents configures the components defined in the provided configuration
// and outputs a report containing data that cannot easily be retrieved just by looking
// at the output directory.
func (m *Manager) ConfigureComponents(ctx context.Context, conf *image.Configuration, output OutputDir) (report *ReportData, err error) {
	if m.rmResolver == nil {
		defaultResolver, err := defaultManifestResolver(m.system.FS(), output, m.local)
		if err != nil {
			return nil, fmt.Errorf("using default release manifest resolver: %w", err)
		}
		m.rmResolver = defaultResolver
	}

	rm, err := m.rmResolver.Resolve(conf.Release.ManifestURI)
	if err != nil {
		return nil, fmt.Errorf("resolving release manifest at uri '%s': %w", conf.Release.ManifestURI, err)
	}

	var customPartitions []*deployment.Partition
	part := m.generatePreparePartition(conf)
	if part != nil {
		customPartitions = append(customPartitions, part)
		if err := m.configureNetworkOnPartition(conf, output, part); err != nil {
			return nil, fmt.Errorf("configuring network on partition with label '%s': %w", part.Label, err)
		}
	}

	k8sScript, k8sConfScript, err := m.configureKubernetes(ctx, conf, rm, output)
	if err != nil {
		return nil, fmt.Errorf("configuring kubernetes: %w", err)
	}

	if err = m.configureIgnition(conf, output, k8sScript, k8sConfScript); err != nil {
		return nil, fmt.Errorf("configuring ignition: %w", err)
	}

	if err = m.downloadSystemExtensions(ctx, conf, rm, output); err != nil {
		return nil, fmt.Errorf("configuring system extensions: %w", err)
	}

	return &ReportData{
		ResolvedManifest: rm,
		CustomPartitions: customPartitions,
	}, nil
}

func defaultManifestResolver(fs vfs.FS, out OutputDir, local bool) (res *resolver.Resolver, err error) {
	manifestsDir := out.ReleaseManifestsDir()
	if err := vfs.MkdirAll(fs, manifestsDir, 0700); err != nil {
		return nil, fmt.Errorf("creating release manifest store '%s': %w", manifestsDir, err)
	}

	extr, err := extractor.New(extractor.WithStore(manifestsDir))
	if err != nil {
		return nil, fmt.Errorf("initialising OCI release manifest extractor: %w", err)
	}

	return resolver.New(source.NewReader(extr, local)), nil
}
