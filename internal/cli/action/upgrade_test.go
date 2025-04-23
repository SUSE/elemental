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
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Upgrade action", Label("upgrade"), func() {
	var s *sys.System
	var tfs vfs.FS
	var cleanup func()
	var err error
	var ctx *cli.Context
	var buffer *bytes.Buffer

	BeforeEach(func() {
		cmd.UpgradeArgs = cmd.UpgradeFlags{}
		buffer = &bytes.Buffer{}
		tfs, cleanup, err = sysmock.TestFS(map[string]string{
			"/etc/elemental/deployment.yaml": badConfig,
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
		Expect(action.Upgrade(ctx)).NotTo(Succeed())
	})
	It("fails to start the upgrade if the deployment file does not exist", func() {
		Expect(vfs.RemoveAll(tfs, "/etc/elemental")).To(Succeed())
		cmd.UpgradeArgs.OperatingSystemImage = "my.registry.org/my/image:test"
		err = action.Upgrade(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("could not read deployment file"))
	})
	It("fails if the setup is inconsistent", func() {
		cmd.UpgradeArgs.OperatingSystemImage = "my.registry.org/my/image:test"
		err = action.Upgrade(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("inconsistent deployment"))
	})
	It("fails if the given OS uri is not valid", func() {
		cmd.UpgradeArgs.OperatingSystemImage = "https://example.com/my/image"
		err = action.Upgrade(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("image source type not supported"))
	})
})
