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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type FileFetcher func(ctx context.Context) (io.ReadCloser, error)

// File places the file identified by key at dest. The file is copied from the
// cache when present. Otherwise downloaded and placed into cache when the settings allow it
func (c *Cache) File(ctx context.Context, key, dest string, fetch FileFetcher) error {
	if !c.policy.reads() {
		return c.fetchTo(ctx, dest, fetch)
	}

	path := c.filePath(key)
	c.logger.Debug("Cache path for key '%s': %s", key, path)

	exists, err := vfs.Exists(c.fs, path)
	if err != nil {
		return fmt.Errorf("checking cache for key '%s': %w", key, err)
	}

	if !exists {
		if !c.policy.fetches() {
			return fmt.Errorf("file '%s' is not cached and downloads are disabled", key)
		}

		c.logger.Info("Storing '%s' in cache", key)
		if err = c.fetchTo(ctx, path, fetch); err != nil {
			return err
		}
	} else {
		c.logger.Info("Using cached '%s'", key)
	}

	if err = vfs.CopyFile(c.fs, path, dest); err != nil {
		return fmt.Errorf("copying cached file to '%s': %w", dest, err)
	}

	return nil
}

// fetchTo streams the fetched contents to a temporary sibling of path and
// renames it into place, so that a failed fetch never leaves a partial file.
func (c *Cache) fetchTo(ctx context.Context, path string, fetch FileFetcher) (err error) {
	reader, err := fetch(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()

	tmp := path + ".tmp"
	file, err := c.fs.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating file '%s': %w", tmp, err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, c.fs.Remove(tmp))
		}
	}()

	if _, err = io.Copy(file, reader); err != nil {
		_ = file.Close()
		return fmt.Errorf("writing file '%s': %w", tmp, err)
	}

	if err = file.Close(); err != nil {
		return fmt.Errorf("closing file '%s': %w", tmp, err)
	}

	if err = c.fs.Rename(tmp, path); err != nil {
		return fmt.Errorf("moving file into place at '%s': %w", path, err)
	}

	return nil
}

func (c *Cache) filePath(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.filesDir(), hex.EncodeToString(sum[:]))
}
