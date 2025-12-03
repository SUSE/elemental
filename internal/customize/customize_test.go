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

package customize_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/internal/config"
	"github.com/suse/elemental/v3/internal/customize"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/install"
	"github.com/suse/elemental/v3/pkg/crypto"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestCustomizeSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Customize test suite")
}

var _ = Describe("Customize runner", Label("customize"), func() {
	const (
		outputDir      config.OutputDir = "/_out"
		installerDescr string           = `
disks:
  - partitions:
    - label: EFI
      fileSystem: vfat
      size: 1024
      role: efi
      mountPoint: /boot
      mountOpts:
        - defaults
        - x-systemd.automount
    - label: RECOVERY
      fileSystem: btrfs
      size: 1280
      role: recovery
      hidden: true
    - label: SYSTEM
      fileSystem: btrfs
      role: system
      mountPoint: /
      mountOpts:
        - ro=vfs
      rwVolumes:
        - path: /var
          noCopyOnWrite: true
          mountOpts:
            - x-initrd.mount
        - path: /root
          mountOpts:
            - x-initrd.mount
        - path: /etc
          snapshotted: true
          mountOpts:
            - x-initrd.mount
        - path: /opt
        - path: /srv
        - path: /home`
		expectedISO = "https://registry.foo.bar/uc-base-kernel-default-iso:0.0.1"
	)

	var fs vfs.FS
	var cleanup func()
	var customizeRunner *customize.Runner
	var sysRunner *sysmock.Runner

	var sideEffects map[string]func(...string) ([]byte, error)
	BeforeEach(func() {
		var err error
		sysRunner = sysmock.NewRunner()

		fs, cleanup, err = sysmock.TestFS(map[string]any{})
		Expect(err).ToNot(HaveOccurred())
		s, err := sys.NewSystem(
			sys.WithRunner(sysRunner),
			sys.WithFS(fs),
		)
		Expect(err).NotTo(HaveOccurred())

		sideEffects = map[string]func(...string) ([]byte, error){}
		sysRunner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			if f := sideEffects[cmd]; f != nil {
				return f(args...)
			}
			return sysRunner.ReturnValue, sysRunner.ReturnError
		}
		Expect(err).ToNot(HaveOccurred())
		Expect(vfs.MkdirAll(fs, string(outputDir), vfs.DirPerm)).To(Succeed())

		customizeRunner = &customize.Runner{
			System: s,
			ConfigManager: &configManagerMock{
				configFucn: func(ctx context.Context, conf *image.Configuration, output config.OutputDir) (*resolver.ResolvedManifest, error) {
					return &resolver.ResolvedManifest{
						CorePlatform: &core.ReleaseManifest{
							Components: core.Components{
								OperatingSystem: &core.OperatingSystem{
									Image: core.Image{
										ISO: expectedISO,
									},
								},
							},
						},
					}, nil
				},
			},
			FileExtractor: &fileExtractorMock{
				extractFunc: func(uri string, local bool) (path string, err error) {
					return "", nil
				},
			},
			Media: &mediaMock{
				customizeFunc: func(d *deployment.Deployment) error {
					return nil
				},
			},
		}
		sideEffects["xorriso"] = func(args ...string) ([]byte, error) {
			file := filepath.Join(string(outputDir), "iso-desc-install", "install.yaml")
			Expect(fs.WriteFile(file, []byte(installerDescr), vfs.FilePerm)).To(Succeed())
			return []byte{}, nil
		}
	})

	AfterEach(func() {
		cleanup()
	})

	It("passes deployment object for ISO media with additional partitions", func() {
		customizeRunner.FileExtractor = &fileExtractorMock{
			extractFunc: func(uri string, local bool) (path string, err error) {
				Expect(uri).To(Equal(expectedISO))
				return "", nil
			},
		}

		customizeDeployment := &deployment.Deployment{}
		customizeRunner.Media = &mediaMock{
			customizeFunc: func(d *deployment.Deployment) error {
				customizeDeployment = d
				return nil
			},
		}
		def := &image.Definition{
			Image: image.Image{
				ImageType: "iso",
			},
			Configuration: &image.Configuration{
				Installation: install.Installation{
					Bootloader:    "grub",
					KernelCmdLine: "console=ttyS0",
					CryptoPolicy:  crypto.FIPSPolicy,
					ISO: install.ISO{
						Device: "/dev/sda",
					},
				},
			},
		}

		// Simulate first boot configuration
		Expect(vfs.MkdirAll(fs, outputDir.FirstbootConfigDir(), vfs.DirPerm)).To(Succeed())

		err := customizeRunner.Run(context.Background(), def, outputDir, true)
		Expect(err).ToNot(HaveOccurred())
		defaultCustomizeDeploymentValidation(customizeDeployment)

		Expect(customizeDeployment.Disks[0].Device).To(Equal("/dev/sda"))
		// [{}, {}, nil, ignition, SYSTEM]
		Expect(len(customizeDeployment.Disks[0].Partitions)).To(Equal(5))
		Expect(customizeDeployment.Disks[0].Partitions[0]).To(Equal(&deployment.Partition{}))
		Expect(customizeDeployment.Disks[0].Partitions[1]).To(Equal(&deployment.Partition{}))
		Expect(customizeDeployment.Disks[0].Partitions[2]).To(BeNil())
		Expect(customizeDeployment.Disks[0].Partitions[3]).To(Equal(&deployment.Partition{
			Label:      deployment.ConfigLabel,
			MountPoint: deployment.ConfigMnt,
			Role:       deployment.Data,
			FileSystem: deployment.Btrfs,
			Size:       256,
			Hidden:     true,
		}))
		Expect(customizeDeployment.Disks[0].Partitions[4]).To(Equal(&deployment.Partition{
			Label:      deployment.SystemLabel,
			Role:       deployment.System,
			MountPoint: deployment.SystemMnt,
			FileSystem: deployment.Btrfs,
			Size:       deployment.AllAvailableSize,
			MountOpts:  []string{"ro=vfs"},
			RWVolumes: []deployment.RWVolume{
				{Path: "/var", NoCopyOnWrite: true, MountOpts: []string{"x-initrd.mount"}},
				{Path: "/root", MountOpts: []string{"x-initrd.mount"}},
				{Path: "/etc", Snapshotted: true, MountOpts: []string{"x-initrd.mount"}},
				{Path: "/opt"}, {Path: "/srv"}, {Path: "/home"},
			},
		}))

	})

	It("passes deployment object for RAW media without additional partitions", func() {
		customizeRunner.FileExtractor = &fileExtractorMock{
			extractFunc: func(uri string, local bool) (path string, err error) {
				Expect(uri).To(Equal(expectedISO))
				return "", nil
			},
		}

		customizeDeployment := &deployment.Deployment{}
		customizeRunner.Media = &mediaMock{
			customizeFunc: func(d *deployment.Deployment) error {
				customizeDeployment = d
				return nil
			},
		}
		sideEffects["truncate"] = func(args ...string) ([]byte, error) {
			// args = [-s 35G customized.raw]
			Expect(args[1]).To(Equal("35G"))
			Expect(args[2]).To(Equal("customized.raw"))
			return []byte{}, nil
		}

		def := &image.Definition{
			Image: image.Image{
				ImageType:       "raw",
				OutputImageName: "customized.raw",
			},
			Configuration: &image.Configuration{
				Installation: install.Installation{
					Bootloader:    "grub",
					KernelCmdLine: "console=ttyS0",
					CryptoPolicy:  crypto.FIPSPolicy,
					RAW: install.RAW{
						DiskSize: "35G",
					},
				},
			},
		}

		err := customizeRunner.Run(context.Background(), def, outputDir, true)
		Expect(err).ToNot(HaveOccurred())
		defaultCustomizeDeploymentValidation(customizeDeployment)
		Expect(customizeDeployment.Disks[0].Device).To(BeEmpty())
		Expect(len(customizeDeployment.Disks[0].Partitions)).To(Equal(0))
	})

	It("fails to configure components", func() {
		customizeRunner.ConfigManager = &configManagerMock{
			configFucn: func(ctx context.Context, conf *image.Configuration, output config.OutputDir) (*resolver.ResolvedManifest, error) {
				return nil, fmt.Errorf("missing manifest")
			},
		}

		err := customizeRunner.Run(context.Background(), &image.Definition{}, outputDir, true)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("missing manifest"))
	})

	It("fails to extract iso from container", func() {
		customizeRunner.FileExtractor = &fileExtractorMock{
			extractFunc: func(uri string, local bool) (path string, err error) {
				return "", fmt.Errorf("extract error")
			},
		}

		err := customizeRunner.Run(context.Background(), &image.Definition{}, outputDir, true)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("extract error"))
	})

	It("fails to load ISO description", func() {
		sideEffects["xorriso"] = func(args ...string) ([]byte, error) {
			return []byte{}, fmt.Errorf("xorriso command failed")
		}

		customizeRunner.FileExtractor = &fileExtractorMock{
			extractFunc: func(uri string, local bool) (path string, err error) {
				return "missing.iso", nil
			},
		}

		err := customizeRunner.Run(context.Background(), &image.Definition{}, outputDir, true)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("'missing.iso': xorriso command failed"))
	})

	It("fails to parse media type", func() {
		def := &image.Definition{
			Image: image.Image{
				ImageType: "foo",
			},
		}

		err := customizeRunner.Run(context.Background(), def, outputDir, true)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("unsupported media type foo: unsupported operation"))
	})

	It("fails to parse customize deployment", func() {
		def := &image.Definition{
			Image: image.Image{
				ImageType: "iso",
			},
			Configuration: &image.Configuration{
				Installation: install.Installation{},
			},
		}

		err := customizeRunner.Run(context.Background(), def, outputDir, true)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("missing device configuration for ISO image type"))

	})

	It("fails to customize media", func() {
		customizeRunner.Media = &mediaMock{
			customizeFunc: func(d *deployment.Deployment) error {
				return fmt.Errorf("customization error")
			},
		}

		def := &image.Definition{
			Image: image.Image{
				ImageType: "iso",
			},
			Configuration: &image.Configuration{
				Installation: install.Installation{
					ISO: install.ISO{
						Device: "/dev/sda",
					},
				},
			},
		}

		err := customizeRunner.Run(context.Background(), def, outputDir, true)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("customization error"))
	})

	It("fails to resize RAW disk", func() {
		def := &image.Definition{
			Image: image.Image{
				ImageType: "raw",
			},
			Configuration: &image.Configuration{
				Installation: install.Installation{
					RAW: install.RAW{
						DiskSize: "35Invalid",
					},
				},
			},
		}

		err := customizeRunner.Run(context.Background(), def, outputDir, true)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("invalid disk size definition '35Invalid'"))

	})
})

