/*
Copyright © 2022-2026 SUSE LLC
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

package sys_test

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	mocksys "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/platform"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestSysSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sys test suite")
}

var _ = Describe("System", Label("system"), func() {
	var mounter *mocksys.Mounter
	var runner *mocksys.Runner
	var syscall *mocksys.Syscall
	var logger log.Logger
	var fs vfs.FS
	BeforeEach(func() {
		mounter = mocksys.NewMounter()
		runner = mocksys.NewRunner()
		syscall = &mocksys.Syscall{}
		logger = log.New(log.WithDiscardAll())
		fs, _, _ = mocksys.TestFS(nil)
	})
	It("Can be set to use custom implementations", func() {
		platform, err := platform.Parse("linux/arm64")
		Expect(err).NotTo(HaveOccurred())
		s, err := sys.NewSystem(
			sys.WithFS(fs), sys.WithLogger(logger),
			sys.WithMounter(mounter), sys.WithPlatform("linux/arm64"),
			sys.WithRunner(runner), sys.WithSyscall(syscall),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.Runner()).To(BeIdenticalTo(runner))
		Expect(s.Mounter()).To(BeIdenticalTo(mounter))
		Expect(s.FS()).To(BeIdenticalTo(fs))
		Expect(s.Logger()).To(BeIdenticalTo(logger))
		Expect(s.Syscall()).To(BeIdenticalTo(syscall))
		Expect(s.Platform()).To(Equal(platform))
	})
	It("It is initalized with all defaults", func() {
		platform, err := platform.NewFromArch(runtime.GOARCH)
		Expect(err).NotTo(HaveOccurred())
		s, err := sys.NewSystem()
		Expect(err).ToNot(HaveOccurred())
		Expect(s.Runner()).NotTo(BeIdenticalTo(runner))
		Expect(s.Platform()).To(Equal(platform))
	})
	It("Fails with invalid platform", func() {
		_, err := sys.NewSystem(
			sys.WithFS(fs), sys.WithLogger(logger),
			sys.WithMounter(mounter), sys.WithPlatform("linux/s390"),
			sys.WithRunner(runner), sys.WithSyscall(syscall),
		)
		Expect(err).To(HaveOccurred())
	})
	It("Checks command existence in path", func() {
		Expect(sys.CommandExists("true")).To(BeTrue())
		Expect(sys.CommandExists("non-existing-command")).To(BeFalse())
		// If full path provided it does not check on PATH
		Expect(sys.CommandExists("/sh")).To(BeFalse())
	})
})
