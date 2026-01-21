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
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	badConfig = `
disks:
- partitions:
  # no system partition
  - label: mylabel
    function: data
    fileSystem: xfs
  - function: efi
    fileSystem: vfat
  - function: recovery
    fileSystem: btrfs
  - function: data
    fileSystem: ext2
`
)

var _ = Describe("Install action", Label("install"), func() {
	var s *sys.System
	var tfs vfs.FS
	var cleanup func()
	var err error
	var cliCmd *cli.Command
	var buffer *bytes.Buffer

	BeforeEach(func() {
		cmd.InstallArgs = cmd.InstallFlags{}
		buffer = &bytes.Buffer{}
		tfs, cleanup, err = sysmock.TestFS(map[string]string{
			"/configDir/bad_config.yaml": badConfig,
			"/dev/device":                "device",
		})
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithFS(tfs),
			sys.WithLogger(log.New(log.WithBuffer(buffer))),
		)
		Expect(err).NotTo(HaveOccurred())
		cliCmd = &cli.Command{
			Metadata: map[string]any{
				"system": s,
			},
		}
	})

	AfterEach(func() {
		cleanup()
	})
	It("fails if no sys.System instance is in metadata", func() {
		cliCmd.Metadata["system"] = nil
		Expect(action.Install(context.Background(), cliCmd)).NotTo(Succeed())
	})
	It("fails to start installing if the configuration file can't be read", func() {
		cmd.InstallArgs.Target = "/dev/device"
		cmd.InstallArgs.OperatingSystemImage = "my.registry.org/my/image:test"
		cmd.InstallArgs.Description = "doesntexist"
		err = action.Install(context.Background(), cliCmd)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ReadFile doesntexist"))
	})
	It("fails if the setup is inconsistent", func() {
		cmd.InstallArgs.Target = "/dev/device"
		cmd.InstallArgs.OperatingSystemImage = "my.registry.org/my/image:test"
		cmd.InstallArgs.Description = "/configDir/bad_config.yaml"
		err = action.Install(context.Background(), cliCmd)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("inconsistent deployment"))
	})
	It("fails if the given OS uri is not valid", func() {
		cmd.InstallArgs.Target = "/dev/device"
		cmd.InstallArgs.OperatingSystemImage = "https://example.com/my/image"
		err = action.Install(context.Background(), cliCmd)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("image source type not supported"))
	})
})
