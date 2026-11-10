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

package qemuimg_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/qemuimg"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestQemuImgSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "qemu-img test suite")
}

var _ = Describe("qemu-img", Label("qemuimg"), func() {
	var runner *sysmock.Runner
	var fs vfs.FS
	var cleanup func()
	var s *sys.System

	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/work/tmp/disk.raw": []byte{},
			"/images/base.qcow2": []byte{},
			"/images/out/.keep":  []byte{},
		})
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithRunner(runner), sys.WithFS(fs),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).ToNot(HaveOccurred())
	})
	AfterEach(func() {
		cleanup()
	})

	It("converts a raw image into a sparse qcow2 image", func() {
		Expect(qemuimg.ConvertToQCOW2(s, "/work/tmp/disk.raw", "/images/out/disk.qcow2")).To(Succeed())
		Expect(runner.CmdsMatch([][]string{
			{"qemu-img", "convert", "-f", "raw", "-O", "qcow2", "/work/tmp/disk.raw", "/images/out/disk.qcow2"},
		})).To(Succeed())
	})

	It("fails to convert if the target image already exists", func() {
		err := qemuimg.ConvertToQCOW2(s, "/work/tmp/disk.raw", "/images/base.qcow2")
		Expect(err).To(MatchError(ContainSubstring("already exists")))
		Expect(runner.GetCmds()).To(BeEmpty())
	})

	It("fails if qemu-img convert fails", func() {
		runner.ReturnError = fmt.Errorf("convert failed")
		err := qemuimg.ConvertToQCOW2(s, "/work/tmp/disk.raw", "/images/out/disk.qcow2")
		Expect(err).To(MatchError(ContainSubstring("convert failed")))
	})

	It("creates an overlay referencing the base image by a relative path", func() {
		Expect(qemuimg.CreateOverlay(s, "/images/base.qcow2", "/images/node1.qcow2")).To(Succeed())
		Expect(qemuimg.CreateOverlay(s, "/images/base.qcow2", "/images/out/node2.qcow2")).To(Succeed())
		Expect(runner.CmdsMatch([][]string{
			{"qemu-img", "create", "-f", "qcow2", "-b", "base.qcow2", "-F", "qcow2", "/images/node1.qcow2"},
			{"qemu-img", "create", "-f", "qcow2", "-b", "../base.qcow2", "-F", "qcow2", "/images/out/node2.qcow2"},
		})).To(Succeed())
	})

	It("fails to create an overlay if the base image does not exist", func() {
		err := qemuimg.CreateOverlay(s, "/images/missing.qcow2", "/images/node1.qcow2")
		Expect(err).To(MatchError(ContainSubstring("does not exist")))
		Expect(runner.GetCmds()).To(BeEmpty())
	})

	It("fails to create an overlay over an existing file", func() {
		err := qemuimg.CreateOverlay(s, "/images/base.qcow2", "/work/tmp/disk.raw")
		Expect(err).To(MatchError(ContainSubstring("already exists")))
		Expect(runner.GetCmds()).To(BeEmpty())
	})

	It("fails if qemu-img create fails", func() {
		runner.ReturnError = fmt.Errorf("create failed")
		err := qemuimg.CreateOverlay(s, "/images/base.qcow2", "/images/node1.qcow2")
		Expect(err).To(MatchError(ContainSubstring("create failed")))
	})
})
