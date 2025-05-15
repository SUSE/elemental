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

package source_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/manifest/source"
)

const (
	testFile     = "test-file.yaml"
	dummyContent = "dummy"
)

var _ = Describe("ReleaseManifestReader", Ordered, Label("release-manifest"), func() {
	var testDir string
	var testFilePath string
	var fileExtr *OCIFileExtractorMock
	var reader *source.ReleaseManifestReader
	BeforeAll(func() {
		var err error
		testDir, err = os.MkdirTemp("", "elemental-manifest-source-*")
		Expect(err).ToNot(HaveOccurred())

		testFilePath = filepath.Join(testDir, testFile)
		Expect(os.WriteFile(testFilePath, []byte(dummyContent), 0644)).To(Succeed())

		fileExtr = &OCIFileExtractorMock{manifestPath: testFilePath}
		reader = source.NewReader(fileExtr)
	})

	AfterAll(func() {
		Expect(os.RemoveAll(testDir)).To(Succeed())
	})

	It("reads from a local manifest source", func() {
		data, err := reader.Read(getSource(source.File, testFilePath))
		Expect(err).ToNot(HaveOccurred())
		Expect(len(data)).ToNot(Equal(0))
		Expect(data).To(Equal([]byte(dummyContent)))
	})
	It("fails to read from a local manifest source", func() {
		data, err := reader.Read(getSource(source.File, testFilePath+"missing"))
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("no such file or directory")))
		Expect(len(data)).To(Equal(0))
	})
	It("reads from an oci manifest source", func() {
		data, err := reader.Read(getSource(source.OCI, "registry.com/foo/bar/test:0.0.1"))
		Expect(err).ToNot(HaveOccurred())
		Expect(len(data)).ToNot(Equal(0))
		Expect(data).To(Equal([]byte(dummyContent)))
	})
	It("fails to read from an oci manifest source", func() {
		failingExtr := &OCIFileExtractorMock{fail: true}
		failingReader := source.NewReader(failingExtr)
		data, err := failingReader.Read(getSource(source.OCI, "registry.com/foo/bar/test:0.0.1"))
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("extracting file from OCI image 'registry.com/foo/bar/test:0.0.1': failed extract"))
		Expect(len(data)).To(Equal(0))
	})
})

func getSource(srcType source.ReleaseManifestSourceType, location string) *source.ReleaseManifestSource {
	sourceURI := fmt.Sprintf("%s://%s", srcType, location)
	rmSrc, err := source.ParseFromURI(sourceURI)
	Expect(err).ToNot(HaveOccurred())
	Expect(rmSrc).ToNot(BeNil())
	return rmSrc
}

type OCIFileExtractorMock struct {
	manifestPath string
	fail         bool
}

func (o OCIFileExtractorMock) ExtractFrom(uri string) (path string, err error) {
	if o.fail {
		return "", fmt.Errorf("failed extract")
	}
	return o.manifestPath, nil
}
