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

package cmd

import (
	"fmt"
	"path/filepath"
)

const defaultCacheDir = "/elemental-cache"

func ValidateCacheFlags(cache bool, cacheDir string, offline bool) error {
	if cache {
		return nil
	}

	if offline {
		return fmt.Errorf("--offline requires --cache")
	}
	if cacheDir != "" && filepath.Clean(cacheDir) != defaultCacheDir {
		return fmt.Errorf("--cache-dir requires --cache")
	}

	return nil
}
