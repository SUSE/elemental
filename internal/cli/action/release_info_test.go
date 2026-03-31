package action_test

import (
	"bytes"
	"context"

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
  name: suse-core-test
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
			Writer: buffer,
		}
		ctx = context.Background()
		cmd.ReleaseInfoArgs.Local = true
		cliCmd.Action = action.ReleaseInfo
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

	It("tests various options of release-info command", func() {
		manifestPath, err := tfs.RawPath("/etc/elemental3/manifest.yaml")
		Expect(err).ToNot(HaveOccurred())
		manifestPath = "file://" + manifestPath

		err = cliCmd.Run(ctx, []string{"", manifestPath})
		Expect(err).ToNot(HaveOccurred())
		Expect(buffer).To(ContainSubstring("CORE PLATFORM"))
		Expect(buffer).To(ContainSubstring("suse-core-test"))
		Expect(buffer).To(ContainSubstring("registry.suse.com/beta/uc/uc-base-os-kernel-default:16.0-55.79"))

		Expect(buffer).To(ContainSubstring("INFRASTRUCTURE COMPONENTS"))
		Expect(buffer).To(ContainSubstring("SLES 16.0"))
		Expect(buffer).To(ContainSubstring("registry.suse.com/beta/uc/rke2:1.35_1.42-1.77"))

		Expect(buffer).To(ContainSubstring("SYSTEMD EXTENSIONS"))
		Expect(buffer).To(ContainSubstring("registry.suse.com/beta/uc/elemental3ctl:0.6_19.2-3.151"))

		Expect(buffer).To(ContainSubstring("CHART NAME"))
		Expect(buffer).To(ContainSubstring("https://metallb.github.io/metallb"))
		Expect(buffer).To(ContainSubstring("https://suse-edge.github.io/charts"))
	})
})
