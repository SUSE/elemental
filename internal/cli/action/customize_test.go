/*
Copyright © 2026 SUSE LLC
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

package action_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/cli/action"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Customize overlays", Label("customize"), func() {
	var fs vfs.FS
	var cleanup func()

	BeforeEach(func() {
		var err error
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/config/existing.qcow2": []byte{},
		})
		Expect(err).ToNot(HaveOccurred())
	})
	AfterEach(func() {
		cleanup()
	})

	It("places overlays next to the output image", func() {
		paths, err := action.ResolveOverlayPaths(fs, "/config/base.qcow2", []string{"node1", "node2.qcow2", " node3 "})
		Expect(err).ToNot(HaveOccurred())
		Expect(paths).To(Equal([]string{"/config/node1.qcow2", "/config/node2.qcow2", "/config/node3.qcow2"}))
	})

	It("returns no paths when no overlays are requested", func() {
		paths, err := action.ResolveOverlayPaths(fs, "/config/base.qcow2", nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(paths).To(BeEmpty())
	})

	DescribeTable("rejects invalid overlay names",
		func(names []string, msg string) {
			_, err := action.ResolveOverlayPaths(fs, "/config/base.qcow2", names)
			Expect(err).To(MatchError(ContainSubstring(msg)))
		},
		Entry("empty name", []string{""}, "invalid overlay name"),
		Entry("path instead of name", []string{"sub/node1"}, "invalid overlay name"),
		Entry("parent directory", []string{".."}, "invalid overlay name"),
		Entry("same as output image", []string{"base"}, "same path as the output image"),
		Entry("duplicated name", []string{"node1", "node1.qcow2"}, "more than once"),
		Entry("existing file", []string{"existing"}, "already exists"),
	)

	DescribeTable("rejects overlays colliding with a non-normalized output image path",
		func(imagePath string) {
			_, err := action.ResolveOverlayPaths(fs, imagePath, []string{"base"})
			Expect(err).To(MatchError(ContainSubstring("same path as the output image")))
		},
		Entry("relative path with leading ./", "./base.qcow2"),
		Entry("absolute path with ./ component", "/config/./base.qcow2"),
		Entry("absolute path with ../ component", "/config/sub/../base.qcow2"),
	)

	It("places overlays next to a non-normalized output image path", func() {
		paths, err := action.ResolveOverlayPaths(fs, "/config/sub/../base.qcow2", []string{"node1"})
		Expect(err).ToNot(HaveOccurred())
		Expect(paths).To(Equal([]string{"/config/node1.qcow2"}))
	})
})
