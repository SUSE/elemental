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

package action

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	cmdpkg "github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/internal/config"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/install"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/crypto"
	"github.com/suse/elemental/v3/pkg/sys"
)

func Init(_ context.Context, cmd *cli.Command) error {
	if cmd.Root().Metadata == nil || cmd.Root().Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}

	system := cmd.Root().Metadata["system"].(*sys.System)
	logger := system.Logger()
	args := &cmdpkg.InitArgs

	logger.Info("Creating new configuration in %s", args.TargetDir)

	conf := defaultConfiguration()
	if err := config.Write(system.FS(), args.TargetDir, conf); err != nil {
		logger.Error("Failed to write configuration to %s", args.TargetDir)
		return err
	}

	logger.Info("Configuration created successfully")
	return nil
}

func defaultConfiguration() *image.Configuration {
	return &image.Configuration{
		Installation: install.Installation{
			SchemaVersion: "v0",
			Bootloader:    "grub",
			KernelCmdLine: "console=ttyS0 quiet loglevel=3",
			RAW: install.RAW{
				DiskSize: "20G",
			},
			ISO: install.ISO{
				Device: "/dev/sda",
			},
			CryptoPolicy: crypto.DefaultPolicy,
		},
		Release: release.Release{
			Name:        "my-product",
			ManifestURI: "oci://registry.example.com/my-product/release-manifest:latest",
		},
		Kubernetes: kubernetes.Kubernetes{
			Nodes: kubernetes.Nodes{
				{
					Hostname: "node1.example",
					Type:     kubernetes.NodeTypeServer,
					Init:     true,
				},
				{
					Hostname: "node2.example",
					Type:     kubernetes.NodeTypeAgent,
				},
			},
			Network: kubernetes.Network{
				APIHost: "192.168.122.100.sslip.io",
				APIVIP4: "192.168.122.100",
			},
		},
		ButaneConfig: defaultButaneConfig(),
	}
}

func defaultButaneConfig() map[string]any {
	// #nosec G101 -- this is a well-known default password hash for bootstrapping, not a real credential
	const defaultPasswordHash = "$6$dkiCjuXvS8brdFUA$w1b4wSV.0wQ7BmZ7l/Be6fhqlk8CMEE8NQkhtaXIPjMTFw90JNYfI1lBhSoUILhmqupcmOp681FHIdvIZdbc90"

	return map[string]any{
		"version": "1.6.0",
		"variant": "fcos",
		"passwd": map[string]any{
			"users": []map[string]any{
				{
					"name":          "root",
					"password_hash": defaultPasswordHash,
				},
			},
		},
	}
}
