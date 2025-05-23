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

package upgrade_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/transaction"
	transmock "github.com/suse/elemental/v3/pkg/transaction/mock"
	"github.com/suse/elemental/v3/pkg/upgrade"
)

func TestUpgradeSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Upgrade test suite")
}

var _ = Describe("Upgrade", Label("upgrade"), func() {
	var runner *sysmock.Runner
	var mounter *sysmock.Mounter
	var syscall *sysmock.Syscall
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var d *deployment.Deployment
	var u *upgrade.Upgrader
	var t *transmock.Transactioner
	var trans *transaction.Transaction

	BeforeEach(func() {
		var err error
		syscall = &sysmock.Syscall{}
		t = &transmock.Transactioner{}
		runner = sysmock.NewRunner()
		mounter = sysmock.NewMounter()
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/dev/pts/empty":         []byte{},
			"/proc/empty":            []byte{},
			"/sys/empty":             []byte{},
			"/snapshot/path/empty":   []byte{},
			"/opt/overlaytree/empty": []byte{},
			"/opt/config.sh":         []byte{},
		})
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithMounter(mounter), sys.WithRunner(runner),
			sys.WithFS(fs), sys.WithLogger(log.New(log.WithDiscardAll())),
			sys.WithSyscall(syscall),
		)
		Expect(err).NotTo(HaveOccurred())

		d = deployment.DefaultDeployment()
		d.SourceOS = deployment.NewDirSrc("/some/dir")
		d.OverlayTree = deployment.NewDirSrc("/opt/overlaytree")
		d.CfgScript = "/opt/config.sh"
		Expect(d.Sanitize(s)).To(Succeed())
		u = upgrade.New(context.Background(), s, upgrade.WithTransaction(t))

		trans = &transaction.Transaction{
			ID:   2,
			Path: "/snapshot/path",
		}
		t.Trans = trans
		t.SrcDigest = "imagedigest"
	})
	AfterEach(func() {
		cleanup()
	})
	It("upgrades to the given deployment", func() {
		Expect(u.Upgrade(d)).To(Succeed())
		Expect(d.SourceOS.GetDigest()).To(Equal("imagedigest"))
		Expect(runner.MatchMilestones([][]string{
			{"rsync"},
			{"/etc/elemental/config.sh"},
		}))
	})
	It("fails on transaction initalization", func() {
		t.InitErr = fmt.Errorf("init failed")
		err := u.Upgrade(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("init failed"))
		Expect(t.RollbackCalled()).To(BeFalse())
	})
	It("fails on transaction start", func() {
		t.StartErr = fmt.Errorf("start failed")
		err := u.Upgrade(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("start failed"))
		Expect(t.RollbackCalled()).To(BeFalse())
	})
	It("fails on image sync", func() {
		t.UpgradeHelper.SyncError = fmt.Errorf("failed sync")
		err := u.Upgrade(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed sync"))
	})
	It("fails on image merge", func() {
		t.UpgradeHelper.MergeError = fmt.Errorf("failed merge")
		err := u.Upgrade(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed merge"))
	})
	It("fails on fstab update", func() {
		t.UpgradeHelper.FstabError = fmt.Errorf("failed fstab update")
		err := u.Upgrade(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed fstab update"))
	})
	It("fails on locking snapshot", func() {
		t.UpgradeHelper.LockError = fmt.Errorf("failed lock")
		err := u.Upgrade(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed lock"))
	})
	It("fails on unpacking overlay", func() {
		runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			if cmd == "rsync" {
				for _, arg := range args {
					if strings.Contains(arg, "/opt/overlaytree/") {
						return []byte{}, fmt.Errorf("failed to sync overlaytree")
					}
				}
			}
			return []byte{}, nil
		}
		err := u.Upgrade(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to sync overlaytree"))
	})
	It("fails on config script execution", func() {
		runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			if cmd == "/etc/elemental/config.sh" {
				return []byte{}, fmt.Errorf("failed to running config script")
			}
			return []byte{}, nil
		}
		err := u.Upgrade(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to running config script"))
	})
	It("fails on transaction commit", func() {
		t.CommitErr = fmt.Errorf("commit failed")
		err := u.Upgrade(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("commit failed"))
		Expect(t.RollbackCalled()).To(BeTrue())
	})
})
