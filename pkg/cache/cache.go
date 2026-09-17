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

package cache

import (
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	DefaultDir = "/cache"
	ociDir     = "oci"
	filesDir   = "files"
)

type Mode string

const (
	Enabled  Mode = "enabled"
	Disabled Mode = "disabled"
	Refresh  Mode = "refresh"
)

type DownloadMode string

const (
	AutoPull DownloadMode = "auto-pull"
	Offline  DownloadMode = "offline"
)

func (m Mode) IsValid() bool {
	return m == Enabled || m == Disabled || m == Refresh
}

func (d DownloadMode) IsValid() bool {
	return d == AutoPull || d == Offline
}

type Policy struct {
	Mode         Mode
	DownloadMode DownloadMode
}

func DefaultPolicy() Policy {
	return Policy{
		Mode:         Enabled,
		DownloadMode: AutoPull,
	}
}

func (p Policy) Validate() error {
	if !p.Mode.IsValid() {
		return fmt.Errorf("invalid cache mode %q", p.Mode)
	}

	if !p.DownloadMode.IsValid() {
		return fmt.Errorf("invalid download mode %q", p.DownloadMode)
	}

	if p.DownloadMode == Offline && p.Mode != Enabled {
		return fmt.Errorf("download mode %q is not valid with cache mode %q", p.DownloadMode, p.Mode)
	}

	return nil
}

func (p Policy) reads() bool {
	return p.Mode != Disabled
}

func (p Policy) fetches() bool {
	return p.DownloadMode != Offline
}

type Cache struct {
	fs     vfs.FS
	logger log.Logger
	dir    string
	policy Policy
}

type Opt func(c *Cache)

func WithFS(fs vfs.FS) Opt {
	return func(c *Cache) {
		c.fs = fs
	}
}

func WithLogger(logger log.Logger) Opt {
	return func(c *Cache) {
		c.logger = logger
	}
}

func WithPolicy(policy Policy) Opt {
	return func(c *Cache) {
		c.policy = policy
	}
}

func New(dir string, opts ...Opt) (*Cache, error) {
	c := &Cache{
		fs:     vfs.New(),
		logger: log.New(),
		dir:    dir,
		policy: DefaultPolicy(),
	}

	for _, o := range opts {
		o(c)
	}

	if err := c.policy.Validate(); err != nil {
		return nil, fmt.Errorf("validating cache policy: %w", err)
	}

	if !c.policy.reads() {
		c.logger.Info("Cache is disabled")
		return c, nil
	}

	// Only the cache's own subdirectories are ever removed, so a refresh
	// pointed at the wrong directory cannot delete unrelated content.
	for _, sub := range []string{c.filesDir(), c.ociDir()} {
		if c.policy.Mode == Refresh {
			c.logger.Info("Refreshing cache at %s", sub)
			if err := c.fs.RemoveAll(sub); err != nil {
				return nil, fmt.Errorf("removing existing cache directory '%s': %w", sub, err)
			}
		}

		if err := vfs.MkdirAll(c.fs, sub, vfs.DirPerm); err != nil {
			return nil, fmt.Errorf("creating cache directory '%s': %w", sub, err)
		}
	}

	c.logger.Info("Using cache at %s", c.dir)
	return c, nil
}

func (c *Cache) Dir() string {
	return c.dir
}

func (c *Cache) filesDir() string {
	return filepath.Join(c.dir, filesDir)
}

func (c *Cache) ociDir() string {
	return filepath.Join(c.dir, ociDir)
}
