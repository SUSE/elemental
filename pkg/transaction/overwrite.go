/*
Copyright © 2022-2026 SUSE LLC
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

package transaction

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/fstab"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

// Overwrite transaction snapshotter is a passthrough snapshotter built to be able to verify that the transaction code works as advertised and verify the transaction interface. Should only be used for debugging.
type Overwrite struct {
	s          *sys.System
	ctx        context.Context
	d          *deployment.Deployment
	cleanStack *cleanstack.CleanStack
	lsBlk      block.Device
}

func NewOverwrite(ctx context.Context, s *sys.System, d *deployment.Deployment, lsBlk block.Device) Interface {
	return Overwrite{s, ctx, d, cleanstack.NewCleanStack(), lsBlk}
}

var _ Interface = (*Overwrite)(nil)
var _ UpgradeHelper = (*Overwrite)(nil)

func (n Overwrite) Commit(trans *Transaction, cleanup func() error) (err error) {
	trans.status = committed
	if cleanup != nil {
		n.cleanStack.Push(cleanup)
	}
	return n.cleanStack.Cleanup(err)
}

func (n Overwrite) Init(deployment.Deployment) (UpgradeHelper, error) {
	return n, nil
}

func (n Overwrite) Start() (*Transaction, error) {
	temp, err := vfs.TempDir(n.s.FS(), "", "overwrite")
	if err != nil {
		return nil, fmt.Errorf("failed creating temp-dir: %w", err)
	}

	hwParts, err := n.lsBlk.GetAllPartitions()
	if err != nil {
		return nil, fmt.Errorf("failed listing partitions")
	}

	sysDisk := n.d.GetSystemDisk()
	if sysDisk == nil {
		return nil, fmt.Errorf("no system disk found in deployment")
	}

	sysPart := n.d.GetSystemPartition()
	if sysPart == nil {
		return nil, fmt.Errorf("no system partition found in deployment")
	}

	err = n.MountPartition(temp, hwParts, sysPart)
	if err != nil {
		return nil, fmt.Errorf("failed mounting partition '%s': %w", sysPart.Label, err)
	}

	for _, p := range sysDisk.Partitions {
		if p.Role == deployment.System {
			continue
		}

		err = n.MountPartition(temp, hwParts, p)
		if err != nil {
			return nil, fmt.Errorf("failed mounting partition '%s': %w", p.Label, err)
		}
	}

	return &Transaction{ID: 0, Path: temp, status: started}, nil
}

func (n Overwrite) MountPartition(root string, hwParts block.PartitionList, p *deployment.Partition) error {
	dev := hwParts.GetByUUIDNameOrLabel(p.UUID, p.Role.String(), p.Label)
	if dev == nil {
		return fmt.Errorf("partition not found: %+v", p)
	}

	target := filepath.Join(root, p.MountPoint)

	n.s.Logger().Debug("Mounting partition with label '%s' to '%s'", p.Label, target)

	if err := vfs.MkdirAll(n.s.FS(), target, vfs.DirPerm); err != nil {
		return fmt.Errorf("failed creating mountpoint %s: %w", target, err)
	}

	err := n.s.Mounter().Mount(dev.Path, target, p.FileSystem.String(), p.MountOpts)
	if err != nil {
		return fmt.Errorf("failed mounting partition '%s': %w", p.Label, err)
	}

	n.cleanStack.Push(func() error {
		n.s.Logger().Debug("Unmounting '%s'", target)
		return n.s.Mounter().Unmount(target)
	})

	return nil
}

func (n Overwrite) Rollback(*Transaction, error) error {
	return fmt.Errorf("cannot rollback transactions using 'overwrite' snapshotter")
}

func (n Overwrite) GetActiveSnapshotIDs() ([]int, error) {
	return []int{0}, nil
}

func (n Overwrite) SyncImageContent(imgSrc *deployment.ImageSource, trans *Transaction, opts ...unpack.Opt) (err error) {
	if trans.status != started {
		return fmt.Errorf("given transaction '%d' is not started", trans.ID)
	}
	var unpacker unpack.Interface

	n.s.Logger().Info("Unpacking image source: %s to %s", imgSrc.String(), trans.Path)
	unpacker, err = unpack.NewUnpacker(n.s, imgSrc, opts...)
	if err != nil {
		return fmt.Errorf("initializing unpacker: %w", err)
	}
	digest, err := unpacker.SynchedUnpack(n.ctx, trans.Path, nil, nil)
	if err != nil {
		return fmt.Errorf("unpacking image to '%s': %w", trans.Path, err)
	}
	imgSrc.SetDigest(digest)

	return nil
}

func (n Overwrite) Merge(*Transaction) error {
	return nil
}

func (n Overwrite) UpdateFstab(trans *Transaction) error {
	lines := []fstab.Line{}
	sysDisk := n.d.GetSystemDisk()
	if sysDisk == nil {
		return fmt.Errorf("no system disk found in deployment")
	}

	for _, part := range sysDisk.Partitions {
		lines = append(lines, fstab.Line{
			Device:     fmt.Sprintf("PARTUUID=%s", part.UUID),
			MountPoint: part.MountPoint,
			Options:    part.MountOpts,
			FileSystem: part.FileSystem.String(),
		})

	}
	fstabFile := filepath.Join(trans.Path, fstab.File)
	return fstab.Write(n.s, fstabFile, lines)
}

func (n Overwrite) Lock(*Transaction) error {
	return nil
}

func (n Overwrite) GenerateKernelCmdline(*Transaction) string {
	sysPart := n.d.GetSystemPartition()
	if sysPart == nil {
		return ""
	}

	return fmt.Sprintf("rootfstype=%s", sysPart.FileSystem.String())
}
