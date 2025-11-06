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

package kmod

import (
	"context"
	"fmt"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type moduleCacheMock struct {
	Err error
}

func (m *moduleCacheMock) Build(context.Context, string) error {
	return m.Err
}

var _ = Describe("Reloading tests", func() {
	var s *sys.System
	var tfs vfs.FS
	var cleanup func()
	var runner *mock.Runner
	var mounter *mock.Mounter
	var conf = &Config{
		BaseDir:    "/tmp/abc",
		MountPoint: "/tmp/xyz",
	}

	BeforeEach(func() {
		var err error

		tfs, cleanup, err = mock.TestFS(map[string]string{
			"/tmp/xyz/usr/lib/modules/modules.dep": "",
		})
		Expect(err).NotTo(HaveOccurred())

		runner = mock.NewRunner()
		mounter = mock.NewMounter()

		s, err = sys.NewSystem(sys.WithFS(tfs),
			sys.WithRunner(runner),
			sys.WithMounter(mounter),
			sys.WithLogger(log.New(log.WithDiscardAll())))
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("Fails to prepare environment", func() {
		Expect(mounter.Mount("/var/xyz", "/tmp/xyz", "", nil)).To(Succeed())
		mounter.ErrorOnUnmount = true

		r := &Reloader{
			System: s,
			Config: conf,
		}

		err := r.Reload(context.Background(), nil)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("preparing environment: cleaning up environment: unmounting /tmp/xyz: unmount error"))
	})

	It("Fails to build module cache", func() {
		r := &Reloader{
			System: s,
			Config: conf,
			ModuleCache: &moduleCacheMock{
				Err: fmt.Errorf("cache error"),
			},
		}

		err := r.Reload(context.Background(), nil)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("building modules cache: cache error"))
	})

	It("Fails to mount overlay", func() {
		mounter.ErrorOnMount = true

		r := &Reloader{
			System:      s,
			Config:      conf,
			ModuleCache: &moduleCacheMock{},
		}

		err := r.Reload(context.Background(), nil)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("mounting overlay filesystem: mount error"))
	})

	It("Fails to create symlink for kernel path", func() {
		r := &Reloader{
			System:      s,
			Config:      conf,
			ModuleCache: &moduleCacheMock{},
			KernelDir:   "/tmp/zzz/6.12.0-160000.20-default",
		}

		err := r.Reload(context.Background(), nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("linking kernel module tree: creating symlink at"))
		Expect(err.Error()).To(ContainSubstring("/tmp/zzz/6.12.0-160000.20-default: no such file or directory"))
	})

	It("Fails to activate modules", func() {
		r := &Reloader{
			System:      s,
			Config:      conf,
			ModuleCache: &moduleCacheMock{},
			KernelDir:   "/usr/lib/modules/6.12.0-160000.15-default",
		}

		runner.SideEffect = func(command string, args ...string) ([]byte, error) {
			if command != "modprobe" {
				return []byte{}, fmt.Errorf("unexpected command: %s", command)
			}

			return []byte{}, fmt.Errorf("modprobe error")
		}

		err := r.Reload(context.Background(), []string{"nvidia"})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("activating module nvidia: modprobe error"))
	})

	It("Successfully activates modules", func() {
		r := &Reloader{
			System:      s,
			Config:      conf,
			ModuleCache: &moduleCacheMock{},
			KernelDir:   "/usr/lib/modules/6.12.0-160000.15-default",
		}

		runner.SideEffect = func(command string, args ...string) ([]byte, error) {
			if command != "modprobe" {
				return []byte{}, fmt.Errorf("unexpected command: %s", command)
			}

			if slices.Contains(args, "-r") {
				return []byte{}, fmt.Errorf("flag -r provided in reload operation")
			}

			return []byte{}, nil
		}

		Expect(r.Reload(context.Background(), []string{"nvidia"})).To(Succeed())
	})
})
