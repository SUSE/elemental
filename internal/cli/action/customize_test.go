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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/cli/action"
	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/urfave/cli/v2"
)

var _ = Describe("Customize action", Label("customize"), func() {
	var s *sys.System
	var tfs vfs.FS
	var cleanup func()
	var err error
	var ctx *cli.Context

	BeforeEach(func() {
		cmd.UpgradeArgs = cmd.UpgradeFlags{}
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithFS(tfs),
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

	It("fails on invalid output flag values", func() {
		cmd.CustomizeArgs.CustomizeOutput = "/targetdir"
		cmd.CustomizeArgs.OutputName = "./path"
		Expect(action.Customize(ctx)).To(MatchError(ContainSubstring("invalid output filename")))

		cmd.CustomizeArgs.OutputName = "path/"
		Expect(action.Customize(ctx)).To(MatchError(ContainSubstring("invalid output filename")))

		cmd.CustomizeArgs.OutputName = "/path"
		Expect(action.Customize(ctx)).To(MatchError(ContainSubstring("invalid output filename")))

		cmd.CustomizeArgs.OutputName = "path/filename"
		Expect(action.Customize(ctx)).To(MatchError(ContainSubstring("invalid output filename")))
	})
})
