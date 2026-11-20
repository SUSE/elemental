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

package cmd_test

import (
	"context"
	"fmt"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/cli/v3"

	"github.com/suse/elemental/v3/internal/cli/app"
	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/sys"
)

var _ = Describe("System platform", Label("platform"), func() {
	var system *sys.System

	// A supported platform other than the host one, so the tests show --platform overriding the host default
	otherPlatform := "linux/arm64"
	if runtime.GOARCH == "arm64" {
		otherPlatform = "linux/amd64"
	}

	captureSystem := func(_ context.Context, c *cli.Command) error {
		system = c.Root().Metadata["system"].(*sys.System)
		return nil
	}

	run := func(command *cli.Command, args ...string) error {
		root := app.New("", cmd.GlobalFlags(), cmd.Setup, cmd.Teardown, command)
		// Return exit errors instead of terminating the test process
		root.ExitErrHandler = func(context.Context, *cli.Command, error) {}
		return root.Run(context.Background(), append([]string{"elemental3"}, args...))
	}

	BeforeEach(func() {
		system = nil
		cmd.CustomizeArgs = cmd.CustomizeFlags{}
		cmd.UnpackArgs = cmd.UnpackFlags{}
		cmd.ReleaseInfoArgs = cmd.ReleaseInfoFlags{}
	})

	It("targets the host platform by default", func() {
		Expect(run(cmd.NewCustomizeCommand("elemental3", captureSystem), "customize")).To(Succeed())
		Expect(system.Platform().String()).To(Equal(fmt.Sprintf("linux/%s", runtime.GOARCH)))
	})

	It("targets the platform given to customize", func() {
		Expect(run(cmd.NewCustomizeCommand("elemental3", captureSystem), "customize", "--platform", otherPlatform)).To(Succeed())
		Expect(system.Platform().String()).To(Equal(otherPlatform))
	})

	It("targets the platform given to unpack-image", func() {
		Expect(run(cmd.NewUnpackImageCommand("elemental3", captureSystem),
			"unpack-image", "--image", "registry.example.com/os:1.0", "--target", "/tmp/os", "--platform", otherPlatform)).To(Succeed())
		Expect(system.Platform().String()).To(Equal(otherPlatform))
	})

	It("targets the platform given to release-info", func() {
		Expect(run(cmd.NewReleaseInfoCommand("elemental3", captureSystem),
			"release-info", "--platform", otherPlatform, "oci://registry.example.com/release-manifest:1.0")).To(Succeed())
		Expect(system.Platform().String()).To(Equal(otherPlatform))
	})

	It("fails on an unsupported platform", func() {
		err := run(cmd.NewCustomizeCommand("elemental3", captureSystem), "customize", "--platform", "linux/s390")
		Expect(err).To(MatchError(ContainSubstring("Unsupported --platform option")))
		Expect(system).To(BeNil())
	})
})
