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

package action_test

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/urfave/cli/v2"

	"github.com/suse/elemental/v3/internal/cli/action"
	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
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
	var tfs sys.FS
	var cleanup func()
	var err error
	var ctx *cli.Context
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
		ctx = cli.NewContext(cli.NewApp(), nil, &cli.Context{})
		if ctx.App.Metadata == nil {
			ctx.App.Metadata = map[string]any{}
		}
		ctx.App.Metadata["system"] = s
	})

	AfterEach(func() {
		cleanup()
	})
	It("fails if no sys.System instance is in metadata", func() {
		ctx.App.Metadata["system"] = nil
		Expect(action.Install(ctx)).NotTo(Succeed())
	})
	It("fails to start installing if the configuration file can't be read", func() {
		cmd.InstallArgs.Target = "/dev/device"
		cmd.InstallArgs.OperatingSystemImage = "my.registry.org/my/image:test"
		cmd.InstallArgs.ConfigFile = "doesntexist"
		err = action.Install(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("config file 'doesntexist' not found"))
	})
	It("fails to start installing if the target device does not exist", func() {
		cmd.InstallArgs.Target = "/dev/doesntexist"
		cmd.InstallArgs.OperatingSystemImage = "my.registry.org/my/image:test"
		err = action.Install(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("inconsistent deployment"))
	})
	It("fails if the setup is inconsistent", func() {
		cmd.InstallArgs.Target = "/dev/device"
		cmd.InstallArgs.OperatingSystemImage = "my.registry.org/my/image:test"
		cmd.InstallArgs.ConfigFile = "/configDir/bad_config.yaml"
		err = action.Install(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("inconsistent deployment"))
	})
	It("fails if the given OS uri is not valid", func() {
		cmd.InstallArgs.Target = "/dev/device"
		cmd.InstallArgs.OperatingSystemImage = "https://example.com/my/image"
		err = action.Install(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("image source type not supported"))
	})
})
