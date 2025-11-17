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
	"strings"

	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/block/lsblk"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/btrfs"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/installer"
	"github.com/suse/elemental/v3/pkg/repart"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
	"github.com/suse/elemental/v3/pkg/upgrade"
)

type Option func(*Installer)

type Installer struct {
	s          *sys.System
	ctx        context.Context
	u          upgrade.Interface
	unpackOpts []unpack.Opt
	b          bootloader.Bootloader
}

func WithUnpackOpts(opts ...unpack.Opt) Option {
	return func(i *Installer) {
		i.unpackOpts = opts
	}
}

func WithUpgrader(u upgrade.Interface) Option {
	return func(i *Installer) {
		i.u = u
	}
}

func WithBootloader(b bootloader.Bootloader) Option {
	return func(i *Installer) {
		i.b = b
	}
}

func New(ctx context.Context, s *sys.System, opts ...Option) *Installer {
	installer := &Installer{
		s:   s,
		ctx: ctx,
	}
	for _, o := range opts {
		o(installer)
	}
	if installer.u == nil {
		installer.u = upgrade.New(ctx, s, upgrade.WithUnpackOpts(installer.unpackOpts...))
	}
	if installer.b == nil {
		installer.b = bootloader.NewNone(s)
	}
	return installer
}

// IsLiveMedia returns true if the current host is a live media
func IsLiveMedia(s *sys.System) bool {
	mnt, err := s.Mounter().IsMountPoint(installer.LiveMountPoint)
	if !mnt || err != nil {
		return false
	}
	exists, _ := vfs.Exists(s.FS(), installer.SquashfsPath)
	return exists
}

// IsRecovery returns true if the current host is booted from an elemental recovery system
func IsRecovery(s *sys.System) bool {
	if IsLiveMedia(s) {
		cmdline, err := s.FS().ReadFile("/proc/cmdline")
		if err != nil {
			return false
		}
		if strings.Contains(string(cmdline), deployment.RecoveryMark) {
			return true
		}
	}
	return false
}

func (i Installer) Install(d *deployment.Deployment) (err error) {
	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	err = i.checkTargetDisks(d)
	if err != nil {
		return err
	}

	for _, disk := range d.Disks {
		err = repart.PartitionAndFormatDevice(i.s, disk)
		if err != nil {
			return fmt.Errorf("partitioning disk '%s': %w", disk.Device, err)
		}
		for _, part := range disk.Partitions {
			i.s.Logger().Debug("creating partition volumes: %+v", part.RWVolumes)
			err = createPartitionVolumes(i.s, cleanup, part)
			if err != nil {
				return fmt.Errorf("creating partition volumes: %w", err)
			}
		}
	}

	err = i.installRecoveryPartition(cleanup, d)
	if err != nil {
		return fmt.Errorf("installing recovery system: %w", err)
	}

	err = i.u.Upgrade(d)
	if err != nil {
		return fmt.Errorf("executing transaction: %w", err)
	}

	return nil
}

func (i Installer) Reset(d *deployment.Deployment) (err error) {
	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	for _, disk := range d.Disks {
		err = repart.ReconcileDevicePartitions(i.s, disk)
		if err != nil {
			return fmt.Errorf("partitioning disk '%s': %w", disk.Device, err)
		}
		for _, part := range disk.Partitions {
			i.s.Logger().Debug("creating partition volumes: %+v", part.RWVolumes)
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

func (i Installer) checkTargetDisks(d *deployment.Deployment) error {
	bDev := lsblk.NewLsDevice(i.s)
	for _, disk := range d.Disks {
		parts, err := bDev.GetDevicePartitions(disk.Device)
		if err != nil {
			return fmt.Errorf("failed to list target device partitions: %w", err)
		}
		for _, part := range parts {
			if part != nil && len(part.MountPoints) > 0 {
				return fmt.Errorf("cannot install, target device (%s) has active mountpoints: %v", disk.Device, part.MountPoints)
			}
		}
	}
	return nil
}

func (i Installer) installRecoveryPartition(cleanup *cleanstack.CleanStack, d *deployment.Deployment) (err error) {
	recPart := d.GetRecoveryPartition()
	if recPart == nil {
		i.s.Logger().Info("No recovery system defined, skipping recovery system installation")
		return nil
	}

	i.s.Logger().Info("Installing recovery system")
	// This is only required if the SourceOS is a remote OCI image we need to extract
	workDir, err := vfs.TempDir(i.s.FS(), "", "elemental_workdir")
	if err != nil {
		return fmt.Errorf("failed creating a temporary directory to extract the OS image: %w", err)
	}
	cleanup.Push(func() error { return i.s.FS().RemoveAll(workDir) })

	mountPoint, err := vfs.TempDir(i.s.FS(), "", "elemental_"+recPart.Role.String())
	if err != nil {
		return fmt.Errorf("creating temporary directory to mount system partition: %w", err)
	}
	cleanup.PushSuccessOnly(func() error { return i.s.FS().RemoveAll(mountPoint) })

	bPart, err := block.GetPartitionByUUID(i.s, lsblk.NewLsDevice(i.s), recPart.UUID, 4)
	if err != nil {
		return fmt.Errorf("finding partition '%s': %w", recPart.UUID, err)
	}
	err = i.s.Mounter().Mount(bPart.Path, mountPoint, "", []string{"rw"})
	if err != nil {
		return fmt.Errorf("mounting partition '%s': %w", bPart.Path, err)
	}
	cleanup.Push(func() error { return i.s.Mounter().Unmount(mountPoint) })

	media := installer.NewMedia(i.ctx, i.s, installer.Disk, installer.WithUnpackOpts(i.unpackOpts...))
	err = media.PrepareInstallerFS(mountPoint, workDir, d)
	if err != nil {
		return fmt.Errorf("failed preparing recovery partition root: %w", err)
	}
	d.SourceOS = deployment.NewRawSrc(filepath.Join(mountPoint, installer.SquashfsRelPath))
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

		if part.FileSystem == deployment.Btrfs {
			err = btrfs.SetBtrfsPartition(s, mountPoint)
			if err != nil {
				return fmt.Errorf("setting btrfs partition volumes: %w", err)
			}
		}
	}

	if part.FileSystem == deployment.Btrfs {
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
	}

	return nil
}
