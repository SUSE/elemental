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

package resolver_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/manifest/source"
)

const (
	corePlatformRef              = "foo.example.com/bar/release-manifest"
	corePlatformVersion          = "1.0"
	expectedCorePlatformImage    = corePlatformRef + ":" + corePlatformVersion
	expectedProductManifestImage = "prod.example.com/bar/release-manifest:0.0.1"
)

var coreManifestPath = filepath.Join("..", "testdata", "full_core_release_manifest.yaml")
var prodManifestPath = filepath.Join("..", "testdata", "full_product_release_manifest.yaml")

func TestResolverSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Release Manifest Resolver test suite")
}

var _ = Describe("Resolver", Label("release-manifest"), func() {
	var reader *SourceReaderMock
	var res *resolver.Resolver
	BeforeEach(func() {
		reader = &SourceReaderMock{}
		res = resolver.New(reader)
	})

	It("resolves a 'product' release manifest correctly", func() {
		By("reading the manifest source from a local file")
		prodManifestFile := fmt.Sprintf("%s://%s", source.File, prodManifestPath)
		resolvedManifest, err := res.Resolve(prodManifestFile)
		Expect(err).ToNot(HaveOccurred())
		Expect(resolvedManifest).ToNot(BeNil())
		validateResolvedManifest(resolvedManifest, false)

		By("reading the manifest source from an OCI image")
		prodManifestOCI := fmt.Sprintf("%s://%s", source.OCI, expectedProductManifestImage)
		resolvedManifest, err = res.Resolve(prodManifestOCI)
		Expect(err).ToNot(HaveOccurred())
		Expect(resolvedManifest).ToNot(BeNil())
		validateResolvedManifest(resolvedManifest, false)
	})

	It("resolves a 'core' release manifest correctly", func() {
		By("reading the manifest source from a local file")
		coreManifestFile := fmt.Sprintf("%s://%s", source.File, coreManifestPath)
		resolvedManifest, err := res.Resolve(coreManifestFile)
		Expect(err).ToNot(HaveOccurred())
		Expect(resolvedManifest).ToNot(BeNil())
		validateResolvedManifest(resolvedManifest, true)

		By("reading the manifest source from an OCI image")
		coreManifestOCI := fmt.Sprintf("%s://%s", source.OCI, expectedCorePlatformImage)
		resolvedManifest, err = res.Resolve(coreManifestOCI)
		Expect(err).ToNot(HaveOccurred())
		Expect(resolvedManifest).ToNot(BeNil())
		validateResolvedManifest(resolvedManifest, true)
	})

	It("fails to convert to a manifest source", func() {
		By("referring an invalid file")
		invalidFile := fmt.Sprintf("%s://foo /invalid/file.yaml", source.File)
		expErrSub := "unable to convert uri 'file://foo /invalid/file.yaml' to manifest source"
		r, err := res.Resolve(invalidFile)
		Expect(err).ToNot(BeNil())
		Expect(err).To(MatchError(ContainSubstring(expErrSub)))
		Expect(r).To(BeNil())

		By("referring an invalid OCI")
		invalidOCI := fmt.Sprintf("%s://registry.com /invalid/release-manifest:0.0.1", source.OCI)
		expErrSub = "unable to convert uri 'oci://registry.com /invalid/release-manifest:0.0.1' to manifest source"
		r, err = res.Resolve(invalidOCI)
		Expect(err).ToNot(BeNil())
		Expect(err).To(MatchError(ContainSubstring(expErrSub)))
		Expect(r).To(BeNil())
	})

	It("fails when the release manifest is missing", func() {
		missingFile := fmt.Sprintf("%s://missing/file.yaml", source.File)
		expErrSub := "reading manifest from source 'missing/file.yaml'"
		r, err := res.Resolve(missingFile)
		Expect(err).ToNot(BeNil())
		Expect(err).To(MatchError(ContainSubstring(expErrSub)))
		Expect(r).To(BeNil())
	})

	It("fails when the release manifest is empty", func() {
		reader.returnEmpty = true
		dummyFileURI := fmt.Sprintf("%s://%s", source.File, "dummy.txt")
		expErrSub := "empty file passed as release manifest: 'dummy.txt'"
		r, err := res.Resolve(dummyFileURI)
		Expect(err).ToNot(BeNil())
		Expect(err).To(MatchError(ContainSubstring(expErrSub)))
		Expect(r).To(BeNil())
	})

	It("fails when the content is not actually a release manifest", func() {
		reader.returnNonReleaseManifest = true
		wrongFileURI := fmt.Sprintf("%s://%s", source.File, "wrong.txt")
		expErrSub := "unable to parse 'wrong.txt' as a valid release manifest"
		r, err := res.Resolve(wrongFileURI)
		Expect(err).ToNot(BeNil())
		Expect(err).To(MatchError(ContainSubstring(expErrSub)))
		Expect(r).To(BeNil())
	})

	It("fails when the release manifest cannot be extracted from the OCI image", func() {
		reader.failOCIExtract = true
		missingOCI := fmt.Sprintf("%s://%s", source.OCI, "registry.com/missing/release-manifest:0.0.1")
		expErr := "reading manifest from source 'registry.com/missing/release-manifest:0.0.1': oci extract error"
		r, err := res.Resolve(missingOCI)
		Expect(err).ToNot(BeNil())
		Expect(err).To(MatchError(expErr))
		Expect(r).To(BeNil())
	})
})

