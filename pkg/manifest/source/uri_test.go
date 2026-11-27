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
	"bytes"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/source"
)

var _ = Describe("ResolveURI", Label("release-manifest"), func() {
	discard := log.New(log.WithDiscardAll())

	DescribeTable("resolves references to canonical file URIs",
		func(ref, base, expected string) {
			uri, err := source.ResolveURI(ref, base, discard)
			Expect(err).ToNot(HaveOccurred())
			Expect(uri).To(Equal(expected))
		},
		Entry("bare relative path", "bar.yaml", "/base", "file:///base/bar.yaml"),
		Entry("dot relative path", "./sub/bar.yaml", "/base", "file:///base/sub/bar.yaml"),
		Entry("parent directory", "../shared/bar.yaml", "/base/conf", "file:///base/shared/bar.yaml"),
		Entry("escaped characters", "my%20file.yaml", "/base", "file:///base/my%20file.yaml"),
		Entry("root base", "bar.yaml", "/", "file:///bar.yaml"),
		Entry("absolute-path reference", "/abs/../bar.yaml", "/base", "file:///bar.yaml"),
		Entry("absolute file URI", "file:///abs/bar.yaml", "/base", "file:///abs/bar.yaml"),
		Entry("localhost file URI", "file://localhost/abs/bar.yaml", "/base", "file:///abs/bar.yaml"),
	)

	It("resolves against the working directory for an empty base", func() {
		wd, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())

		uri, err := source.ResolveURI("bar.yaml", "", discard)
		Expect(err).ToNot(HaveOccurred())
		Expect(uri).To(Equal("file://" + filepath.Join(wd, "bar.yaml")))
	})

	It("leaves other schemes unchanged", func() {
		uri, err := source.ResolveURI("oci://registry.example.com/manifest:1.0", "/base", discard)
		Expect(err).ToNot(HaveOccurred())
		Expect(uri).To(Equal("oci://registry.example.com/manifest:1.0"))
	})

	// legacy-file-dot-uri
	It("resolves the deprecated file://./ form with a warning", func() {
		buf := &bytes.Buffer{}
		uri, err := source.ResolveURI("file://./sub/bar.yaml", "/base", log.New(log.WithBuffer(buf)))
		Expect(err).ToNot(HaveOccurred())
		Expect(uri).To(Equal("file:///base/sub/bar.yaml"))
		Expect(buf.String()).To(ContainSubstring("deprecated"))
	})

	It("does not warn for relative references", func() {
		buf := &bytes.Buffer{}
		_, err := source.ResolveURI("./sub/bar.yaml", "/base", log.New(log.WithBuffer(buf)))
		Expect(err).ToNot(HaveOccurred())
		Expect(buf.String()).ToNot(ContainSubstring("deprecated"))
	})

	DescribeTable("rejects URIs that are not valid manifest sources",
		func(ref, errSubstring string) {
			_, err := source.ResolveURI(ref, "/base", discard)
			Expect(err).To(MatchError(ContainSubstring(errSubstring)))
		},
		Entry("unsupported scheme", "https://example.com/manifest.yaml", "manifest source type 'https' is not supported"),
		Entry("invalid OCI reference", "oci://registry.example.com/bar:00|11", "invalid OCI image reference"),
	)

	DescribeTable("rejects file URIs that are not local paths",
		func(ref string) {
			_, err := source.ResolveURI(ref, "/base", discard)
			Expect(err).To(MatchError(ContainSubstring("unsupported file uri")))
		},
		Entry("file name as host", "file://bar.yaml"),
		Entry("opaque path", "file:bar.yaml"),
		Entry("network-path reference", "//host/bar.yaml"),
	)
})
