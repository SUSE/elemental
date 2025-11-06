package kmod

import (
	"bytes"
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
	var logBuffer *bytes.Buffer

	BeforeEach(func() {
		var err error

		tfs, cleanup, err = mock.TestFS(map[string]string{
			"/tmp/abc/def/ghi": "",
		})
		Expect(err).NotTo(HaveOccurred())

		runner = mock.NewRunner()
		mounter = mock.NewMounter()
		logBuffer = bytes.NewBuffer(nil)

		s, err = sys.NewSystem(sys.WithFS(tfs),
			sys.WithRunner(runner),
			sys.WithMounter(mounter),
			sys.WithLogger(log.New(log.WithBuffer(logBuffer))))
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("Skips unload operation if mount point is not found", func() {
		u := &Unloader{
			System: s,
			Config: conf,
		}
		Expect(u.Unload(context.Background(), nil)).To(Succeed())

		Expect(logBuffer.String()).To(ContainSubstring("Kernel modules mount point not found, skipping unload"))
	})

	It("Fails to remove module", func() {
		Expect(mounter.Mount("/var/xyz", "/tmp/xyz", "", nil)).To(Succeed())

		runner.SideEffect = func(command string, args ...string) ([]byte, error) {
			if command != "modprobe" {
				return []byte{}, fmt.Errorf("unexpected command: %s", command)
			}

			if !slices.Contains(args, "-r") {
				return []byte{}, fmt.Errorf("flag -r not provided in unload operation")
			}

			return []byte{}, fmt.Errorf("modprobe error")
		}

		u := &Unloader{
			System: s,
			Config: conf,
		}

		err := u.Unload(context.Background(), []string{"nvidia"})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("deactivating module nvidia: modprobe error"))
	})

	It("Fails to unmount mount point", func() {
		Expect(mounter.Mount("/var/xyz", "/tmp/xyz", "", nil)).To(Succeed())
		mounter.ErrorOnUnmount = true

		u := &Unloader{
			System: s,
			Config: conf,
		}

		err := u.Unload(context.Background(), nil)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("unmounting /tmp/xyz: unmount error"))
	})

	It("Successfully unloads modules", func() {
		Expect(mounter.Mount("/var/xyz", "/tmp/xyz", "", nil)).To(Succeed())
		Expect(mounter.IsMountPoint("/tmp/xyz")).To(BeTrue())

		runner.SideEffect = func(command string, args ...string) ([]byte, error) {
			if command != "modprobe" {
				return []byte{}, fmt.Errorf("unexpected command: %s", command)
			}

			if !slices.Contains(args, "-r") {
				return []byte{}, fmt.Errorf("flag -r not provided in unload operation")
			}

			return []byte{}, nil
		}

		u := &Unloader{
			System: s,
			Config: conf,
		}

		Expect(u.Unload(context.Background(), []string{"nvidia"})).To(Succeed())

		Expect(logBuffer.String()).To(ContainSubstring("Unloading kernel modules completed"))
		Expect(mounter.IsMountPoint("/tmp/xyz")).To(BeFalse())
		Expect(vfs.Exists(tfs, "/tmp/abc")).To(BeFalse())
	})
})
