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

package source_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/manifest/source"
)

var _ = Describe("ReleaseManifestSourceType", Label("release-manifest"), func() {
	It("is parsed correctly as 'File' source type", func() {
		srcType, err := source.ParseType("file")
		Expect(err).ToNot(HaveOccurred())
		Expect(srcType).To(Equal(source.File))
	})

	It("is parsed correctly as 'OCI' source type", func() {
		srcType, err := source.ParseType("oci")
		Expect(err).ToNot(HaveOccurred())
		Expect(srcType).To(Equal(source.OCI))
	})

	It("fails for an unexpected source type", func() {
		expErrMsg := "manifest source type 'unknown' is not supported. Supported source types: 'file', 'oci'"
		_, err := source.ParseType("unknown")
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErrMsg))
	})
})

var _ = Describe("ReleaseManifestSource", Label("release-manifest"), func() {
	It("is initialised correctly from a 'file' type source", func() {
		fileURI := "file:///foo/../bar///release_manifest.yaml"
		normalisedURI := "/bar/release_manifest.yaml"

		rmSource, err := source.ParseFromURI(fileURI)
		Expect(err).ToNot(HaveOccurred())
		Expect(rmSource).ToNot(BeNil())
		Expect(rmSource.URI()).To(Equal(normalisedURI))
		Expect(rmSource.Type()).To(Equal(source.File))
	})

	It("is initialised correctly from an 'oci' type source", func() {
		src := "foo.example.com/bar/release-manifest:0.0.1"
		ociURI := fmt.Sprintf("%s://%s", source.OCI, src)

		rmSource, err := source.ParseFromURI(ociURI)
		Expect(err).ToNot(HaveOccurred())
		Expect(rmSource).ToNot(BeNil())
		Expect(rmSource.URI()).To(Equal(src))
		Expect(rmSource.Type()).To(Equal(source.OCI))
	})

	It("initialization fails", func() {
		By("throwing a parse error")
		brokenURI := "file:// /foo/bar/release_manifest.yaml"
		expErr := "parsing manifest source uri: parse \"file:// /foo/bar/release_manifest.yaml\": invalid character \" \" in host name"
		validateInitialisationErr(brokenURI, expErr)

		By("throwing a 'missing scheme' error")
		missingSchemeURI := "/foo/bar/release_manifest.yaml"
		expErr = "missing scheme in source uri: '/foo/bar/release_manifest.yaml'"
		validateInitialisationErr(missingSchemeURI, expErr)

		By("throwing an 'unknown source' error")
		src := "unknown"
		unknownSrc := fmt.Sprintf("%s:///foo/bar/release_manifest.yaml", src)
		expErr = fmt.Sprintf("parsing manifest source type: manifest source type '%s' is not supported. Supported source types: 'file', 'oci'", src)
		validateInitialisationErr(unknownSrc, expErr)

		By("throwing an 'invalid OCI image' error")
		invalidOCI := "oci://foo.example.com/bar:00|11"
		expErr = "invalid OCI image reference: could not parse reference: foo.example.com/bar:00|11"
		validateInitialisationErr(invalidOCI, expErr)
	})
})

func validateInitialisationErr(uri, errMsg string) {
	rmSource, err := source.ParseFromURI(uri)
	Expect(err).ToNot(BeNil())
	Expect(err).To(MatchError(errMsg))
	Expect(rmSource).To(BeNil())
}
