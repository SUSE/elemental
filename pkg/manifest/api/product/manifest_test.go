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

		Expect(rm.MetaData).ToNot(BeNil())
		Expect(rm.MetaData.Name).To(Equal("suse-edge"))
		Expect(rm.MetaData.Version).To(Equal("3.2.0"))
		Expect(len(rm.MetaData.UpgradePathsFrom)).To(Equal(1))
		Expect(rm.MetaData.UpgradePathsFrom[0]).To(Equal("3.1.2"))
		Expect(rm.MetaData.CreationDate).To(Equal("2025-01-20"))

		Expect(rm.CorePlatform).ToNot(BeNil())
		Expect(rm.CorePlatform.Image).To(Equal("foo.example.com/bar/release-manifest"))
		Expect(rm.CorePlatform.Version).To(Equal("1.0"))

		Expect(rm.Components.Helm).ToNot(BeNil())
		Expect(len(rm.Components.Helm.Charts)).To(Equal(1))
		Expect(rm.Components.Helm.Charts[0].Name).To(Equal("Bar"))
		Expect(rm.Components.Helm.Charts[0].Chart).To(Equal("bar"))
		Expect(rm.Components.Helm.Charts[0].Version).To(Equal("0.0.0"))
		Expect(rm.Components.Helm.Charts[0].Namespace).To(Equal("bar-system"))
		Expect(rm.Components.Helm.Charts[0].Values).To(Equal(map[string]any{"image": map[string]any{"tag": "latest"}}))
		Expect(len(rm.Components.Helm.Charts[0].DependsOn)).To(Equal(1))
		Expect(rm.Components.Helm.Charts[0].DependsOn[0]).To(Equal("foo"))
		Expect(len(rm.Components.Helm.Charts[0].Images)).To(Equal(1))
		Expect(rm.Components.Helm.Charts[0].Images[0].Name).To(Equal("bar"))
		Expect(rm.Components.Helm.Charts[0].Images[0].Image).To(Equal("registry.com/bar/bar:0.0.0"))
		Expect(len(rm.Components.Helm.Repositories)).To(Equal(1))
		Expect(rm.Components.Helm.Repositories[0].Name).To(Equal("bar-charts"))
		Expect(rm.Components.Helm.Repositories[0].URL).To(Equal("https://bar.github.io/charts"))
	})

	It("fails when unknown field is introduced", func() {
		expErrMsg := "unmarshaling 'product' release manifest: error unmarshaling JSON: while decoding JSON: json: unknown field \"operatingSystem\""
		data := []byte(unknownFieldManifest)
		rm, err := product.Parse(data)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErrMsg))
		Expect(rm).To(BeNil())
	})

	It("fails when 'corePlatform' is missing", func() {
		expErrMsg := "missing 'corePlatform' field"
		missingBasePlatformManifest := ""
		data := []byte(missingBasePlatformManifest)
		rm, err := product.Parse(data)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErrMsg))
		Expect(rm).To(BeNil())
	})
})
