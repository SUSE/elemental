/*
Copyright © 2026 SUSE LLC
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
	"path/filepath"

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

var expectedReleaseSubstring = `components:
  helm:
    - chart: metallb
    - chart: endpoint-copier-operator
      credentials:
        username: release-user
        password: release-pass`
var expectedClusterSubstring = `helm:
  charts:
    - name: example-chart
      repositoryName: example-chart-collection
      version: "1.0"
      targetNamespace: exampleNamespace
      valuesFile: ""
    - name: example-auth-chart
      repositoryName: example-auth-chart-collection
      version: "2.0"
      targetNamespace: exampleNamespace
      valuesFile: ""
  repositories:
    - name: example-chart-collection
      url: https://example-charts.io
    - name: example-auth-chart-collection
      url: https://example-auth-charts.io
      credentials:
        username: example-user
        password: example-pass`

var _ = Describe("Init action", Label("init"), func() {
	var s *sys.System
	var tfs vfs.FS
	var cleanup func()
	var err error
	var cliCmd *cli.Command
	var buffer *bytes.Buffer
	const targetDir = "/tmp/init-test"

	BeforeEach(func() {
		cmd.InitArgs = cmd.InitFlags{}
		buffer = &bytes.Buffer{}
		tfs, cleanup, err = sysmock.TestFS(map[string]any{})
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
		cmd.InitArgs.TargetDir = targetDir
	})

	AfterEach(func() {
		cleanup()
	})

	It("creates all expected files and directories", func() {
		Expect(action.Init(context.Background(), cliCmd)).To(Succeed())

		exists, _ := vfs.Exists(tfs, filepath.Join(targetDir, "install.yaml"))
		Expect(exists).To(BeTrue())

		exists, _ = vfs.Exists(tfs, filepath.Join(targetDir, "release.yaml"))
		Expect(exists).To(BeTrue())

		exists, _ = vfs.Exists(tfs, filepath.Join(targetDir, "butane.yaml"))
		Expect(exists).To(BeTrue())

		info, err := tfs.Stat(filepath.Join(targetDir, "network"))
		Expect(err).ToNot(HaveOccurred())
		Expect(info.IsDir()).To(BeTrue())

		exists, _ = vfs.Exists(tfs, filepath.Join(targetDir, "kubernetes", "cluster.yaml"))
		Expect(exists).To(BeTrue())
	})

	It("writes valid install.yaml with schema version", func() {
		Expect(action.Init(context.Background(), cliCmd)).To(Succeed())

		data, err := tfs.ReadFile(filepath.Join(targetDir, "install.yaml"))
		Expect(err).ToNot(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("schema: v0"))
		Expect(string(data)).To(ContainSubstring("bootloader: grub"))
	})

	It("writes valid cluster.yaml with auth and non-auth helm repositories", func() {
		Expect(action.Init(context.Background(), cliCmd)).To(Succeed())

		data, err := tfs.ReadFile(filepath.Join(targetDir, "kubernetes", "cluster.yaml"))
		Expect(err).ToNot(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(expectedClusterSubstring))
	})

	It("writes valid release.yaml with manifest URI", func() {
		Expect(action.Init(context.Background(), cliCmd)).To(Succeed())

		data, err := tfs.ReadFile(filepath.Join(targetDir, "release.yaml"))
		Expect(err).ToNot(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("manifestURI:"))
		Expect(string(data)).To(ContainSubstring(expectedReleaseSubstring))
	})

	It("writes butane.yaml with root user", func() {
		Expect(action.Init(context.Background(), cliCmd)).To(Succeed())

		data, err := tfs.ReadFile(filepath.Join(targetDir, "butane.yaml"))
		Expect(err).ToNot(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("variant: fcos"))
		Expect(string(data)).To(ContainSubstring("name: root"))
	})

	It("logs success message", func() {
		Expect(action.Init(context.Background(), cliCmd)).To(Succeed())
		Expect(buffer.String()).To(ContainSubstring("Configuration created successfully"))
	})

	It("fails if configuration already exists", func() {
		Expect(vfs.MkdirAll(tfs, targetDir, 0755)).To(Succeed())
		Expect(tfs.WriteFile(filepath.Join(targetDir, "install.yaml"), []byte("schema: v0"), 0644)).To(Succeed())

		err := action.Init(context.Background(), cliCmd)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("configuration already exists"))
	})
})
