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

	"github.com/suse/elemental/v3/pkg/extractor"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	dummyContent = "dummy"
	dummyOCI     = "registry.com/dummy/file-img:0.0.1"
	fileName     = "file.yaml"
)

func TestOCIFileExtractor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OCI File Extractor test suite")
}

var _ = Describe("OCIFileExtractor", Label("file-extractor"), func() {
	var unpacker *unpackerMock
	var cleanup func()
	var tfs vfs.FS
	var extrOpts []extractor.OCIFileExtractorOpts
	var defaultSearchPaths []string
	BeforeEach(func() {
		var err error
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())

		unpacker = &unpackerMock{
			fileAtPath: fileName,
			digest:     "sha256:" + randomDigestEnc(64),
			tfs:        tfs,
		}

		extrOpts = []extractor.OCIFileExtractorOpts{
			extractor.WithOCIUnpacker(unpacker),
			extractor.WithFS(tfs),
		}

		defaultSearchPaths = []string{"file*.yaml"}
	})

	AfterEach(func() {
		cleanup()
	})

	It("extracts file to default store", func() {
		digestEnc := randomDigestEnc(64)
		unpacker.digest = "sha256:" + digestEnc
		storePathPrefix := filepath.Join(os.TempDir(), "extracted-files-")
		expectedStorePath := filepath.Join(storePathPrefix, digestEnc)

		defaultStoreExtr, err := extractor.New(defaultSearchPaths, extrOpts...)
		Expect(err).ToNot(HaveOccurred())

		extractedFile, err := defaultStoreExtr.ExtractFrom(dummyOCI, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(filepath.Dir(extractedFile)).To(Equal(expectedStorePath))
		validateExtractedFileContent(tfs, extractedFile)
	})

	It("extracts file to custom store", func() {
		digestEnc := randomDigestEnc(128)
		fileStoreName := digestEnc[:64]
		unpacker.digest = "sha512:" + digestEnc

		customStoreRoot, err := vfs.TempDir(tfs, "", "extractor-custom-store-")
		Expect(err).ToNot(HaveOccurred())

		expectedFileStore := filepath.Join(customStoreRoot, fileStoreName)

		extrOpts = append(extrOpts, extractor.WithStore(customStoreRoot))
		customStoreExtr, err := extractor.New(defaultSearchPaths, extrOpts...)
		Expect(err).ToNot(HaveOccurred())

		extractedFile, err := customStoreExtr.ExtractFrom(dummyOCI, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.HasPrefix(extractedFile, customStoreRoot)).To(BeTrue())
		Expect(filepath.Dir(extractedFile)).To(Equal(expectedFileStore))
		validateExtractedFileContent(tfs, extractedFile)
	})

	It("extracts first found file", func() {
		digestEnc := randomDigestEnc(64)
		unpacker.digest = "sha512:" + digestEnc
		unpacker.multipleFiles = true
		unpacker.fileAtPath = "file3.yaml"

		storePathPrefix := filepath.Join(os.TempDir(), "extracted-files-")
		expectedExtractedFile := filepath.Join(storePathPrefix, digestEnc, "file2.yaml")

		customStoreExtr, err := extractor.New(defaultSearchPaths, extrOpts...)
		Expect(err).ToNot(HaveOccurred())

		extractedFile, err := customStoreExtr.ExtractFrom(dummyOCI, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(extractedFile).To(Equal(expectedExtractedFile))
		validateExtractedFileContent(tfs, extractedFile)
	})

	It("fails when unpacking an OCI image", func() {
		unpacker.fail = true
		expErr := "unpacking oci image: unpack failure"

		defaultExtr, err := extractor.New(defaultSearchPaths, extrOpts...)
		Expect(err).ToNot(HaveOccurred())

		file, err := defaultExtr.ExtractFrom(dummyOCI, false)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErr))
		Expect(file).To(BeEmpty())
	})

	It("fails when file is missing in the unpacked image", func() {
		customSearchPath := filepath.Join("dummy", "file*.yaml")
		expErr := "locating file at unpacked OCI filesystem: failed to find file matching [dummy/file*.yaml] in /tmp/unpacked-oci-"

		extr, err := extractor.New([]string{customSearchPath}, extrOpts...)
		Expect(err).ToNot(HaveOccurred())

		file, err := extr.ExtractFrom(dummyOCI, false)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErr))
		Expect(file).To(BeEmpty())
	})

	It("fails when produced digest is not in an OCI format", func() {
		unpacker.digest = "d41d8cd98f00b204e9800998ecf8427e"
		expErr := fmt.Sprintf("generating file store based on digest: invalid digest format '%s', expected '<algorithm>:<hash>'", unpacker.digest)

		extr, err := extractor.New(defaultSearchPaths, extrOpts...)
		Expect(err).ToNot(HaveOccurred())

		file, err := extr.ExtractFrom(dummyOCI, false)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(expErr))
		Expect(file).To(BeEmpty())
	})

	It("fails when custom store does not exist on filesystem", func() {
		missing := "/missing"
		errSubstring := "store path '/missing' does not exist in provided filesystem"

		extrOpts = append(extrOpts, extractor.WithStore(missing))
		_, err := extractor.New(defaultSearchPaths, extrOpts...)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(errSubstring))
	})
})

func validateExtractedFileContent(fs vfs.FS, file string) {
	data, err := fs.ReadFile(file)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(data)).To(Equal(dummyContent))
}

type unpackerMock struct {
	fileAtPath    string
	fail          bool
	digest        string
	multipleFiles bool
	tfs           vfs.FS
}

func (u unpackerMock) Unpack(ctx context.Context, uri, dest string, local bool) (digest string, err error) {
	if u.fail {
		return "", fmt.Errorf("unpack failure")
	}

	dir := filepath.Dir(filepath.Join(dest, u.fileAtPath))
	if err := vfs.MkdirAll(u.tfs, dir, 0755); err != nil {
		return "", err
	}

	if err := u.tfs.WriteFile(filepath.Join(dest, u.fileAtPath), []byte(dummyContent), 0644); err != nil {
		return "", err
	}

	if u.multipleFiles {
		secondFile := filepath.Join(filepath.Dir(u.fileAtPath), "file2.yaml")
		if err := u.tfs.WriteFile(filepath.Join(dest, secondFile), []byte(dummyContent), 0644); err != nil {
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
