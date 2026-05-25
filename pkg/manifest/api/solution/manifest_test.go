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

package solution_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/manifest/api/solution"
)

const unknownFieldManifest = `
schema: v1
metadata:
  name: "suse-edge"
  version: "3.2.0"
lifecycle:
  availabilityDate: "2025-01-20"
  fullSupportEndDate: "2026-01-20"
  maintenanceSupportEndDate: "2027-01-20"
components:
  operatingSystem:
    version: "6.2"
    image: "registry.com/foo/bar/sl-micro:6.2"
`

const brokenManifest = `
schema: v1
metadata:
  name: "suse-edge"
  version: "3.2.0"
lifecycle:
  availabilityDate: "2025-01-20"
  fullSupportEndDate: "2026-01-20"
  maintenanceSupportEndDate: "2027-01-20"
components:
  systemd:
    extensions:
    - name: "missing_img"
  helm:
    charts:
    - version: "0.0"
      chart: "oci://foo.bar"
      dependsOn:
      - name: "bar"
        type: "broken"
`

const missingLifecycleManifest = `
schema: v1
corePlatform:
  image: "foo.example.com/bar/release-manifest:1.0"
`

const badDateManifest = `
schema: v1
lifecycle:
  availabilityDate: "not-a-date"
  fullSupportEndDate: "2026-01-20"
  maintenanceSupportEndDate: "2027-01-20"
corePlatform:
  image: "foo.example.com/bar/release-manifest:1.0"
`

func TestSolutionManifestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Solution Release Manifest API test suite")
}

