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

package install

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/block/lsblk"
	"github.com/suse/elemental/v3/pkg/btrfs"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/diskrepart"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/upgrade"
)

type Option func(*Installer)

type Installer struct {
	s *sys.System
	u upgrade.Interface
}

func WithUpgrader(u upgrade.Interface) Option {
	return func(i *Installer) {
		i.u = u
	}
}

func New(ctx context.Context, s *sys.System, opts ...Option) *Installer {
	installer := &Installer{
		s: s,
	}
	for _, o := range opts {
		o(installer)
	}
	if installer.u == nil {
		installer.u = upgrade.New(ctx, s)
	}
	return installer
}

func (i Installer) Install(d *deployment.Deployment) (err error) {
	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	for _, disk := range d.Disks {
		err = diskrepart.PartitionAndFormatDevice(i.s, disk)
		if err != nil {
			return fmt.Errorf("partitioning disk '%s': %w", disk.Device, err)
		}
		for _, part := range disk.Partitions {
			err = createPartitionVolumes(i.s, cleanup, part)
			if err != nil {
				return fmt.Errorf("creating partition volumes: %w", err)
			}
		}
	}

	err = i.u.Upgrade(d)
	if err != nil {
		return fmt.Errorf("executing transaction: %w", err)
	}

	return nil
}

func createPartitionVolumes(s *sys.System, cleanStack *cleanstack.CleanStack, part *deployment.Partition) (err error) {
	var mountPoint string

	if len(part.RWVolumes) > 0 || part.Role == deployment.System {
		mountPoint, err = vfs.TempDir(s.FS(), "", "elemental_"+part.Role.String())
		if err != nil {
			return fmt.Errorf("creating temporary directory to mount system partition: %w", err)
		}
		cleanStack.PushSuccessOnly(func() error { return s.FS().RemoveAll(mountPoint) })

		bDev := lsblk.NewLsDevice(s)
		bPart, err := block.GetPartitionByUUID(s, bDev, part.UUID, 4)
		if err != nil {
			return fmt.Errorf("finding partition '%s': %w", part.UUID, err)
		}
		err = s.Mounter().Mount(bPart.Path, mountPoint, "", []string{})
		if err != nil {
			return fmt.Errorf("mounting partition '%s': %w", bPart.Path, err)
		}
		cleanStack.Push(func() error { return s.Mounter().Unmount(mountPoint) })

		err = btrfs.SetBtrfsPartition(s, mountPoint)
		if err != nil {
			return fmt.Errorf("setting btrfs partition volumes: %w", err)
		}
	}

	for _, rwVol := range part.RWVolumes {
		if rwVol.Snapshotted {
			continue
		}
		subvolume := filepath.Join(mountPoint, btrfs.TopSubVol, rwVol.Path)
		err = btrfs.CreateSubvolume(s, subvolume, true)
		if err != nil {
			return fmt.Errorf("creating subvolume '%s': %w", subvolume, err)
		}
	}

	return nil
}
