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

	"github.com/suse/elemental/v3/pkg/log"
)

// Check and normalize URIs, converting relative file URIs to canonical
// file:///abs/path/filename syntax.
func ResolveURI(ref, baseDir string, logger log.Logger) (string, error) {
	// legacy-file-dot-uri
	if rewritten, warn := RewriteLegacyRelativeURI(ref); warn {
		logger.Warn("%s", LegacyRelativeWarning(ref, rewritten))
		ref = rewritten
	}

	uri, err := resolveURI(ref, baseDir, logger)
	if err != nil {
		return "", err
	}

	if _, err = ParseFromURI(uri); err != nil {
		return "", err
	}

	logger.Debug("Resolved manifest URI %q to %q", ref, uri)
	return uri, nil
}

func resolveURI(ref, baseDir string, logger log.Logger) (string, error) {
	u, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("parsing manifest uri: %w", err)
	}

	switch u.Scheme {
	case "":
		absBase, err := filepath.Abs(baseDir)
		if err != nil {
			return "", fmt.Errorf("resolving base directory %q: %w", baseDir, err)
		}

		// The trailing slash makes the base a directory, so references resolve
		// inside it instead of replacing its last segment
		base := &url.URL{Scheme: File.String(), Path: absBase + "/"}
		u = base.ResolveReference(u)
	case File.String():
	default:
		return ref, nil
	}

	p, err := filePath(ref, u)
	if err != nil {
		return "", err
	}
	return FileURI(p)
}

func FileURI(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path of %q: %w", p, err)
	}
	return (&url.URL{Scheme: File.String(), Path: abs}).String(), nil
}

// filePath returns the cleaned filesystem path a local file URI refers to.
func filePath(uri string, u *url.URL) (string, error) {
	if u.Opaque != "" || (u.Host != "" && u.Host != "localhost") {
		return "", fmt.Errorf("unsupported file uri %q: use file:///<path> or a relative reference", uri)
	}
	return filepath.Clean(u.Path), nil
}
