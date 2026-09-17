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
	"errors"
	"fmt"
	"io/fs"

	containerregistry "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/match"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type ImageFetcher func(ctx context.Context) (containerregistry.Image, error)

// Image returns the container image identified by ref for the given platform. The image
// is served from the cache when present. Otherwise downloaded and cached when policy allows it.
func (c *Cache) Image(ctx context.Context, ref string, platform containerregistry.Platform, fetch ImageFetcher) (containerregistry.Image, error) {
	if !c.policy.reads() {
		return fetch(ctx)
	}

	path, err := c.layout()
	if err != nil {
		return nil, err
	}

	img, found, err := c.cachedImage(path, ref, platform)
	if err != nil {
		return nil, err
	}

	if found {
		c.logger.Debug("Using cached image %s (%s)", ref, platform.String())
		return img, nil
	}

	if !c.policy.fetches() {
		return nil, fmt.Errorf("image %s (%s) is not cached and downloads are disabled", ref, platform.String())
	}

	img, err = fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching image %s: %w", ref, err)
	}

	c.logger.Debug("Storing image %s (%s) in cache", ref, platform.String())
	opts := []layout.Option{
		layout.WithAnnotations(map[string]string{ocispec.AnnotationRefName: ref}),
		layout.WithPlatform(platform),
	}
	if err = path.ReplaceImage(img, matchImage(ref, platform), opts...); err != nil {
		return nil, fmt.Errorf("writing image %s to cache: %w", ref, err)
	}

	img, found, err = c.cachedImage(path, ref, platform)
	if err != nil {
		return nil, err
	}

	if !found {
		return nil, fmt.Errorf("image %s missing from cache after being written", ref)
	}

	return img, nil
}

// matchImage matches the index entry holding ref for the given platform.
func matchImage(ref string, platform containerregistry.Platform) match.Matcher {
	return func(desc containerregistry.Descriptor) bool {
		return desc.Annotations[ocispec.AnnotationRefName] == ref &&
			desc.Platform != nil && desc.Platform.Equals(platform)
	}
}

// layout opens the OCI layout backing the image cache, initializing an empty one on first use.
func (c *Cache) layout() (layout.Path, error) {
	dir, err := c.fs.RawPath(c.ociDir())
	if err != nil {
		return "", fmt.Errorf("resolving image cache directory: %w", err)
	}

	path, err := layout.FromPath(dir)
	if err == nil {
		return path, nil
	}

	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("opening image cache at '%s': %w", dir, err)
	}

	path, err = layout.Write(dir, empty.Index)
	if err != nil {
		return "", fmt.Errorf("initializing image cache at '%s': %w", dir, err)
	}

	return path, nil
}

// cachedImage looks up ref for the given platform in the layout index
func (c *Cache) cachedImage(path layout.Path, ref string, platform containerregistry.Platform) (containerregistry.Image, bool, error) {
	index, err := path.ImageIndex()
	if err != nil {
		return nil, false, fmt.Errorf("reading image cache index: %w", err)
	}

	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, false, fmt.Errorf("parsing image cache index: %w", err)
	}

	matches := matchImage(ref, platform)
	for _, desc := range manifest.Manifests {
		if !matches(desc) {
			continue
		}

		img, err := path.Image(desc.Digest)
		if err != nil {
			return nil, false, fmt.Errorf("reading cached image %s: %w", ref, err)
		}

		return img, true, nil
	}

	return nil, false, nil
}
