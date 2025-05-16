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

package extractor_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/internal/manifest/extractor"
)

const (
	unpackDirPrefix     = "release-manifest-unpack-"
	dummyContent        = "dummy"
	dummyOCI            = "registry.com/dummy/release-manifest:0.0.1"
	releaseManifestName = "release_manifest.yaml"
)

func TestOCIReleaseManifestExtractor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Release Manifest OCI Extractor test suite")
}

var _ = Describe("OCIReleaseManifestExtractor", Label("release-manifest"), func() {
	var customStoreRoot string
	var unpacker *unpackerMock
	BeforeEach(func() {
		var err error
		customStoreRoot, err = os.MkdirTemp("", "extractor-custom-store-")
		Expect(err).ToNot(HaveOccurred())

		unpacker = &unpackerMock{
			manifestAtPath: releaseManifestName,
			digest:         "sha256:" + randomDigestEnc(64),
		}
	})

	AfterEach(func() {
		Expect(os.RemoveAll(customStoreRoot)).To(Succeed())
	})

	It("extracts release manifest to default store", func() {
		digestEnc := randomDigestEnc(64)
		unpacker.digest = "sha256:" + digestEnc
		expectedDefaultStoreRoot := filepath.Join(os.TempDir(), "release-manifests")
		expectedManifestStore := filepath.Join(expectedDefaultStoreRoot, digestEnc)

		defaultStoreExtr, err := extractor.New(extractor.WithOCIUnpacker(unpacker))
		Expect(err).ToNot(HaveOccurred())

		extractedManifest, err := defaultStoreExtr.ExtractFrom(dummyOCI)
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.HasPrefix(extractedManifest, expectedDefaultStoreRoot)).To(BeTrue())
		Expect(filepath.Dir(extractedManifest)).To(Equal(expectedManifestStore))
		validateExtractedManifestContent(extractedManifest)
		Expect(os.RemoveAll(expectedDefaultStoreRoot)).To(Succeed())
	})

	It("extracts release manifest to custom store", func() {
		digestEnc := randomDigestEnc(128)
		manifestStoreName := digestEnc[:64]
		unpacker.digest = "sha512:" + digestEnc
		expectedManifestStore := filepath.Join(customStoreRoot, manifestStoreName)

		customStoreExtr, err := extractor.New(extractor.WithStore(customStoreRoot), extractor.WithOCIUnpacker(unpacker))
		Expect(err).ToNot(HaveOccurred())

		extractedManifest, err := customStoreExtr.ExtractFrom(dummyOCI)
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.HasPrefix(extractedManifest, customStoreRoot)).To(BeTrue())
		Expect(filepath.Dir(extractedManifest)).To(Equal(expectedManifestStore))
		validateExtractedManifestContent(extractedManifest)
	})

	It("extracts release manifest using custom search paths", func() {
		customManifestName := "release_manifest_foo.yaml"
		searchPaths := []string{filepath.Join("dummy", "release_manifest*.yaml")}
		unpacker.manifestAtPath = filepath.Join("dummy", customManifestName)

		customSearchPathExtr, err := extractor.New(extractor.WithSearchPaths(searchPaths), extractor.WithStore(customStoreRoot), extractor.WithOCIUnpacker(unpacker))
		Expect(err).ToNot(HaveOccurred())

		extractedManifest, err := customSearchPathExtr.ExtractFrom(dummyOCI)
		Expect(err).ToNot(HaveOccurred())
		Expect(filepath.Base(extractedManifest)).To(Equal(customManifestName))
		validateExtractedManifestContent(extractedManifest)
	})

	It("fails when unpacking an OCI image", func() {
		unpacker.fail = true
		expErr := "unpacking oci image: unpack failure"

		defaultExtr, err := extractor.New(extractor.WithOCIUnpacker(unpacker), extractor.WithStore(customStoreRoot))
		Expect(err).ToNot(HaveOccurred())

		manifest, err := defaultExtr.ExtractFrom(dummyOCI)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErr))
		Expect(manifest).To(BeEmpty())
	})

	It("fails when release manifest is missing in the unpacked image", func() {
		customSearchPath := filepath.Join("dummy", "release_manifest*.yaml")
		expErr := "locating release manifest at unpacked OCI filesystem: release manifest not found at paths: [dummy/release_manifest*.yaml]"

		extr, err := extractor.New(extractor.WithSearchPaths([]string{customSearchPath}), extractor.WithOCIUnpacker(unpacker), extractor.WithStore(customStoreRoot))
		Expect(err).ToNot(HaveOccurred())

		manifest, err := extr.ExtractFrom(dummyOCI)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErr))
		Expect(manifest).To(BeEmpty())
	})

	It("fails when there is more than one release manfiest at the search path", func() {
		unpacker.multipleManifest = true
		unpacker.manifestAtPath = filepath.Join("etc", "release-manifest", releaseManifestName)
		expErr := "locating release manifest at unpacked OCI filesystem: expected a single release manifest at 'etc/release-manifest', got '2'"

		extr, err := extractor.New(extractor.WithOCIUnpacker(unpacker), extractor.WithStore(customStoreRoot))
		Expect(err).ToNot(HaveOccurred())

		manifest, err := extr.ExtractFrom(dummyOCI)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErr))
		Expect(manifest).To(BeEmpty())
	})

	It("fails when produced digest is not in an OCI format", func() {
		unpacker.digest = "d41d8cd98f00b204e9800998ecf8427e"
		expErr := fmt.Sprintf("generating manifest store based on digest: invalid digest format '%s', expected '<algorithm>:<hash>'", unpacker.digest)

		extr, err := extractor.New(extractor.WithOCIUnpacker(unpacker), extractor.WithStore(customStoreRoot))
		Expect(err).ToNot(HaveOccurred())

		manifest, err := extr.ExtractFrom(dummyOCI)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErr))
		Expect(manifest).To(BeEmpty())
	})
})

func validateExtractedManifestContent(manifest string) {
	data, err := os.ReadFile(manifest)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(data)).To(Equal(dummyContent))
}

type unpackerMock struct {
	manifestAtPath   string
	fail             bool
	digest           string
	multipleManifest bool
}

func (u unpackerMock) Unpack(ctx context.Context, uri, dest string) (digest string, err error) {
	if u.fail {
		return "", fmt.Errorf("unpack failure")
	}

	dir := filepath.Dir(filepath.Join(dest, u.manifestAtPath))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	if err := os.WriteFile(filepath.Join(dest, u.manifestAtPath), []byte(dummyContent), 0644); err != nil {
		return "", err
	}

	if u.multipleManifest {
		secondManifest := filepath.Join(filepath.Dir(u.manifestAtPath), "release_manifest2.yaml")
		if err := os.WriteFile(filepath.Join(dest, secondManifest), []byte(dummyContent), 0644); err != nil {
			return "", err
		}
	}

	return u.digest, nil
}

func randomDigestEnc(n int) string {
	const letters = "0123456789abcdef"

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}
