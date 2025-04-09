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

package deployment_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	"github.com/suse/elemental/v3/pkg/deployment"
)

const src = `
uri: raw:///path/to/image/file.raw
digest: adfasdfadsfaf
`

var _ = Describe("Image Source", Label("imagesource"), func() {
	It("initiates an OCI image source from URI", func() {
		imgsrc, err := deployment.NewSrcFromURI("registry.org/my/image")
		Expect(err).NotTo(HaveOccurred())
		Expect(imgsrc.String()).To(Equal("oci://registry.org/my/image:latest"))
		Expect(imgsrc.IsOCI()).To(BeTrue())
		Expect(imgsrc.URI()).To(Equal("registry.org/my/image:latest"))
		Expect(imgsrc.IsEmpty()).To(BeFalse())
	})
	It("initiates a Raw image source from URI", func() {
		imgsrc, err := deployment.NewSrcFromURI("raw:///some/path/to/image")
		Expect(err).NotTo(HaveOccurred())
		Expect(imgsrc.String()).To(Equal("raw:///some/path/to/image"))
		Expect(imgsrc.IsOCI()).To(BeFalse())
		Expect(imgsrc.IsRaw()).To(BeTrue())
		Expect(imgsrc.URI()).To(Equal("/some/path/to/image"))
	})
	It("initiates a Dir image source from URI", func() {
		imgsrc, err := deployment.NewSrcFromURI("dir://some/path/to/directory")
		Expect(err).NotTo(HaveOccurred())
		Expect(imgsrc.String()).To(Equal("dir://some/path/to/directory"))
		Expect(imgsrc.IsDir()).To(BeTrue())
		Expect(imgsrc.IsRaw()).To(BeFalse())
		Expect(imgsrc.URI()).To(Equal("some/path/to/directory"))
	})
	It("fails with unknown schema in URI", func() {
		imgsrc, err := deployment.NewSrcFromURI("https://example.com/my/image")
		Expect(err).To(HaveOccurred())
		Expect(imgsrc.IsEmpty()).To(BeTrue())
	})
	It("initiates an empty image source", func() {
		imgsrc := deployment.NewEmptySrc()
		Expect(imgsrc.IsEmpty()).To(BeTrue())
	})
	It("initiates an OCI image source", func() {
		imgsrc := deployment.NewOCISrc("my/ref:mytag")
		Expect(imgsrc.IsDir()).To(BeFalse())
		Expect(imgsrc.IsOCI()).To(BeTrue())
		Expect(imgsrc.GetDigest()).To(BeEmpty())
		imgsrc.SetDigest("somedigest")
		Expect(imgsrc.GetDigest()).To(Equal("somedigest"))
	})
	It("initiates a Raw image source", func() {
		imgsrc := deployment.NewRawSrc("my/image.raw")
		Expect(imgsrc.IsRaw()).To(BeTrue())
	})
	It("initiates a Dir image source", func() {
		imgsrc := deployment.NewDirSrc("/some/dir")
		Expect(imgsrc.IsDir()).To(BeTrue())
	})
	It("serializes an image source", func() {
		imgsrc, err := deployment.NewSrcFromURI("oci://registry.org/my/image")
		Expect(err).NotTo(HaveOccurred())
		imgsrc.SetDigest("somedigest")
		data, err := yaml.Marshal(imgsrc)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal(`digest: somedigest
uri: oci://registry.org/my/image:latest
`))
	})
	It("deserializes an image source", func() {
		imgsrc := deployment.NewEmptySrc()
		Expect(yaml.Unmarshal([]byte(src), imgsrc)).To(Succeed())
		Expect(imgsrc.IsEmpty()).To(BeFalse())
		Expect(imgsrc.IsRaw()).To(BeTrue())
		Expect(imgsrc.String()).To(Equal("raw:///path/to/image/file.raw"))
		Expect(imgsrc.GetDigest()).To(Equal("adfasdfadsfaf"))
	})
	It("fails to deserialize an image without uri", func() {
		imgsrc := deployment.NewEmptySrc()
		Expect(yaml.Unmarshal([]byte("digest: adfadsfa"), imgsrc)).NotTo(Succeed())
	})
	It("fails to deserialize invalid URI type", func() {
		imgsrc := deployment.NewEmptySrc()
		Expect(yaml.Unmarshal([]byte("uri: https://example/com"), imgsrc)).NotTo(Succeed())
	})
})
