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

const brokenManifest = `
metadata:
  name: "suse-edge"
  version: "3.2.0"
components:
  operatingSystem:
    image:
      base: foo.bar:latest
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

		Expect(rm.Schema).To(BeEquivalentTo("v0"))

		Expect(rm.Metadata).ToNot(BeNil())
		Expect(rm.Metadata.Name).To(Equal("suse-core"))
		Expect(rm.Metadata.Version).To(Equal("1.0"))
		Expect(rm.Metadata.CreationDate).To(Equal("2000-01-01"))

		Expect(rm.Components).ToNot(BeNil())
		Expect(rm.Components.OperatingSystem).ToNot(BeNil())
		Expect(rm.Components.OperatingSystem.Image.Base).To(Equal("registry.com/foo/bar/os-base:6.2"))
		Expect(rm.Components.OperatingSystem.Image.ISO).To(Equal("registry.com/foo/bar/installer-iso:6.2"))

		Expect(rm.Components.Systemd.Extensions).To(HaveLen(1))
		Expect(rm.Components.Systemd.Extensions[0].Name).To(Equal("elemental3ctl"))
		Expect(rm.Components.Systemd.Extensions[0].Image).To(Equal("https://example.com/elemental3ctl_0.0.raw"))
		Expect(rm.Components.Systemd.Extensions[0].Required).To(BeTrue())

		Expect(rm.Components.Kubernetes).ToNot(BeNil())
		Expect(rm.Components.Kubernetes.Version).To(Equal("v1.35.0+rke2r1"))
		Expect(rm.Components.Kubernetes.Image).To(Equal("registry.example.com/rke2:1.35_1.0"))

		Expect(rm.Components.Helm).ToNot(BeNil())
		Expect(len(rm.Components.Helm.Charts)).To(Equal(1))
		Expect(rm.Components.Helm.Charts[0].Name).To(Equal("Foo"))
		Expect(rm.Components.Helm.Charts[0].Chart).To(Equal("foo"))
		Expect(rm.Components.Helm.Charts[0].Version).To(Equal("0.0.0"))
		Expect(rm.Components.Helm.Charts[0].Namespace).To(Equal("foo-system"))
		Expect(rm.Components.Helm.Charts[0].Values).To(Equal(map[string]any{"image": map[string]any{"tag": "latest"}}))
		Expect(len(rm.Components.Helm.Charts[0].DependsOn)).To(Equal(1))
		Expect(rm.Components.Helm.Charts[0].DependsOn[0].Name).To(Equal("baz"))
		Expect(rm.Components.Helm.Charts[0].DependsOn[0].Type).To(BeEquivalentTo("helm"))
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

	It("defaults to schema v0 when schema field is missing", func() {
		data := []byte(`
components:
  operatingSystem:
    image:
      base: "registry.com/foo/bar/os-base:6.2"
      iso: "registry.com/foo/bar/installer-iso:6.2"
`)
		rm, err := core.Parse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm).ToNot(BeNil())
	})

	It("succeeds with explicit schema v0", func() {
		data := []byte(`
schema: v0
components:
  operatingSystem:
    image:
      base: "registry.com/foo/bar/os-base:6.2"
      iso: "registry.com/foo/bar/installer-iso:6.2"
`)
		rm, err := core.Parse(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm).ToNot(BeNil())
		Expect(rm.Schema).To(BeEquivalentTo("v0"))
	})

	It("fails with unknown schema version", func() {
		data := []byte(`
schema: v99
components:
  operatingSystem:
    image:
      base: "registry.com/foo/bar/os-base:6.2"
      iso: "registry.com/foo/bar/installer-iso:6.2"
`)
		rm, err := core.Parse(data)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`unsupported manifest schema version: "v99"`))
		Expect(rm).To(BeNil())
	})

	It("fails when manifest is broken", func() {
		expErrors := []string{
			"field \"ReleaseManifest.components.operatingSystem.image.iso\" is required",
			"field \"ReleaseManifest.components.systemd.extensions[0].image\" is required",
			"field \"ReleaseManifest.components.helm.charts[0].dependsOn[0].type\" must be one of [sysext helm], but got \"broken\"",
		}

		data := []byte(brokenManifest)
		rm, err := core.Parse(data)
		Expect(err).To(HaveOccurred())

		errMsg := err.Error()
		for _, msg := range expErrors {
			Expect(errMsg).To(ContainSubstring(msg))
		}

		Expect(rm).To(BeNil())
	})
})
