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

package api_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/manifest/api"
)

var _ = Describe("LoadSchemaVersion", Label("release-manifest"), func() {
	It("defaults to v0 when schema field is missing", func() {
		data := []byte(`
metadata:
  name: "test"
`)
		version, err := api.LoadSchemaVersion(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(Equal(api.SchemaV0))
	})

	It("returns v0 when schema is explicitly set to v0", func() {
		data := []byte(`
schema: v0
metadata:
  name: "test"
`)
		version, err := api.LoadSchemaVersion(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(Equal(api.SchemaV0))
	})

	It("fails with an unsupported schema version", func() {
		data := []byte(`
schema: v99
metadata:
  name: "test"
`)
		version, err := api.LoadSchemaVersion(data)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`unsupported manifest schema version: "v99"`))
		Expect(version).To(BeEquivalentTo(""))
	})

	It("fails with malformed YAML", func() {
		data := []byte(`]]]`)
		version, err := api.LoadSchemaVersion(data)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("extracting schema version"))
		Expect(version).To(BeEquivalentTo(""))
	})
})
