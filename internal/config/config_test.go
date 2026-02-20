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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/config"
	v0 "github.com/suse/elemental/v3/internal/config/v0"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
)

var _ = Describe("Schema", func() {

	It("Successfully loads a schema version", func() {
		var configDir v0.Dir = "/config"
		fs, cleanup, err := sysmock.TestFS(map[string]any{
			configDir.InstallFilepath(): "schema: v0",
		})
		Expect(err).ToNot(HaveOccurred())
		defer cleanup()

		Expect(err).ToNot(HaveOccurred())
		schemaVersion, err := config.LoadSchemaVersion(fs, string(configDir))
		Expect(err).ToNot(HaveOccurred())
		Expect(schemaVersion).To(Equal(config.SchemaV0))
	})

	It("Fails to load schema from missing file", func() {
		fs, cleanup, err := sysmock.TestFS(map[string]any{})
		Expect(err).ToNot(HaveOccurred())
		defer cleanup()

		Expect(err).ToNot(HaveOccurred())
		schemaVersion, err := config.LoadSchemaVersion(fs, "/missing-config")
		Expect(err).To(HaveOccurred())
		Expect(schemaVersion).To(BeEmpty())
	})

	It("Fails to load an unknown schema version", func() {
		var configDir v0.Dir = "/config"
		fs, cleanup, err := sysmock.TestFS(map[string]any{
			configDir.InstallFilepath(): "schema: v99",
		})
		Expect(err).ToNot(HaveOccurred())
		defer cleanup()

		Expect(err).ToNot(HaveOccurred())
		schemaVersion, err := config.LoadSchemaVersion(fs, string(configDir))
		Expect(err).To(HaveOccurred())
		Expect(schemaVersion).To(BeEmpty())
	})
})
