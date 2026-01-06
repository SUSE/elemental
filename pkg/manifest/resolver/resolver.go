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

package resolver

import (
	"errors"
	"fmt"

	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/api/product"
	"github.com/suse/elemental/v3/pkg/manifest/source"
)

type ResolvedManifest struct {
	// Release manifest for the core platform
	CorePlatform *core.ReleaseManifest
	// Product release manifest that extends the core platform
	ProductExtension *product.ReleaseManifest
}

type SourceReader interface {
	// Read reads a release manifest from the given source and returns the file contents
	Read(m *source.ReleaseManifestSource) ([]byte, error)
}

type Resolver struct {
	sourceReader SourceReader
}

func New(reader SourceReader) *Resolver {
	return &Resolver{
		sourceReader: reader,
	}
}

// Resolve resolves a release manifest at a given uri to its
// underlying component parts (i.e. product and core platform)
func (r *Resolver) Resolve(uri string) (*ResolvedManifest, error) {
	resolved := &ResolvedManifest{}
	if err := r.resolveRecursive(uri, resolved); err != nil {
		return nil, err
	}

	return resolved, nil
}

func (r *Resolver) resolveRecursive(uri string, rm *ResolvedManifest) error {
	rmSrc, err := source.ParseFromURI(uri)
	if err != nil {
		return fmt.Errorf("unable to convert uri '%s' to manifest source: %w", uri, err)
	}

	data, err := r.sourceReader.Read(rmSrc)
	if err != nil {
		return fmt.Errorf("reading manifest from source '%s': %w", rmSrc.URI(), err)
	}

	if len(data) == 0 {
		return fmt.Errorf("empty file passed as release manifest: '%s'", rmSrc.URI())
	}

	productManifest, prodErr := product.Parse(data)
	if prodErr != nil {
		coreManifest, coreErr := core.Parse(data)
		if coreErr != nil {
			combinedErr := errors.Join(prodErr, coreErr)
			return fmt.Errorf("unable to parse '%s' as a valid release manifest: %w", rmSrc.URI(), combinedErr)
		}

		rm.CorePlatform = coreManifest
		return nil
	}
	rm.ProductExtension = productManifest

	coreReleaseManifestOCI := fmt.Sprintf("%s://%s", source.OCI, rm.ProductExtension.CorePlatform.Image)
	return r.resolveRecursive(coreReleaseManifestOCI, rm)
}
