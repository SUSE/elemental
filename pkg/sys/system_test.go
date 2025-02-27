/*
Copyright © 2021 SUSE LLC

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/sys"
	mocksys "github.com/suse/elemental/v3/pkg/sys/mock"
)

var _ = Describe("System", Label("system"), func() {
	var mounter *mocksys.FakeMounter
	var runner *mocksys.FakeRunner
	var syscall *mocksys.FakeSyscall
	var logger sys.Logger
	var fs sys.FS
	BeforeEach(func() {
		mounter = mocksys.NewFakeMounter()
		runner = mocksys.NewFakeRunner()
		syscall = &mocksys.FakeSyscall{}
		logger = sys.NewNullLogger()
		fs, _, _ = mocksys.TestFS(nil)
	})
	It("It exits out when tyring to get an unitialized system instance", func() {
		Expect(func() { _ = sys.GetSystem() }).To(Panic())
	})
	It("Can be set to use custom implementations", func() {
		platform, err := sys.ParsePlatform("linux/arm64")
		Expect(err).NotTo(HaveOccurred())
		sys.SetSystem(
			sys.WithFS(fs), sys.WithLogger(logger),
			sys.WithMounter(mounter), sys.WithPlatform("linux/arm64"),
			sys.WithRunner(runner), sys.WithSyscall(syscall),
		)
		s := sys.GetSystem()
		Expect(s.Runner).To(BeIdenticalTo(runner))
		Expect(s.Mounter).To(BeIdenticalTo(mounter))
		Expect(s.FS).To(BeIdenticalTo(fs))
		Expect(s.Logger).To(BeIdenticalTo(logger))
		Expect(s.Syscall).To(BeIdenticalTo(syscall))
		Expect(s.Platform).To(Equal(platform))
	})
	It("It it is initalized with all defaults", func() {
		platform, err := sys.NewPlatformFromArch(runtime.GOARCH)
		Expect(err).NotTo(HaveOccurred())
		sys.SetSystem()
		s := sys.GetSystem()
		Expect(s.Runner).NotTo(BeIdenticalTo(runner))
		Expect(s.Platform).To(Equal(platform))
	})

})
