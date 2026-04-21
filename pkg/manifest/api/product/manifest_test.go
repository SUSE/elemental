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

package product_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/manifest/api/product"
)

const unknownFieldManifest = `
metadata:
  name: "suse-edge"
  version: "3.2.0"
  upgradePathsFrom: 
  - "3.1.2"
  creationDate: "2025-01-20"
components:
  operatingSystem:
    version: "6.2"
    image: "registry.com/foo/bar/sl-micro:6.2"
`

const brokenManifest = `
metadata:
  name: "suse-edge"
  version: "3.2.0"
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

func TestProductManifestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Product Release Manifest API test suite")
}

var _ = Describe("ReleaseManifest", Label("release-manifest"), func() {
	It("is parsed correctly", func() {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "full_product_release_manifest.yaml"))
		Expect(err).NotTo(HaveOccurred())

		rm, err := product.Parse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm).ToNot(BeNil())

		Expect(rm.Schema).To(BeEquivalentTo("v0"))

		Expect(rm.Metadata).ToNot(BeNil())
		Expect(rm.Metadata.Name).To(Equal("suse-edge"))
		Expect(rm.Metadata.Version).To(Equal("3.2.0"))
		Expect(rm.Metadata.CreationDate).To(Equal("2025-01-20"))

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
		rm, err := product.Parse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm).ToNot(BeNil())
	})

	It("succeeds with explicit schema v0", func() {
		data := []byte(`
schema: v0
corePlatform:
  image: "foo.example.com/bar/release-manifest:1.0"
`)
		rm, err := product.Parse(data)
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
		rm, err := product.Parse(data)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`unsupported manifest schema version: "v99"`))
		Expect(rm).To(BeNil())
	})

	It("fails when unknown field is introduced", func() {
		expErrMsg := "field operatingSystem not found in type product.Components"
		data := []byte(unknownFieldManifest)
		rm, err := product.Parse(data)
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
		rm, err := product.Parse(data)
		Expect(err).To(HaveOccurred())

		errMsg := err.Error()
		for _, msg := range expErrors {
			Expect(errMsg).To(ContainSubstring(msg))
		}

		Expect(rm).To(BeNil())
	})
})