var _ = Describe("ReleaseManifest", Label("release-manifest"), func() {
	It("is parsed correctly", func() {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "full_solution_release_manifest.yaml"))
		Expect(err).NotTo(HaveOccurred())

		rm, err := solution.Parse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm).ToNot(BeNil())

		Expect(rm.Schema).To(BeEquivalentTo("v1"))

		Expect(rm.Metadata).ToNot(BeNil())
		Expect(rm.Metadata.Name).To(Equal("suse-edge"))
		Expect(rm.Metadata.Version).To(Equal("3.2.0"))

		Expect(rm.Lifecycle).ToNot(BeNil())
		Expect(rm.Lifecycle.AvailabilityDate).To(Equal("2025-01-20"))
		Expect(rm.Lifecycle.FullSupportEndDate).To(Equal("2026-01-20"))
		Expect(rm.Lifecycle.MaintenanceSupportEndDate).To(Equal("2027-01-20"))

		Expect(rm.CorePlatform).ToNot(BeNil())
		Expect(rm.CorePlatform.Image).To(Equal("foo.example.com/bar/release-manifest:1.0"))

		Expect(rm.Components.Systemd.Extensions).To(HaveLen(1))
		Expect(rm.Components.Systemd.Extensions[0].Name).To(Equal("foo-ext"))
		Expect(rm.Components.Systemd.Extensions[0].Image).To(Equal("https://example.com/foo-ext_0.0.raw"))
		Expect(rm.Components.Systemd.Extensions[0].Required).To(BeFalse())

		Expect(rm.Components.Helm).ToNot(BeNil())
		Expect(len(rm.Components.Helm.Charts)).To(Equal(1))
		Expect(rm.Components.Helm.Charts[0].Name).To(Equal("Bar"))
		Expect(rm.Components.Helm.Charts[0].Chart).To(Equal("bar"))
		Expect(rm.Components.Helm.Charts[0].Version).To(Equal("0.0.0"))
		Expect(rm.Components.Helm.Charts[0].Namespace).To(Equal("bar-system"))
		Expect(rm.Components.Helm.Charts[0].Values).To(Equal(map[string]any{"image": map[string]any{"tag": "latest"}}))
		Expect(len(rm.Components.Helm.Charts[0].DependsOn)).To(Equal(2))
		Expect(rm.Components.Helm.Charts[0].DependsOn[0].Name).To(Equal("foo"))
		Expect(rm.Components.Helm.Charts[0].DependsOn[0].Type).To(BeEquivalentTo("helm"))
		Expect(rm.Components.Helm.Charts[0].DependsOn[1].Name).To(Equal("bar"))
		Expect(rm.Components.Helm.Charts[0].DependsOn[1].Type).To(BeEquivalentTo("sysext"))
		Expect(len(rm.Components.Helm.Charts[0].Images)).To(Equal(1))
		Expect(rm.Components.Helm.Charts[0].Images[0].Name).To(Equal("bar"))
		Expect(rm.Components.Helm.Charts[0].Images[0].Image).To(Equal("registry.com/bar/bar:0.0.0"))
		Expect(len(rm.Components.Helm.Repositories)).To(Equal(1))
		Expect(rm.Components.Helm.Repositories[0].Name).To(Equal("bar-charts"))
		Expect(rm.Components.Helm.Repositories[0].URL).To(Equal("https://bar.github.io/charts"))
	})

	It("defaults to schema v0 when schema field is missing", func() {
		data := []byte(`
corePlatform:
  image: "foo.example.com/bar/release-manifest:1.0"
`)
		rm, err := solution.Parse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm).ToNot(BeNil())
		Expect(rm.Schema).To(BeEquivalentTo(""))
	})

	It("succeeds with explicit schema v0", func() {
		data := []byte(`
schema: v0
corePlatform:
  image: "foo.example.com/bar/release-manifest:1.0"
`)
		rm, err := solution.Parse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm).ToNot(BeNil())
		Expect(rm.Schema).To(BeEquivalentTo("v0"))
	})

	It("fails with unknown schema version", func() {
		data := []byte(`
schema: v99
corePlatform:
  image: "foo.example.com/bar/release-manifest:1.0"
`)
		rm, err := solution.Parse(data)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`unsupported manifest schema version: "v99"`))
		Expect(rm).To(BeNil())
	})

	It("allows metadata.creationDate under schema v0", func() {
		data := []byte(`
schema: v0
metadata:
  name: "suse-edge"
  version: "3.2.0"
  creationDate: "2025-01-20"
corePlatform:
  image: "foo.example.com/bar/release-manifest:1.0"
`)
		rm, err := solution.Parse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm.Metadata.CreationDate).To(Equal("2025-01-20"))
	})

	It("allows metadata.creationDate when schema is omitted (default v0)", func() {
		data := []byte(`
metadata:
  name: "suse-edge"
  version: "3.2.0"
  creationDate: "2025-01-20"
corePlatform:
  image: "foo.example.com/bar/release-manifest:1.0"
`)
		rm, err := solution.Parse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm.Metadata.CreationDate).To(Equal("2025-01-20"))
	})

	It("rejects metadata.creationDate under schema v1", func() {
		data := []byte(`
schema: v1
metadata:
  name: "suse-edge"
  version: "3.2.0"
  creationDate: "2025-01-20"
lifecycle:
  availabilityDate: "2025-01-20"
corePlatform:
  image: "foo.example.com/bar/release-manifest:1.0"
`)
		rm, err := solution.Parse(data)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`field "metadata.creationDate" is not allowed in schema "v1"`))
		Expect(rm).To(BeNil())
	})

	It("succeeds when the lifecycle section is omitted", func() {
		data := []byte(missingLifecycleManifest)
		rm, err := solution.Parse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm).ToNot(BeNil())
		Expect(rm.Lifecycle).To(BeNil())
	})

	It("fails when a lifecycle date is malformed", func() {
		data := []byte(badDateManifest)
		rm, err := solution.Parse(data)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`field "ReleaseManifest.lifecycle.availabilityDate" must be a date in YYYY-MM-DD format, but got "not-a-date"`))
		Expect(rm).To(BeNil())
	})

	It("fails when lifecycle is present but availabilityDate is missing", func() {
		data := []byte(`
schema: v1
lifecycle:
  fullSupportEndDate: "2026-01-20"
corePlatform:
  image: "foo.example.com/bar/release-manifest:1.0"
`)
		rm, err := solution.Parse(data)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`field "ReleaseManifest.lifecycle.availabilityDate" is required`))
		Expect(rm).To(BeNil())
	})

	It("accepts a lifecycle block with only availabilityDate", func() {
		data := []byte(`
schema: v1
lifecycle:
  availabilityDate: "2025-01-20"
corePlatform:
  image: "foo.example.com/bar/release-manifest:1.0"
`)
		rm, err := solution.Parse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm).ToNot(BeNil())
		Expect(rm.Lifecycle).ToNot(BeNil())
		Expect(rm.Lifecycle.AvailabilityDate).To(Equal("2025-01-20"))
		Expect(rm.Lifecycle.FullSupportEndDate).To(BeEmpty())
	})

	It("fails when unknown field is introduced", func() {
		expErrMsg := "field operatingSystem not found in type solution.Components"
		data := []byte(unknownFieldManifest)
		rm, err := solution.Parse(data)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(expErrMsg))
		Expect(rm).To(BeNil())
	})

	It("fails when manifest is broken", func() {
		expErrors := []string{
			"field \"ReleaseManifest.corePlatform\" is required",
			"field \"ReleaseManifest.components.systemd.extensions[0].image\" is required",
			"field \"ReleaseManifest.components.helm.charts[0].dependsOn[0].type\" must be one of [sysext helm], but got \"broken\"",
		}

		data := []byte(brokenManifest)
		rm, err := solution.Parse(data)
		Expect(err).To(HaveOccurred())

		errMsg := err.Error()
		for _, msg := range expErrors {
			Expect(errMsg).To(ContainSubstring(msg))
		}

		Expect(rm).To(BeNil())
	})
})
