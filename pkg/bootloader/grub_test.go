/*
Copyright © 2022-2025 SUSE LLC
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

package bootloader_test

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Grub tests", Label("bootloader", "grub"), func() {
	var tfs vfs.FS
	var s *sys.System
	var cleanup func()
	var grub *bootloader.Grub
	var esp *deployment.Partition
	var runner *sysmock.Runner
	var syscall *sysmock.Syscall
	var mounter *sysmock.Mounter
	BeforeEach(func() {
		var err error
		tfs, cleanup, err = sysmock.TestFS(map[string]any{
			"/dev/pts/empty": []byte{},
			"/proc/empty":    []byte{},
			"/sys/empty":     []byte{},
		})

		Expect(err).NotTo(HaveOccurred())

		runner = sysmock.NewRunner()
		syscall = &sysmock.Syscall{}
		mounter = sysmock.NewMounter()
		s, err = sys.NewSystem(
			sys.WithSyscall(syscall),
			sys.WithRunner(runner),
			sys.WithFS(tfs),
			sys.WithLogger(log.New(log.WithDiscardAll())),
			sys.WithMounter(mounter),
		)
		Expect(err).NotTo(HaveOccurred())

		runner.SideEffect = func(command string, args ...string) ([]byte, error) {
			switch filepath.Base(command) {
			// create the initrd specified in the second-to-last argument. (inside /target/dir, since the real code chroots into the install target
			case "dracut":
				initrdPath := args[len(args)-2]
				_, err := tfs.Create(filepath.Join("/target/dir", initrdPath))
				Expect(err).NotTo(HaveOccurred())
				return nil, nil
			case "grub2-editenv":
				_, err := tfs.Create(args[0])
				Expect(err).NotTo(HaveOccurred())
				return nil, nil
			case "rsync":
				return nil, nil
			}

			return nil, fmt.Errorf("command '%s', %w", command, errors.ErrUnsupported)
		}

		esp = &deployment.Partition{
			Role:       deployment.EFI,
			MountPoint: "/boot/efi",
		}

		grub = bootloader.NewGrub(s)

		// Setup GRUB and EFI dirs
		Expect(vfs.MkdirAll(tfs, "/target/dir/usr/share/efi/x86_64", vfs.DirPerm)).To(Succeed())
		Expect(vfs.MkdirAll(tfs, "/target/dir/usr/share/efi/aarch64", vfs.DirPerm)).To(Succeed())
		Expect(vfs.MkdirAll(tfs, "/target/dir/usr/share/grub2/x86_64-efi", vfs.DirPerm)).To(Succeed())
		Expect(vfs.MkdirAll(tfs, "/target/dir/usr/share/grub2/arm64-efi", vfs.DirPerm)).To(Succeed())

		Expect(tfs.WriteFile("/target/dir/usr/share/efi/x86_64/shim-opensuse.efi", []byte("x86_64 shim-opensuse.efi"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/target/dir/usr/share/efi/x86_64/MokManager.efi", []byte("x86_64 MokManager.efi"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/target/dir/usr/share/grub2/x86_64-efi/grub.efi", []byte("x86_64 grub.efi"), vfs.FilePerm)).To(Succeed())

		Expect(tfs.Symlink("/target/dir/usr/share/efi/x86_64/shim-opensuse.efi", "/target/dir/usr/share/efi/x86_64/shim.efi")).To(Succeed())
		Expect(tfs.Symlink("/target/dir/usr/share/grub2/x86_64-efi/grub.efi", "/target/dir/usr/share/efi/x86_64/grub.efi")).To(Succeed())

		// Setup /etc/os-release file with openSUSE tumbleweed ID
		Expect(vfs.MkdirAll(tfs, "/target/dir/etc", vfs.DirPerm)).To(Succeed())
		Expect(tfs.WriteFile("/target/dir/etc/os-release", []byte("ID=opensuse-tumbleweed\nNAME=openSUSE Tumbleweed"), vfs.FilePerm)).To(Succeed())
		// Setup kernel dirs
		Expect(vfs.MkdirAll(tfs, "/target/dir/usr/lib/modules/6.14.4-1-default", vfs.DirPerm)).To(Succeed())
		Expect(tfs.WriteFile("/target/dir/usr/lib/modules/6.14.4-1-default/vmlinuz", []byte("6.14.4-1-default vmlinux.xz"), vfs.FilePerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("Fails installing bootloader to System partition", func() {
		sysPart := &deployment.Partition{
			Role: deployment.System,
		}
		err := grub.Install("/target/dir", sysPart)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, errors.ErrUnsupported)).To(BeTrue())
	})
	It("Copies EFI applications to ESP", func() {
		err := grub.Install("/target/dir", esp)
		Expect(err).ToNot(HaveOccurred())

		// shim must not be a symlink
		file, err := tfs.Lstat("/target/dir/boot/efi/EFI/ELEMENTAL/shim.efi")
		Expect(err).ToNot(HaveOccurred())
		Expect(file.Mode() & fs.ModeSymlink).To(Equal(fs.FileMode(0)))

		// Shim, MokManager and grub.efi should exist.
		Expect(vfs.Exists(tfs, "/target/dir/boot/efi/EFI/ELEMENTAL/shim.efi")).To(BeTrue())
		Expect(vfs.Exists(tfs, "/target/dir/boot/efi/EFI/ELEMENTAL/MokManager.efi")).To(BeTrue())
		Expect(vfs.Exists(tfs, "/target/dir/boot/efi/EFI/ELEMENTAL/grub.efi")).To(BeTrue())

		// Kernel and initrd exist
		Expect(vfs.Exists(tfs, "/target/dir/boot/efi/opensuse-tumbleweed/6.14.4-1-default/vmlinuz")).To(BeTrue())
		Expect(vfs.Exists(tfs, "/target/dir/boot/efi/opensuse-tumbleweed/6.14.4-1-default/initrd")).To(BeTrue())

		// Grub env and loader entries files exist
		Expect(vfs.Exists(tfs, "/target/dir/boot/efi/grubenv")).To(BeTrue())
		Expect(vfs.Exists(tfs, "/target/dir/boot/efi/loader/entries/active")).To(BeTrue())
	})
})
