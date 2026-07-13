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


package action

import (
	"fmt"
	"path/filepath"
)

func resolveCacheDir(cache bool, cacheDir string) (string, error) {
	if !cache {
		return "", nil
	}

	if cacheDir == "" {
		return "", fmt.Errorf("cache directory not set")
	}

	abs, err := filepath.Abs(cacheDir)
	if err != nil {
		return "", fmt.Errorf("resolving cache directory %q: %w", cacheDir, err)
	}

	return abs, nil
}
