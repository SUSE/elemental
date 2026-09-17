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
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/cache"
	"github.com/suse/elemental/v3/pkg/extractor"
	"github.com/suse/elemental/v3/pkg/hauler"
	"github.com/suse/elemental/v3/pkg/http"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/manifest/source"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

type unpackFunc func(ctx context.Context, imageRef, destDir string) error

type helmConfigurator interface {
	Configure(ctx context.Context, conf *image.Configuration, manifest *resolver.ResolvedManifest, bConfig *butane.Config) ([]string, error)
	ContainerImages() []string
}

type releaseManifestResolver interface {
	Resolve(uri string) (*resolver.ResolvedManifest, error)
}

type downloader interface {
	URLBody(ctx context.Context, url string) (io.ReadCloser, error)
}

type Manager struct {
	system   *sys.System
	local    bool
	cache    *cache.Cache
	platform string

	rmResolver  releaseManifestResolver
	downloader  downloader
	unpackImage unpackFunc
	helm        helmConfigurator
	imageStore  imageStore
}

type Opts func(m *Manager)

func WithManifestResolver(r releaseManifestResolver) Opts {
	return func(m *Manager) {
		m.rmResolver = r
	}
}

func WithDownloader(d downloader) Opts {
	return func(m *Manager) {
		m.downloader = d
	}
}

func WithUnpackFunc(u unpackFunc) Opts {
	return func(m *Manager) {
		m.unpackImage = u
	}
}

func WithLocal(local bool) Opts {
	return func(m *Manager) {
		m.local = local
	}
}

func WithPlatform(platform string) Opts {
	return func(m *Manager) {
		m.platform = platform
	}
}

func WithCache(c *cache.Cache) Opts {
	return func(m *Manager) {
		m.cache = c
	}
}

func WithImageStore(s imageStore) Opts {
	return func(m *Manager) {
		m.imageStore = s
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

	if m.downloader == nil {
		m.downloader = &http.Downloader{}
	}

	if m.platform == "" {
		m.platform = sys.Platform().String()
	}

	if m.cache == nil {
		// A disabled cache fetches everything directly
		policy := cache.Policy{Mode: cache.Disabled, DownloadMode: cache.AutoPull}
		m.cache, _ = cache.New("", cache.WithPolicy(policy), cache.WithFS(sys.FS()), cache.WithLogger(log.New(log.WithDiscardAll())))
	}

	if m.imageStore == nil {
		m.imageStore = hauler.New(sys)
	}

	if m.unpackImage == nil {
		m.unpackImage = func(ctx context.Context, imageRef, destDir string) error {
			unpacker := unpack.NewOCIUnpacker(
				sys, imageRef,
				unpack.WithLocalOCI(m.local),
				unpack.WithCacheOCI(m.cache),
				unpack.WithPlatformRefOCI(m.platform),
			)
			_, err := unpacker.Unpack(ctx, destDir)
			return err
		}
	}

	return m
}

// downloadFile places the contents of url at dest, going through the cache.
func (m *Manager) downloadFile(ctx context.Context, url, dest string) error {
	return m.cache.File(ctx, url, dest, func(ctx context.Context) (io.ReadCloser, error) {
		return m.downloader.URLBody(ctx, url)
	})
}

// ConfigureComponents configures the components defined in the provided configuration
// and returns the resolved release manifest from said configuration.
func (m *Manager) ConfigureComponents(ctx context.Context, conf *image.Configuration, output Output) (rm *resolver.ResolvedManifest, err error) {
	if m.rmResolver == nil {
		defaultResolver, err := defaultManifestResolver(m.system.FS(), output, m.local, m.cache)
		if err != nil {
			return nil, fmt.Errorf("using default release manifest resolver: %w", err)
		}
		m.rmResolver = defaultResolver
	}

	rm, err = m.rmResolver.Resolve(conf.Release.ManifestURI)
	if err != nil {
		return nil, fmt.Errorf("resolving release manifest at uri '%s': %w", conf.Release.ManifestURI, err)
	}

	if err = m.configureNetworkOnFirstboot(conf, output); err != nil {
		return nil, fmt.Errorf("configuring network: %w", err)
	}

	if err = m.configureCustomScripts(conf, output); err != nil {
		return nil, fmt.Errorf("configuring custom scripts: %w", err)
	}

	extensions, err := enabledExtensions(rm, conf, m.system.Logger())
	if err != nil {
		return nil, fmt.Errorf("filtering enabled systemd extensions: %w", err)
	}

	if len(extensions) != 0 {
		if err = m.downloadSystemExtensions(ctx, extensions, output); err != nil {
			return nil, fmt.Errorf("downloading system extensions: %w", err)
		}
	}

	if err = m.configureSystem(ctx, conf, output, rm, extensions); err != nil {
		return nil, fmt.Errorf("configuring ignition: %w", err)
	}

	return rm, nil
}

func defaultManifestResolver(fs vfs.FS, out Output, local bool, c *cache.Cache) (res *resolver.Resolver, err error) {
	const (
		globPattern = "release_manifest*.yaml"
	)

	searchPaths := []string{
		globPattern,
		filepath.Join("etc", "release-manifest", globPattern),
	}

	manifestsDir := out.ReleaseManifestsStoreDir()
	if err := vfs.MkdirAll(fs, manifestsDir, 0700); err != nil {
		return nil, fmt.Errorf("creating release manifest store '%s': %w", manifestsDir, err)
	}

	extr, err := extractor.New(searchPaths, extractor.WithStore(manifestsDir), extractor.WithLocal(local), extractor.WithCache(c))
	if err != nil {
		return nil, fmt.Errorf("initializing OCI release manifest extractor: %w", err)
	}

	return resolver.New(source.NewReader(extr)), nil
}
