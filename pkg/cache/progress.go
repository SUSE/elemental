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
	"io"

	containerregistry "github.com/google/go-containerregistry/pkg/v1"
	"github.com/schollz/progressbar/v3"
)

type progressImage struct {
	containerregistry.Image
	bar *progressbar.ProgressBar
}

func withProgress(img containerregistry.Image) (*progressImage, error) {
	manifest, err := img.Manifest()
	if err != nil {
		return nil, err
	}

	var total int64
	for _, layer := range manifest.Layers {
		total += layer.Size
	}

	return &progressImage{Image: img, bar: progressbar.DefaultBytes(total, "Downloading")}, nil
}

func (p *progressImage) Close() error {
	return p.bar.Close()
}

func (p *progressImage) Layers() ([]containerregistry.Layer, error) {
	layers, err := p.Image.Layers()
	if err != nil {
		return nil, err
	}

	wrapped := make([]containerregistry.Layer, 0, len(layers))
	for _, layer := range layers {
		wrapped = append(wrapped, &progressLayer{Layer: layer, bar: p.bar})
	}

	return wrapped, nil
}

func (p *progressImage) LayerByDigest(h containerregistry.Hash) (containerregistry.Layer, error) {
	layer, err := p.Image.LayerByDigest(h)
	if err != nil {
		return nil, err
	}

	return &progressLayer{Layer: layer, bar: p.bar}, nil
}

func (p *progressImage) LayerByDiffID(h containerregistry.Hash) (containerregistry.Layer, error) {
	layer, err := p.Image.LayerByDiffID(h)
	if err != nil {
		return nil, err
	}

	return &progressLayer{Layer: layer, bar: p.bar}, nil
}

type progressLayer struct {
	containerregistry.Layer
	bar *progressbar.ProgressBar
}

func (l *progressLayer) Compressed() (io.ReadCloser, error) {
	rc, err := l.Layer.Compressed()
	if err != nil {
		return nil, err
	}

	r := progressbar.NewReader(rc, l.bar)

	return &r, nil
}