type configManagerMock struct {
	configFucn func(ctx context.Context, conf *image.Configuration, output config.OutputDir) (*resolver.ResolvedManifest, error)
}

func (c *configManagerMock) ConfigureComponents(ctx context.Context, conf *image.Configuration, output config.OutputDir) (*resolver.ResolvedManifest, error) {
	if c.configFucn != nil {
		return c.configFucn(ctx, conf, output)
	}

	panic("not implemented")
}

type fileExtractorMock struct {
	extractFunc func(uri string, local bool) (path string, err error)
}

func (f *fileExtractorMock) ExtractFrom(uri string, local bool) (path string, err error) {
	if f.extractFunc != nil {
		return f.extractFunc(uri, local)
	}

	panic("not implemented")
}

type mediaMock struct {
	customizeFunc func(d *deployment.Deployment) error
}

func (m *mediaMock) Customize(d *deployment.Deployment) error {
	if m.customizeFunc != nil {
		return m.customizeFunc(d)
	}

	panic("not implemented")
}

func defaultCustomizeDeploymentValidation(dep *deployment.Deployment) {
	Expect(dep.BootConfig.Bootloader).To(Equal("grub"))

	expectedCMD := fmt.Sprintf("console=ttyS0 %s %s", "fips=1", fmt.Sprintf("boot=LABEL=%s", deployment.EfiLabel))
	Expect(dep.BootConfig.KernelCmdline).To(Equal(expectedCMD))
	Expect(dep.Security.CryptoPolicy).To(Equal(crypto.FIPSPolicy))

	expectedImgSrc := deployment.NewDirSrc("/_out/overlays")
	Expect(dep.OverlayTree).To(Equal(expectedImgSrc))
	Expect(dep.Installer.CfgScript).To(Equal("/_out/auto_installer.sh"))
}
