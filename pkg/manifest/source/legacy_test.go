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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/manifest/source"
)

var _ = Describe("Legacy file://./ references", Label("release-manifest"), func() {
	DescribeTable("rewrites the deprecated form to a relative reference",
		func(ref, expected string) {
			rewritten, warn := source.RewriteLegacyRelativeURI(ref)
			Expect(warn).To(BeTrue())
			Expect(rewritten).To(Equal(expected))
		},
		Entry("file", "file://./bar.yaml", "./bar.yaml"),
		Entry("nested file", "file://./sub/bar.yaml", "./sub/bar.yaml"),
		Entry("parent directory", "file://./../bar.yaml", "./../bar.yaml"),
		Entry("escaped characters", "file://./my%20file.yaml", "./my%20file.yaml"),
		Entry("colon in first segment", "file://./foo:bar.yaml", "./foo:bar.yaml"),
		Entry("empty path", "file://./", "./"),
		Entry("query is kept for later validation", "file://./bar.yaml?x=1", "./bar.yaml?x=1"),
	)

	DescribeTable("leaves other references untouched",
		func(ref string) {
			rewritten, warn := source.RewriteLegacyRelativeURI(ref)
			Expect(warn).To(BeFalse())
			Expect(rewritten).To(Equal(ref))
		},
		Entry("relative reference", "./bar.yaml"),
		Entry("absolute file URI", "file:///bar.yaml"),
		Entry("other host", "file://example.com/bar.yaml"),
		Entry("dot host with port", "file://.:22/bar.yaml"),
		Entry("oci", "oci://registry.example.com/manifest:1.0"),
		Entry("unparseable", "file:// /bar.yaml"),
	)

	It("names both forms in the warning", func() {
		msg := source.LegacyRelativeWarning("file://./bar.yaml", "./bar.yaml")
		Expect(msg).To(ContainSubstring(`"file://./bar.yaml"`))
		Expect(msg).To(ContainSubstring("deprecated"))
		Expect(msg).To(ContainSubstring(`"./bar.yaml"`))
	})
})
