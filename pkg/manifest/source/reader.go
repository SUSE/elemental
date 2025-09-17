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

package source

import (
	"fmt"
	"os"
)

type OCIFileExtractor interface {
	// ExtractFrom extracts a file from a given OCI image and
	// returns the path to the extracted file.
	ExtractFrom(uri string, local bool) (path string, err error)
}

type ReleaseManifestReader struct {
	extractor OCIFileExtractor
	local     bool
}

func NewReader(ociFileExtractor OCIFileExtractor, local bool) *ReleaseManifestReader {
	return &ReleaseManifestReader{
		extractor: ociFileExtractor,
		local:     local,
	}
}

func (r *ReleaseManifestReader) Read(src *ReleaseManifestSource) ([]byte, error) {
	switch src.Type() {
	case File:
		return r.readLocal(src.URI())
	case OCI:
		filepath, err := r.extractor.ExtractFrom(src.URI(), r.local)
		if err != nil {
			return nil, fmt.Errorf("extracting file from OCI image '%s': %w", src.URI(), err)
		}
		return r.readLocal(filepath)
	default:
		return nil, fmt.Errorf("unsupported source type: '%s'", src.Type())
	}
}

func (r *ReleaseManifestReader) readLocal(path string) ([]byte, error) {
	return os.ReadFile(path)
}