func validateResolvedManifest(rm *resolver.ResolvedManifest, coreOnly bool) {
	Expect(rm.CorePlatform).ToNot(BeNil())

	Expect(rm.CorePlatform.MetaData).ToNot(BeNil())
	Expect(rm.CorePlatform.MetaData.Name).To(Equal("suse-core"))
	Expect(rm.CorePlatform.MetaData.Version).To(Equal("1.0"))
	Expect(len(rm.CorePlatform.MetaData.UpgradePathsFrom)).To(Equal(1))
	Expect(rm.CorePlatform.MetaData.UpgradePathsFrom[0]).To(Equal("0.0.1"))
	Expect(rm.CorePlatform.MetaData.CreationDate).To(Equal("2000-01-01"))

	Expect(rm.CorePlatform.Components).ToNot(BeNil())
	Expect(rm.CorePlatform.Components.OperatingSystem).ToNot(BeNil())
	Expect(rm.CorePlatform.Components.OperatingSystem.Version).To(Equal("6.2"))
	Expect(rm.CorePlatform.Components.OperatingSystem.Image).To(Equal("registry.com/foo/bar/sl-micro:6.2"))

	Expect(rm.CorePlatform.Components.Kubernetes).ToNot(BeNil())
	Expect(rm.CorePlatform.Components.Kubernetes.RKE2).ToNot(BeNil())
	Expect(rm.CorePlatform.Components.Kubernetes.RKE2.Version).To(Equal("1.32"))
	Expect(rm.CorePlatform.Components.Kubernetes.RKE2.Image).To(Equal("registry.com/foo/bar/rke2:1.32"))

	Expect(rm.CorePlatform.Components.Helm).ToNot(BeNil())
	Expect(len(rm.CorePlatform.Components.Helm.Charts)).To(Equal(1))
	Expect(rm.CorePlatform.Components.Helm.Charts[0].Name).To(Equal("Foo"))
	Expect(rm.CorePlatform.Components.Helm.Charts[0].Chart).To(Equal("foo"))
	Expect(rm.CorePlatform.Components.Helm.Charts[0].Version).To(Equal("0.0.0"))
	Expect(rm.CorePlatform.Components.Helm.Charts[0].Namespace).To(Equal("foo-system"))
	Expect(rm.CorePlatform.Components.Helm.Charts[0].Values).To(Equal("image:\n  tag: latest"))
	Expect(len(rm.CorePlatform.Components.Helm.Charts[0].DependsOn)).To(Equal(1))
	Expect(rm.CorePlatform.Components.Helm.Charts[0].DependsOn[0]).To(Equal("baz"))
	Expect(len(rm.CorePlatform.Components.Helm.Charts[0].Images)).To(Equal(1))
	Expect(rm.CorePlatform.Components.Helm.Charts[0].Images[0].Name).To(Equal("foo"))
	Expect(rm.CorePlatform.Components.Helm.Charts[0].Images[0].Image).To(Equal("registry.com/foo/foo:0.0.0"))
	Expect(len(rm.CorePlatform.Components.Helm.Repositories)).To(Equal(1))
	Expect(rm.CorePlatform.Components.Helm.Repositories[0].Name).To(Equal("foo-charts"))
	Expect(rm.CorePlatform.Components.Helm.Repositories[0].URL).To(Equal("https://foo.github.io/charts"))

	if !coreOnly {
		Expect(rm.ProductExtension).ToNot(BeNil())

		Expect(rm.ProductExtension.MetaData).ToNot(BeNil())
		Expect(rm.ProductExtension.MetaData.Name).To(Equal("suse-edge"))
		Expect(rm.ProductExtension.MetaData.Version).To(Equal("3.2.0"))
		Expect(len(rm.ProductExtension.MetaData.UpgradePathsFrom)).To(Equal(1))
		Expect(rm.ProductExtension.MetaData.UpgradePathsFrom[0]).To(Equal("3.1.2"))
		Expect(rm.ProductExtension.MetaData.CreationDate).To(Equal("2025-01-20"))

		Expect(rm.ProductExtension.CorePlatform).ToNot(BeNil())
		Expect(rm.ProductExtension.CorePlatform.Image).To(Equal("foo.example.com/bar/release-manifest"))
		Expect(rm.ProductExtension.CorePlatform.Version).To(Equal("1.0"))

		Expect(rm.ProductExtension.Components.Helm).ToNot(BeNil())
		Expect(len(rm.ProductExtension.Components.Helm.Charts)).To(Equal(1))
		Expect(rm.ProductExtension.Components.Helm.Charts[0].Name).To(Equal("Bar"))
		Expect(rm.ProductExtension.Components.Helm.Charts[0].Chart).To(Equal("bar"))
		Expect(rm.ProductExtension.Components.Helm.Charts[0].Version).To(Equal("0.0.0"))
		Expect(rm.ProductExtension.Components.Helm.Charts[0].Namespace).To(Equal("bar-system"))
		Expect(rm.ProductExtension.Components.Helm.Charts[0].Values).To(Equal("image:\n  tag: latest"))
		Expect(len(rm.ProductExtension.Components.Helm.Charts[0].DependsOn)).To(Equal(1))
		Expect(rm.ProductExtension.Components.Helm.Charts[0].DependsOn[0]).To(Equal("foo"))
		Expect(len(rm.ProductExtension.Components.Helm.Charts[0].Images)).To(Equal(1))
		Expect(rm.ProductExtension.Components.Helm.Charts[0].Images[0].Name).To(Equal("bar"))
		Expect(rm.ProductExtension.Components.Helm.Charts[0].Images[0].Image).To(Equal("registry.com/bar/bar:0.0.0"))
		Expect(len(rm.ProductExtension.Components.Helm.Repositories)).To(Equal(1))
		Expect(rm.ProductExtension.Components.Helm.Repositories[0].Name).To(Equal("bar-charts"))
		Expect(rm.ProductExtension.Components.Helm.Repositories[0].URL).To(Equal("https://bar.github.io/charts"))
	} else {
		Expect(rm.ProductExtension).To(BeNil())
	}
}

type SourceReaderMock struct {
	failOCIExtract           bool
	returnEmpty              bool
	returnNonReleaseManifest bool
}

func (s SourceReaderMock) Read(m *source.ReleaseManifestSource) ([]byte, error) {
	if s.returnEmpty {
		return []byte{}, nil
	}

	if s.returnNonReleaseManifest {
		return []byte("this is not a release manifest"), nil
	}

	if m.Type() == source.OCI {
		if s.failOCIExtract {
			return nil, fmt.Errorf("oci extract error")
		}

		switch {
		case m.URI() == expectedCorePlatformImage:
			return os.ReadFile(coreManifestPath)
		case m.URI() == expectedProductManifestImage:
			return os.ReadFile(prodManifestPath)
		default:
			return nil, fmt.Errorf("unexpected image uri '%s'", m.URI())
		}
	}
	return os.ReadFile(m.URI())
}
