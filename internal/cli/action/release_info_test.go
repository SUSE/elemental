package action_test

import (
	"bytes"
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/internal/cli/action"
	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/urfave/cli/v3"
)

var _ = Describe("Release info tests", Label("release-info"), func() {
	var s *sys.System
	var tfs vfs.FS
	var cleanup func()
	var err error
	var cliCmd *cli.Command
	var buffer *bytes.Buffer
	var ctx context.Context
	var manifest = `metadata:
  name: suse-core
  version: 0.6-rc.20260317
  creationDate: '2026-03-17'
components:
  operatingSystem:
    image:
      base: registry.suse.com/beta/uc/uc-base-os-kernel-default:16.0-55.79
      iso: registry.suse.com/beta/uc/uc-base-kernel-default-iso:16.0-55.132
  kubernetes:
    version: v1.35.0+rke2r1
    image: registry.suse.com/beta/uc/rke2:1.35_1.42-1.77
  systemd:
    extensions:
    - name: elemental3ctl
      image: registry.suse.com/beta/uc/elemental3ctl:0.6_19.2-3.151
      required: true
  helm:
    charts:
    - name: MetalLB
      chart: metallb
      version: 0.15.2
      namespace: metallb-system
      repository: metallb
    - name: Endpoint Copier Operator
      chart: endpoint-copier-operator
      version: 0.3.0
      namespace: endpoint-copier-operator
      repository: suse-edge
    repositories:
    - name: metallb
      url: https://metallb.github.io/metallb
    - name: suse-edge
      url: https://suse-edge.github.io/charts`

	BeforeEach(func() {
		cmd.ReleaseInfoArgs = cmd.ReleaseInfoFlags{}
		buffer = &bytes.Buffer{}
		tfs, cleanup, err = sysmock.TestFS(map[string]string{
			"/etc/elemental3/manifest.yaml": manifest,
		})
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithFS(tfs),
			sys.WithLogger(log.New(log.WithBuffer(buffer))),
		)
		cliCmd = &cli.Command{
			Metadata: map[string]any{
				"system": s,
			},
		}
		ctx = context.Background()
		cmd.ReleaseInfoArgs.Local = true
		cliCmd.Action = action.ReleaseInfo
		cliCmd.Writer = buffer
	})
	AfterEach(func() {
		cleanup()
	})

	It("fails if no sys.System instance is available", func() {
		cliCmd.Metadata["system"] = nil
		Expect(action.ReleaseInfo(ctx, cliCmd)).ToNot(Succeed())
	})

	It("fails if no argument is passed to it", func() {
		Expect(action.ReleaseInfo(ctx, cliCmd)).ToNot(Succeed())
	})

	It("fails if inexistent file is passed as argument", func() {
		manifestPath, err := tfs.RawPath("/etc/elemental3/nosuchfile.yaml")
		Expect(err).ToNot(HaveOccurred())

		err = cliCmd.Run(ctx, []string{"", manifestPath})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("use the 'oci://' prefix"))
	})

	It("tests various options of release-info command", func() {
		manifestPath, err := tfs.RawPath("/etc/elemental3/manifest.yaml")
		Expect(err).ToNot(HaveOccurred())
		manifestPath = "file://" + manifestPath

		err = cliCmd.Run(ctx, []string{"", manifestPath})
		Expect(err).ToNot(HaveOccurred())
		Expect(buffer).To(ContainSubstring("Core Manifest"))
		Expect(buffer).To(ContainSubstring("Helm Repositories"))
		Expect(buffer).To(ContainSubstring("metallb"))
		Expect(buffer).To(ContainSubstring("suse-edge"))
		Expect(buffer).To(ContainSubstring("Helm Charts"))
	})

	It("tests for JSON output", func() {
		manifestPath, err := tfs.RawPath("/etc/elemental3/manifest.yaml")
		Expect(err).ToNot(HaveOccurred())
		manifestPath = "file://" + manifestPath

		cmd.ReleaseInfoArgs.Output = "json"
		err = cliCmd.Run(ctx, []string{"", manifestPath})
		Expect(err).ToNot(HaveOccurred())
		Expect(json.Valid(buffer.Bytes())).To(BeTrue())
		Expect(buffer).To(ContainSubstring("core"))
		Expect(buffer).To(ContainSubstring("helmRepos"))
		Expect(buffer).To(ContainSubstring("helmCharts"))
		Expect(buffer).To(ContainSubstring("systemdExtensions"))
	})

	It("tests for YAML output", func() {
		manifestPath, err := tfs.RawPath("/etc/elemental3/manifest.yaml")
		Expect(err).ToNot(HaveOccurred())
		manifestPath = "file://" + manifestPath

		cmd.ReleaseInfoArgs.Output = "yaml"
		err = cliCmd.Run(ctx, []string{"", manifestPath})
		Expect(err).ToNot(HaveOccurred())
		Expect(buffer).To(ContainSubstring("core"))
		Expect(buffer).To(ContainSubstring("helmRepos"))
		Expect(buffer).To(ContainSubstring("helmCharts"))
		Expect(buffer).To(ContainSubstring("systemdExtensions"))
	})
})
