/*
Copyright © 2025-2026 SUSE LLC
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
	"bytes"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/urfave/cli/v3"

	"github.com/suse/elemental/v3/internal/cli/action"
	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/installer"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const lsblkJson = `{
	"blockdevices": [
	   {
	      "label": "RECOVERY",
		  "partlabel": "RECOVERY",
		  "partuuid": "ddb334a8-48a2-c4de-ddb3-849eb2443e92",
		  "size": 2726297600,
		  "fstype": "btrfs",
		  "mountpoints": ["/run/initramfs/live"],
		  "path": "/dev/device2",
		  "pkname": "/dev/device",
		  "type": "part"
	   }
	]
 }`

var _ = Describe("Reset action", Label("reset"), func() {
	var s *sys.System
	var tfs vfs.FS
	var cleanup func()
	var err error
	var cliCmd *cli.Command
	var buffer *bytes.Buffer
	var mounter *sysmock.Mounter
	var runner *sysmock.Runner

	BeforeEach(func() {
		cmd.InstallArgs = cmd.InstallFlags{}
		buffer = &bytes.Buffer{}
		tfs, cleanup, err = sysmock.TestFS(map[string]string{
			"/dev/device":          "device",
			installer.SquashfsPath: "liveimage",
			installer.InstallDesc:  "",
			"/proc/cmdline":        deployment.RecoveryMark,
		})
		Expect(err).NotTo(HaveOccurred())
		mounter = sysmock.NewMounter()
		runner = sysmock.NewRunner()
		s, err = sys.NewSystem(
			sys.WithFS(tfs), sys.WithMounter(mounter),
			sys.WithLogger(log.New(log.WithBuffer(buffer))),
			sys.WithRunner(runner),
		)
		Expect(err).NotTo(HaveOccurred())
		cliCmd = &cli.Command{
			Metadata: map[string]any{
				"system": s,
			},
		}
		// Reset requires a live media mounted
		Expect(mounter.Mount("/dev/device", installer.LiveMountPoint, "auto", []string{})).To(Succeed())
	})

	AfterEach(func() {
		cleanup()
	})
	It("fails if no sys.System instance is in metadata", func() {
		cliCmd.Metadata["system"] = nil
		Expect(action.Reset(context.Background(), cliCmd)).NotTo(Succeed())
	})
	It("fails to start installing if the configuration file can't be read", func() {
		cmd.InstallArgs.Description = "doesntexist"
		Expect(action.Reset(context.Background(), cliCmd)).To(MatchError(ContainSubstring("ReadFile doesntexist")))
	})
	It("fails if a live media is not detected", func() {
		tfs.RemoveAll(installer.SquashfsPath)
		Expect(action.Reset(context.Background(), cliCmd)).To(MatchError(ContainSubstring("requires booting from recovery system")))
	})
	It("recovery partition not found", func() {
		Expect(action.Reset(context.Background(), cliCmd)).To(MatchError(ContainSubstring("live mount point not found")))
	})
	It("fails if the setup is inconsistent", func() {
		// Ensure recovery partition is found
		runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			if cmd == "lsblk" {
				return []byte(lsblkJson), runner.ReturnError
			}
			return []byte{}, runner.ReturnError
		}
		Expect(action.Reset(context.Background(), cliCmd)).To(MatchError(ContainSubstring("no system partition found in deployment")))
	})
})
