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

package overlay

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestOverlaySuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Overlay test suite")
}

var _ = Describe("Overlay Tests", func() {
	var tfs vfs.FS
	var cleanup func()
	var mounter *mock.Mounter

	BeforeEach(func() {
		var err error
		tfs, cleanup, err = mock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())

		mounter = mock.NewMounter()
	})
	AfterEach(func() {
		cleanup()
	})

	It("Creates an overlay with predefined work and merged directories", func() {
		overlay, err := New(
			"/var/.lower",
			"/var/.upper",
			nil,
			tfs,
			WithWorkDir("/var/.work"),
			WithTarget("/var/.merged"))
		Expect(err).ToNot(HaveOccurred())

		Expect(overlay.lowerDir).To(Equal("/var/.lower"))
		Expect(overlay.upperDir).To(Equal("/var/.upper"))
		Expect(overlay.workDir).To(Equal("/var/.work"))
		Expect(overlay.mergedDir).To(Equal("/var/.merged"))
		Expect(overlay.tempDir).To(BeEmpty())
	})

	It("Fails to mount and unmount", func() {
		mounter.ErrorOnMount = true
		mounter.ErrorOnUnmount = true

		overlay, err := New(
			"/var/.lower",
			"/var/.upper",
			mounter,
			tfs,
			WithWorkDir("/var/.work"),
			WithTarget("/var/.merged"))
		Expect(err).ToNot(HaveOccurred())

		mergedDir, err := overlay.Mount()
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("mount error"))
		Expect(mergedDir).To(BeEmpty())

		err = overlay.Unmount()
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("unmount error"))
	})

	It("Successfully mounts and unmounts", func() {
		overlay, err := New(
			"/var/.lower",
			"/var/.upper",
			mounter,
			tfs,
			WithWorkDir("/var/.work"),
			WithTarget("/var/.merged"))
		Expect(err).ToNot(HaveOccurred())

		mergedDir, err := overlay.Mount()
		Expect(err).ToNot(HaveOccurred())
		Expect(mergedDir).To(Equal("/var/.merged"))

		Expect(mounter.IsMountPoint("/var/.merged")).To(BeTrue())

		mountPoints, err := mounter.List()
		Expect(err).ToNot(HaveOccurred())
		Expect(mountPoints).To(HaveLen(1))

		mountPoint := mountPoints[0]
		Expect(mountPoint.Path).To(Equal("/var/.merged"))
		Expect(mountPoint.Type).To(Equal("overlay"))
		Expect(mountPoint.Device).To(Equal("overlay"))
		Expect(mountPoint.Opts).To(HaveLen(3))
		Expect(mountPoint.Opts).To(ContainElement("lowerdir=/var/.lower"))
		Expect(mountPoint.Opts).To(ContainElement("upperdir=/var/.upper"))
		Expect(mountPoint.Opts).To(ContainElement("workdir=/var/.work"))

		Expect(overlay.Unmount()).To(Succeed())

		Expect(mounter.IsMountPoint("/var/.merged")).To(BeFalse())
	})

	It("Creates an overlay with temporary work and merged directories", func() {
		overlay, err := New("/var/.lower", "/var/.upper", mounter, tfs)
		Expect(err).ToNot(HaveOccurred())

		Expect(overlay.lowerDir).To(Equal("/var/.lower"))
		Expect(overlay.upperDir).To(Equal("/var/.upper"))
		Expect(overlay.workDir).To(ContainSubstring("/overlay-/.work"))
		Expect(overlay.mergedDir).To(ContainSubstring("/overlay-/.merged"))
		Expect(overlay.tempDir).To(ContainSubstring("/overlay-"))

		Expect(vfs.Exists(tfs, overlay.workDir)).To(BeTrue())
		Expect(vfs.Exists(tfs, overlay.mergedDir)).To(BeTrue())
		Expect(vfs.Exists(tfs, overlay.tempDir)).To(BeTrue())

		Expect(overlay.Unmount()).To(Succeed(), "Clean up temp dirs on unmounting")

		Expect(vfs.Exists(tfs, overlay.workDir)).To(BeFalse())
		Expect(vfs.Exists(tfs, overlay.mergedDir)).To(BeFalse())
		Expect(vfs.Exists(tfs, overlay.tempDir)).To(BeFalse())
	})
})
