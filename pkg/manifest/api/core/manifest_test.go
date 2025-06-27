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

package core_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/manifest/api/core"
)

const invalidManifest = `
metadata:
  name: "suse-core"
  version: "0.0.1"
  upgradePathsFrom: 
  - "0.0.1-rc"
  creationDate: "2000-01-01"
corePlatform:
  name: "suse-edge"
  version: "0.0.0"
`

func TestCoreManifestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Core Release Manifest API test suite")
}

var _ = Describe("ReleaseManifest", Label("release-manifest"), func() {
	It("is parsed correctly", func() {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "full_core_release_manifest.yaml"))
		Expect(err).NotTo(HaveOccurred())

		rm, err := core.Parse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm).ToNot(BeNil())

		Expect(rm.MetaData).ToNot(BeNil())
		Expect(rm.MetaData.Name).To(Equal("suse-core"))
		Expect(rm.MetaData.Version).To(Equal("1.0"))
		Expect(len(rm.MetaData.UpgradePathsFrom)).To(Equal(1))
		Expect(rm.MetaData.UpgradePathsFrom[0]).To(Equal("0.0.1"))
		Expect(rm.MetaData.CreationDate).To(Equal("2000-01-01"))

		Expect(rm.Components).ToNot(BeNil())
		Expect(rm.Components.OperatingSystem).ToNot(BeNil())
		Expect(rm.Components.OperatingSystem.Version).To(Equal("6.2"))
		Expect(rm.Components.OperatingSystem.Image).To(Equal("registry.com/foo/bar/sl-micro:6.2"))

		Expect(rm.Components.Kubernetes).ToNot(BeNil())
		Expect(rm.Components.Kubernetes.RKE2).ToNot(BeNil())
		Expect(rm.Components.Kubernetes.RKE2.Version).To(Equal("1.32"))
		Expect(rm.Components.Kubernetes.RKE2.Image).To(Equal("registry.com/foo/bar/rke2:1.32"))

		Expect(rm.Components.Helm).ToNot(BeNil())
		Expect(len(rm.Components.Helm.Charts)).To(Equal(1))
		Expect(rm.Components.Helm.Charts[0].Name).To(Equal("Foo"))
		Expect(rm.Components.Helm.Charts[0].Chart).To(Equal("foo"))
		Expect(rm.Components.Helm.Charts[0].Version).To(Equal("0.0.0"))
		Expect(rm.Components.Helm.Charts[0].Namespace).To(Equal("foo-system"))
		Expect(rm.Components.Helm.Charts[0].Values).To(Equal(map[string]any{"image": map[string]any{"tag": "latest"}}))
		Expect(len(rm.Components.Helm.Charts[0].DependsOn)).To(Equal(1))
		Expect(rm.Components.Helm.Charts[0].DependsOn[0]).To(Equal("baz"))
		Expect(len(rm.Components.Helm.Charts[0].Images)).To(Equal(1))
		Expect(rm.Components.Helm.Charts[0].Images[0].Name).To(Equal("foo"))
		Expect(rm.Components.Helm.Charts[0].Images[0].Image).To(Equal("registry.com/foo/foo:0.0.0"))
		Expect(len(rm.Components.Helm.Repositories)).To(Equal(1))
		Expect(rm.Components.Helm.Repositories[0].Name).To(Equal("foo-charts"))
		Expect(rm.Components.Helm.Repositories[0].URL).To(Equal("https://foo.github.io/charts"))
	})

	It("fails when unknown field is introduced", func() {
		expErrMsg := "field corePlatform not found in type core.ReleaseManifest"
		data := []byte(invalidManifest)
		rm, err := core.Parse(data)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(expErrMsg))
		Expect(rm).To(BeNil())
	})
})
