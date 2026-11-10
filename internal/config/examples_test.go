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

package config_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/config"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/api/solution"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

// examplesDir holds the example configuration directories shipped with the repository.
// Every directory in it must remain a valid configuration directory, so that changes to
// the configuration parsing or validation rules also update the examples.
const examplesDir = "../../examples/elemental/customize"

var _ = Describe("Example configuration directories", Label("examples"), func() {
	dirs, err := exampleDirs()

	It("are found", func() {
		Expect(err).ToNot(HaveOccurred())
		Expect(dirs).ToNot(BeEmpty(), "no example configuration directories found in %s", examplesDir)
	})

	for _, dir := range dirs {
		It("parses and validates "+filepath.Base(dir), func() {
			conf, err := config.Parse(vfs.New(), dir)
			Expect(err).ToNot(HaveOccurred())

			uri := conf.Release.ManifestURI
			if !strings.HasPrefix(uri, "file://") {
				return
			}

			manifestPath := strings.TrimPrefix(uri, "file://")
			if !filepath.IsAbs(manifestPath) {
				manifestPath = filepath.Join(dir, manifestPath)
			}
			data, err := os.ReadFile(manifestPath)
			Expect(err).ToNot(HaveOccurred(), "reading release manifest referenced by %s", uri)

			_, solutionErr := solution.Parse(data)
			if solutionErr == nil {
				return
			}
			_, coreErr := core.Parse(data)
			Expect(coreErr).ToNot(HaveOccurred(),
				"release manifest %s is neither a valid solution (%v) nor core platform manifest", manifestPath, solutionErr)
		})
	}
})

// exampleDirs returns the example configuration directories, ignoring any files and
// leftovers from local runs (e.g. caches) that are not configuration directories.
func exampleDirs() ([]string, error) {
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		return nil, err
	}

	dirs := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(examplesDir, entry.Name()))
		}
	}
	return dirs, nil
}
