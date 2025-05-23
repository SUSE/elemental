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
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
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
	var unpacker *unpackerMock
	var cleanup func()
	var tfs vfs.FS
	var extrOpts []extractor.Opts
	BeforeEach(func() {
		var err error
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())

		unpacker = &unpackerMock{
			manifestAtPath: releaseManifestName,
			digest:         "sha256:" + randomDigestEnc(64),
			tfs:            tfs,
		}

		extrOpts = []extractor.Opts{
			extractor.WithOCIUnpacker(unpacker),
			extractor.WithFS(tfs),
		}
	})

	AfterEach(func() {
		cleanup()
	})

	It("extracts release manifest to default store", func() {
		digestEnc := randomDigestEnc(64)
		unpacker.digest = "sha256:" + digestEnc
		storePathPrefix := filepath.Join(os.TempDir(), "release-manifests-")
		expectedStorePath := filepath.Join(storePathPrefix, digestEnc)

		defaultStoreExtr, err := extractor.New(extrOpts...)
		Expect(err).ToNot(HaveOccurred())

		extractedManifest, err := defaultStoreExtr.ExtractFrom(dummyOCI)
		Expect(err).ToNot(HaveOccurred())
		Expect(filepath.Dir(extractedManifest)).To(Equal(expectedStorePath))
		validateExtractedManifestContent(tfs, extractedManifest)
	})

	It("extracts release manifest to custom store", func() {
		digestEnc := randomDigestEnc(128)
		manifestStoreName := digestEnc[:64]
		unpacker.digest = "sha512:" + digestEnc

		customStoreRoot, err := vfs.TempDir(tfs, "", "extractor-custom-store-")
		Expect(err).ToNot(HaveOccurred())

		expectedManifestStore := filepath.Join(customStoreRoot, manifestStoreName)

		extrOpts = append(extrOpts, extractor.WithStore(customStoreRoot))
		customStoreExtr, err := extractor.New(extrOpts...)
		Expect(err).ToNot(HaveOccurred())

		extractedManifest, err := customStoreExtr.ExtractFrom(dummyOCI)
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.HasPrefix(extractedManifest, customStoreRoot)).To(BeTrue())
		Expect(filepath.Dir(extractedManifest)).To(Equal(expectedManifestStore))
		validateExtractedManifestContent(tfs, extractedManifest)
	})

	It("extracts release manifest using custom search paths", func() {
		customManifestName := "release_manifest_foo.yaml"
		searchPaths := []string{filepath.Join("dummy", "release_manifest*.yaml")}
		unpacker.manifestAtPath = filepath.Join("dummy", customManifestName)

		extrOpts = append(extrOpts, extractor.WithSearchPaths(searchPaths))
		customSearchPathExtr, err := extractor.New(extrOpts...)
		Expect(err).ToNot(HaveOccurred())

		extractedManifest, err := customSearchPathExtr.ExtractFrom(dummyOCI)
		Expect(err).ToNot(HaveOccurred())
		Expect(filepath.Base(extractedManifest)).To(Equal(customManifestName))
		validateExtractedManifestContent(tfs, extractedManifest)
	})

	It("fails when unpacking an OCI image", func() {
		unpacker.fail = true
		expErr := "unpacking oci image: unpack failure"

		defaultExtr, err := extractor.New(extrOpts...)
		Expect(err).ToNot(HaveOccurred())

		manifest, err := defaultExtr.ExtractFrom(dummyOCI)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErr))
		Expect(manifest).To(BeEmpty())
	})

	It("fails when release manifest is missing in the unpacked image", func() {
		customSearchPath := filepath.Join("dummy", "release_manifest*.yaml")
		expErr := "locating release manifest at unpacked OCI filesystem: failed to find file matching [dummy/release_manifest*.yaml] in /tmp/release-manifest-unpack-"

		extrOpts = append(extrOpts, extractor.WithSearchPaths([]string{customSearchPath}))
		extr, err := extractor.New(extrOpts...)
		Expect(err).ToNot(HaveOccurred())

		manifest, err := extr.ExtractFrom(dummyOCI)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErr))
		Expect(manifest).To(BeEmpty())
	})

	It("fails when produced digest is not in an OCI format", func() {
		unpacker.digest = "d41d8cd98f00b204e9800998ecf8427e"
		expErr := fmt.Sprintf("generating manifest store based on digest: invalid digest format '%s', expected '<algorithm>:<hash>'", unpacker.digest)

		extr, err := extractor.New(extrOpts...)
		Expect(err).ToNot(HaveOccurred())

		manifest, err := extr.ExtractFrom(dummyOCI)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErr))
		Expect(manifest).To(BeEmpty())
	})

	It("fails when custom store does not exist on filesystem", func() {
		missing := "/missing"
		errSubstring := "store path '/missing' does not exist in provided filesystem"

		extrOpts = append(extrOpts, extractor.WithStore(missing))
		_, err := extractor.New(extrOpts...)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(errSubstring))
	})
})

func validateExtractedManifestContent(fs vfs.FS, manifest string) {
	data, err := fs.ReadFile(manifest)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(data)).To(Equal(dummyContent))
}

type unpackerMock struct {
	manifestAtPath   string
	fail             bool
	digest           string
	multipleManifest bool
	tfs              vfs.FS
}

func (u unpackerMock) Unpack(ctx context.Context, uri, dest string) (digest string, err error) {
	if u.fail {
		return "", fmt.Errorf("unpack failure")
	}

	dir := filepath.Dir(filepath.Join(dest, u.manifestAtPath))
	if err := vfs.MkdirAll(u.tfs, dir, 0755); err != nil {
		return "", err
	}

	if err := u.tfs.WriteFile(filepath.Join(dest, u.manifestAtPath), []byte(dummyContent), 0644); err != nil {
		return "", err
	}

	if u.multipleManifest {
		secondManifest := filepath.Join(filepath.Dir(u.manifestAtPath), "release_manifest2.yaml")
		if err := u.tfs.WriteFile(filepath.Join(dest, secondManifest), []byte(dummyContent), 0644); err != nil {
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
