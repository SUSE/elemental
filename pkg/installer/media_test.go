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

package installer_test

import (
	"context"
	"fmt"

	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/installer"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestInstallerMediaSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "InstallerMedia test suite")
}

var _ = Describe("Install", Label("install"), func() {
	var runner *sysmock.Runner
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var d *deployment.Deployment

	var sideEffects map[string]func(...string) ([]byte, error)
	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		sideEffects = map[string]func(...string) ([]byte, error){}
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/dev/device":  []byte{},
			"/dev/device1": []byte{},
			"/dev/device2": []byte{},
		})
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithRunner(runner), sys.WithFS(fs),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
		d = deployment.DefaultDeployment()
		d.Installer = deployment.LiveInstaller{}
		runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			if f := sideEffects[cmd]; f != nil {
				return f(args...)
			}
			return runner.ReturnValue, runner.ReturnError
		}
		Expect(vfs.MkdirAll(fs, "/some/dir", vfs.DirPerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("Creates an installation ISO", func() {
		sideEffects["xorriso"] = func(args ...string) ([]byte, error) {
			Expect(fs.WriteFile("/some/dir/build/installer.iso", []byte("data"), vfs.FilePerm)).To(Succeed())
			return []byte{}, nil
		}

		d.SourceOS = deployment.NewDirSrc("/some/root")
		d.Installer.OverlayTree = deployment.NewDirSrc("/some/dir/iso-overlay")
		d.Installer.CfgScript = "/some/dir/config-live.sh"

		iso := installer.NewISO(context.Background(), s, installer.WithBootloader(bootloader.NewNone(s)))

		iso.OutputDir = "/some/dir/build"
		d.CfgScript = "/some/dir/config.sh"
		d.OverlayTree = deployment.NewDirSrc("/some/dir/install-overlay")

		Expect(vfs.MkdirAll(fs, "/some/dir/iso-overlay", vfs.DirPerm)).To(Succeed())
		Expect(vfs.MkdirAll(fs, "/some/dir/install-overlay", vfs.DirPerm)).To(Succeed())
		Expect(fs.WriteFile("/some/dir/config-live.sh", []byte("live config script"), vfs.FilePerm)).To(Succeed())
		Expect(fs.WriteFile("/some/dir/config.sh", []byte("install config script"), vfs.FilePerm)).To(Succeed())

		Expect(iso.Build(d)).To(Succeed())
		Expect(runner.MatchMilestones([][]string{
			{"mksquashfs", "/some/dir/build/elemental-installer/rootfs", "/some/dir/build/elemental-installer/iso/LiveOS/squashfs.img"},
			{"mkfs.vfat", "-n", "EFI", "/some/dir/build/elemental-installer/efi.img"},
			{"mcopy", "-s", "-i", "/some/dir/build/elemental-installer/efi.img", "/some/dir/build/elemental-installer/efi/EFI", "::"},
			{"xorriso", "-volid", "LIVE", "-padding", "0", "-outdev", "/some/dir/build/installer.iso"},
		}))
	})
	It("fails to create an ISO without an output directory defined", func() {
		d.SourceOS = deployment.NewDirSrc("/some/root")
		iso := installer.NewISO(context.Background(), s, installer.WithBootloader(bootloader.NewNone(s)))

		err := iso.Build(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("undefined output directory"))
	})
	It("fails to create an ISO on a readonly FS", func() {
		roFS, err := sysmock.ReadOnlyTestFS(fs)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithRunner(runner), sys.WithFS(roFS),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())

		d.SourceOS = deployment.NewDirSrc("/some/root")
		iso := installer.NewISO(context.Background(), s, installer.WithBootloader(bootloader.NewNone(s)))
		iso.OutputDir = "/some/dir/build"

		err = iso.Build(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("operation not permitted"))
	})
	It("fails to sync OS content", func() {
		sideEffects["rsync"] = func(args ...string) ([]byte, error) {
			return []byte{}, fmt.Errorf("rsync command failed")
		}

		d.SourceOS = deployment.NewDirSrc("/some/root")
		iso := installer.NewISO(context.Background(), s, installer.WithBootloader(bootloader.NewNone(s)))
		iso.OutputDir = "/some/dir/build"

		err := iso.Build(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("rsync command failed"))
	})
	It("fails to burn ISO", func() {
		sideEffects["xorriso"] = func(args ...string) ([]byte, error) {
			return []byte{}, fmt.Errorf("xorriso command failed")
		}

		d.SourceOS = deployment.NewDirSrc("/some/root")
		iso := installer.NewISO(context.Background(), s, installer.WithBootloader(bootloader.NewNone(s)))
		iso.OutputDir = "/some/dir/build"

		err := iso.Build(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("xorriso command failed"))
	})
	It("customizes an ISO", func() {
		Expect(vfs.MkdirAll(fs, "/some/dir/build", vfs.DirPerm)).To(Succeed())

		// Create the file pointed out by -outdev when xorriso is called.
		sideEffects["xorriso"] = func(args ...string) ([]byte, error) {
			for i, arg := range args {
				if arg != "-outdev" {
					continue
				}

				file := args[i+1]
				_, err := fs.Create(file)
				Expect(err).To(Succeed())

				break
			}
			return []byte{}, nil
		}

		_, err := fs.Create("/some/dir/installer.iso")
		Expect(err).To(Succeed())

		iso := installer.NewISO(context.Background(), s, installer.WithBootloader(bootloader.NewNone(s)))
		iso.InputFile = "/some/dir/installer.iso"
		iso.OutputDir = "/some/dir/build"
		iso.Name = "installer2"

		Expect(iso.Customize(d)).To(Succeed())

		Expect(vfs.Exists(fs, "/some/dir/build/installer2.iso")).To(BeTrue())
	})
	It("fails to customize non-existent input file", func() {
		iso := installer.NewISO(context.Background(), s)
		iso.InputFile = "/non-existent/installer.iso"
		iso.OutputDir = "/some/dir/build"
		iso.Name = "installer2.iso"

		err := iso.Customize(d)
		Expect(err).ToNot(Succeed())
		Expect(err.Error()).To(ContainSubstring("target input file /non-existent/installer.iso does not exist"))
	})
	It("fails to customize an ISO using xorriso", func() {
		// Create the file pointed out by -outdev when xorriso is called.
		sideEffects["xorriso"] = func(args ...string) ([]byte, error) {
			return []byte{}, fmt.Errorf("failed to run xorriso")
		}

		_, err := fs.Create("/some/dir/installer.iso")
		Expect(err).To(Succeed())

		iso := installer.NewISO(context.Background(), s, installer.WithBootloader(bootloader.NewNone(s)))
		iso.InputFile = "/some/dir/installer.iso"
		iso.OutputDir = "/some/dir/build"
		iso.Name = "installer2"

		err = iso.Customize(d)
		Expect(err).ToNot(Succeed())
		Expect(err.Error()).To(ContainSubstring("failed to run xorriso"))
	})
})
