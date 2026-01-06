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

package source

import (
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
)

type ReleaseManifestSourceType int

const (
	File ReleaseManifestSourceType = iota + 1
	OCI
)

func (r ReleaseManifestSourceType) String() string {
	switch r {
	case File:
		return "file"
	case OCI:
		return "oci"
	default:
		return "unknown"
	}
}

func ParseType(str string) (ReleaseManifestSourceType, error) {
	switch str {
	case File.String():
		return File, nil
	case OCI.String():
		return OCI, nil
	default:
		return ReleaseManifestSourceType(0), fmt.Errorf("manifest source type '%s' is not supported. Supported source types: '%s', '%s'", str, File, OCI)
	}
}

type ReleaseManifestSource struct {
	uri     string
	srcType ReleaseManifestSourceType
}

// ParseFromURI validates the given URI and parses it to a release manifest source
func ParseFromURI(uri string) (*ReleaseManifestSource, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest source uri: %w", err)
	}

	if u.Scheme == "" {
		return nil, fmt.Errorf("missing scheme in source uri: '%s'", uri)
	}

	srcType, err := ParseType(u.Scheme)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest source type: %w", err)
	}

	source := u.Opaque
	if source == "" {
		source = filepath.Join(u.Host, u.Path)
	}

	switch srcType {
	case File:
		source = filepath.Clean(source)
	case OCI:
		if _, err := name.ParseReference(source); err != nil {
			return nil, fmt.Errorf("invalid OCI image reference: %w", err)
		}
	}

	return &ReleaseManifestSource{
		uri:     source,
		srcType: srcType,
	}, nil
}

func (r *ReleaseManifestSource) URI() string {
	return r.uri
}

func (r *ReleaseManifestSource) Type() ReleaseManifestSourceType {
	return r.srcType
}
