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

package build

import (
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/deployment"
)

func (b *Builder) generatePreparePartition(d *image.Definition) *deployment.Partition {
	const (
		PrepareLabel = "ELEMENTAL-PREPARE"
		PrepareMnt   = "/run/elemental/prepare"
		PrepareSize  = deployment.MiB(2048)
	)

	if d.Network.ConfigDir == "" && d.Network.CustomScript == "" {
		b.System.Logger().Info("No dependency configurations requiring %s partition generation, skipping.", PrepareLabel)
		return nil
	}

	return &deployment.Partition{
		Label:      PrepareLabel,
		Role:       deployment.Data,
		MountPoint: PrepareMnt,
		FileSystem: deployment.Btrfs,
		Size:       PrepareSize,
		Hidden:     true,
	}
}
