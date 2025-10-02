/*
Copyright © 2022-2025 SUSE LLC
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
	"errors"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/block/lsblk"
	"github.com/suse/elemental/v3/pkg/btrfs"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/snapper"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	snapshotPathTmpl = ".snapshots/%d/snapshot"
	updateProgress   = "update-in-progress"
	maxSnapshots     = 8
)

type snapperContext struct {
	ctx          context.Context
	s            *sys.System
	partitions   deployment.Partitions
	cleanStack   *cleanstack.CleanStack
	snap         *snapper.Snapper
	maxSnapshots int
}

// checkCancelled returns the given error if not nil, otherwise it returns the context error if any.
func (sc snapperContext) checkCancelled(err error) error {
	if err != nil {
		return err
	}
	return sc.ctx.Err()
}

type snapperT struct {
	snapperContext
	defaultID    int
	activeID     int
	rootDir      string
	hwPartitions block.PartitionList
}

func NewSnapper(ctx context.Context, s *sys.System) Interface {
	sc := snapperContext{
		ctx:          ctx,
		s:            s,
		cleanStack:   cleanstack.NewCleanStack(),
		snap:         snapper.New(s),
		maxSnapshots: maxSnapshots,
	}
	return &snapperT{
		snapperContext: sc,
	}
}

// Init checks initial snapshotter configuration and sets it if needed
func (sn *snapperT) Init(d deployment.Deployment) (uh UpgradeHelper, err error) {
	defer func() { err = sn.checkCancelled(err) }()

	for _, disk := range d.Disks {
		sn.partitions = append(sn.partitions, disk.Partitions...)
	}

	if ok, err := sn.isInitiated(d); ok {
		return sn.snapperContext, nil
	} else if err != nil {
		return nil, fmt.Errorf("determining snapshots state: %w", err)
	}
	err = sn.snap.InitRootVolumes(sn.rootDir)
	if err != nil {
		return nil, fmt.Errorf("initializing root snapper volumes: %w", err)
	}
	return sn.snapperContext, nil
}

// Start starts a transaction for this snapper instance and returns the work in progress transaction object.
func (sn snapperT) Start() (trans *Transaction, err error) {
	defer func() { err = sn.checkCancelled(err) }()

	sn.s.Logger().Info("Starting a btrfs snapshotter transaction")

	if len(sn.hwPartitions) == 0 {
		return nil, fmt.Errorf("uninitialized snapshotter")
	}

	sn.s.Logger().Info("Creating new snapshot")
	trans, err = sn.createNewSnapshot(sn.defaultID)
	if err != nil {
		return nil, fmt.Errorf("creating new snapshot: %w", err)
	}

	sn.s.Logger().Info("Setting RW subvolumes")
	err = sn.preparePartitions(sn.defaultID, trans)
	if err != nil {
		err = fmt.Errorf("preparing partitions: %w", err)
		return trans, sn.Rollback(trans, err)
	}

	return trans, nil
}

// Commit closes the current transaction (fstab update, SELinux relabel, set snapshot as RO, ...)
func (sn snapperT) Commit(trans *Transaction) (err error) {
	defer func() { err = sn.checkCancelled(err) }()

	if trans.status != started {
		return fmt.Errorf("transaction '%d' is not started", trans.ID)
	}
	sn.s.Logger().Info("Committing transaction")

	sn.s.Logger().Info("Creating post-transaction snapshots")
	err = sn.createPostSnapshots(trans.Path)
	if err != nil {
		return fmt.Errorf("creating post transaction snapshots: %w", err)
	}

	sn.s.Logger().Info("Setting new default snapshot")
	err = sn.snap.SetDefault(trans.Path, trans.ID, map[string]string{updateProgress: ""})
	if err != nil {
		return fmt.Errorf("setting new default snapshot: %w", err)
	}

	// We are ignoring these errors as the default snapshot is already changed
	// so rebooting is expected to succeed to the new system already
	iErr := sn.snap.Cleanup(sn.rootDir, sn.maxSnapshots)
	if iErr != nil {
		sn.s.Logger().Warn("failed to clear old snapshots")
	}
	iErr = sn.cleanStack.Cleanup(err)
	if iErr != nil {
		sn.s.Logger().Warn("failed to cleanup transaction resources")
	}
	sn.s.Logger().Info("Transaction closed")
	trans.status = committed
	return nil
}

// Rollback closes the given in progress transaction by deleting the
// associated resources. This is a cleanup method in case occurs during a transaction.
func (sn snapperT) Rollback(trans *Transaction, e error) (err error) {
	if trans.status == committed {
		sn.s.Logger().Warn("cannot rollback a committed transaction")
		return e
	}
	sn.s.Logger().Error("Closing transaction due to a failure: %v", e)
	err = sn.cleanStack.Cleanup(e)
	err = errors.Join(err, sn.snap.DeleteByPath(trans.Path))
	trans.status = failed
	return err
}

// isInitiatied checks if the current snapper instance is already initiated.
// Does nothing if it is already initiated or initiates it if it is not.
func (sn *snapperT) isInitiated(d deployment.Deployment) (bool, error) {
	if sn.defaultID > 0 {
		return true, nil
	}
	err := sn.probe(d)
	if err != nil {
		return false, err
	}
	return sn.defaultID > 0, nil
}

// probe analyses the current host and finds the current system partition and
// the root of the current snapshot.
func (sn *snapperT) probe(d deployment.Deployment) (err error) {
	blockDev := lsblk.NewLsDevice(sn.s)
	sn.hwPartitions, err = blockDev.GetAllPartitions()
	if err != nil {
		return fmt.Errorf("probing host partitions: %w", err)
	}

	sysPart := d.GetSystemPartition()
	if sysPart == nil {
		return fmt.Errorf("no system partition defined in deployment")
	}

	part := sn.hwPartitions.GetByUUIDNameOrLabel(sysPart.UUID, sysPart.Role.String(), sysPart.Label)
	if part == nil {
		return fmt.Errorf("system partition not found: %+v", sysPart)
	}

	mountPoints, err := sn.s.Mounter().GetMountPoints(part.Path)
	if err != nil {
		return fmt.Errorf("getting mount points: %w", err)
	} else if len(mountPoints) == 0 {
		return fmt.Errorf("no mountpoints found for device '%s'", part.Path)
	}

	r := regexp.MustCompile(fmt.Sprintf(`%s/.snapshots/\d+/snapshot$`, btrfs.TopSubVol))
	for _, mnt := range mountPoints {
		for _, opt := range mnt.Opts {
			if r.Match([]byte(opt)) {
				sn.rootDir = mnt.Path
			}
		}
	}

	if sn.rootDir != "" {
		snaps, err := sn.snap.ListSnapshots(sn.rootDir, "root")
		if err != nil {
			return fmt.Errorf("listing snapshots: %w", err)
		}
		sn.defaultID = snaps.GetDefault()
		sn.activeID = snaps.GetActive()
	} else {
		// Assume a freshly formatted partition
		// setting root over top level volume
		sn.rootDir = filepath.Join(mountPoints[0].Path, btrfs.TopSubVol)
	}

	return nil
}

// mountPartition mounts the given partition to the given mount point. In addition it also
// sets the umount cleanup task.
func (sn snapperT) mountPartition(part *deployment.Partition, mountPoint string) error {
	err := vfs.MkdirAll(sn.s.FS(), mountPoint, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("creating partition mountpoint path '%s': %w", mountPoint, err)
	}
	bPart := sn.hwPartitions.GetByUUID(part.UUID)
	if bPart == nil {
		return fmt.Errorf("partition '%s' not found", part.UUID)
	}
	err = sn.s.Mounter().Mount(bPart.Path, mountPoint, "", []string{"rw"})
	if err != nil {
		return fmt.Errorf("mounting partition at '%s': %w", mountPoint, err)
	}
	sn.cleanStack.Push(func() error { return sn.s.Mounter().Unmount(mountPoint) })
	return nil
}

// mountPartitionToTempDir mounts the given partition to a temporary directory. In addition
// it also sets the umount cleanup task and the temporary directory removal task.
func (sn snapperT) mountPartitionToTempDir(part *deployment.Partition) (string, error) {
	mountPoint, err := vfs.TempDir(sn.s.FS(), "", "elemental_"+part.Role.String())
	if err != nil {
		return "", fmt.Errorf("creating a temporary directory: %w", err)
	}
	sn.cleanStack.PushSuccessOnly(func() error { return sn.s.FS().RemoveAll(mountPoint) })

	err = sn.mountPartition(part, mountPoint)
	if err != nil {
		return "", err
	}

	return mountPoint, nil
}

// mountVol mounts the given volume from the given partition. In addition it also sets
// the umount cleanup task.
func (sn snapperT) mountVol(part *deployment.Partition, volumePath, mountPoint string) error {
	bPart := sn.hwPartitions.GetByUUID(part.UUID)
	if bPart == nil {
		return fmt.Errorf("partition '%s' not found", part.UUID)
	}
	err := vfs.MkdirAll(sn.s.FS(), mountPoint, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("creating mountpoint at '%s': %w", mountPoint, err)
	}
	err = sn.s.Mounter().Mount(
		bPart.Path, mountPoint, "",
		[]string{"rw", fmt.Sprintf("subvol=%s", filepath.Join(btrfs.TopSubVol, volumePath))},
	)
	if err != nil {
		return fmt.Errorf("mounting rw volume at '%s': %w", mountPoint, err)
	}
	sn.cleanStack.Push(func() error { return sn.s.Mounter().Unmount(mountPoint) })
	return nil
}

// createSnapshottedVolWithMerge creates a new snapshotted rw volume with all the associated snapshots required
// for a 3 way merge process. It returns a new Merge struct.
func (sn snapperT) createSnapshottedVolWithMerge(root string, target string, rwVol deployment.RWVolume) (*Merge, error) {
	snaps, err := sn.snap.ListSnapshots(root, snapper.ConfigName(rwVol.Path))
	if err != nil {
		return nil, fmt.Errorf("listing snapshots for rw volume '%s': %w", rwVol.Path, err)
	}
	stock := snaps.GetWithUserdata("stock", "true")
	if len(stock) != 1 {
		return nil, fmt.Errorf("inconsistent number of stock rw snapshots for subvolume '%s': %d", rwVol.Path, len(stock))
	}
	preID, err := sn.snap.CreateSnapshot(
		root, snapper.ConfigName(rwVol.Path), 0, false,
		fmt.Sprintf("pre-transaction %s snapshot", rwVol.Path), map[string]string{"pre-transaction": "true"},
	)
	if err != nil {
		return nil, fmt.Errorf("creating the pre-transaction snapshot for volume '%s': %w", rwVol.Path, err)
	}
	oldStockPath := filepath.Join(root, rwVol.Path, fmt.Sprintf(snapshotPathTmpl, stock[0]))
	err = btrfs.CreateSnapshot(sn.s, target, oldStockPath, !rwVol.NoCopyOnWrite)
	if err != nil {
		return nil, fmt.Errorf("creating the snapshotted volume '%s': %w", rwVol.Path, err)
	}
	return &Merge{
		Old:      oldStockPath,
		Modified: filepath.Join(root, rwVol.Path, fmt.Sprintf(snapshotPathTmpl, preID)),
	}, nil
}

// createSnapshottedVol creates a new snapshotted rw volume, if there is a base snapshot as reference it creates
// a mergeable volume and if no base snapshot is defined (e.g. install time) it just creates a new volume.
func (sn snapperT) createSnapshottedVol(baseID int, rwVol deployment.RWVolume, path string, basePath string) (*Merge, error) {
	fullVolPath := filepath.Join(path, rwVol.Path)
	err := sn.s.FS().RemoveAll(fullVolPath)
	if err != nil {
		return nil, fmt.Errorf("clearing the new subvolume path '%s': %w", fullVolPath, err)
	}
	if baseID > 0 {
		merge, err := sn.createSnapshottedVolWithMerge(basePath, fullVolPath, rwVol)
		if err != nil {
			return nil, fmt.Errorf("creating volume with merge: %w", err)
		}
		return merge, nil
	}
	err = btrfs.CreateSubvolume(sn.s, fullVolPath, !rwVol.NoCopyOnWrite)
	if err != nil {
		return nil, fmt.Errorf("creating subvolume: %w", err)
	}
	return nil, nil
}

// createPostSnapshots creates post transaction snapshots of RW volumes. These are the snapshots which include
// the changes applied by the transaction close hook callback.
func (sn snapperT) createPostSnapshots(root string) (err error) {
	for _, rwVol := range sn.partitions.GetSnapshottedVolumes() {
		_, err = sn.snap.CreateSnapshot(
			root, snapper.ConfigName(rwVol.Path), 0, false,
			fmt.Sprintf("post-transaction %s snapshot", rwVol.Path),
			map[string]string{"post-transaction": "true"},
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// preparePartitions essentially prepares all the volumes and partitions to be mounted at the
// expected locations of the new snapshot for the given in progress transaction.
func (sn snapperT) preparePartitions(baseID int, trans *Transaction) error {
	for _, part := range sn.partitions {
		if part.Role != deployment.System && part.MountPoint != "" {
			err := sn.mountPartition(part, filepath.Join(trans.Path, part.MountPoint))
			if err != nil {
				return err
			}
		}
		for _, rwVol := range part.RWVolumes {
			volumePath := filepath.Join(trans.Path, rwVol.Path)
			err := vfs.MkdirAll(sn.s.FS(), filepath.Dir(volumePath), vfs.DirPerm)
			if err != nil {
				return fmt.Errorf("creating snapshotted subvolume path for '%s': %w", rwVol.Path, err)
			}
			if rwVol.Snapshotted {
				rootPath := trans.Path
				basePath := filepath.Join(sn.rootDir, fmt.Sprintf(snapshotPathTmpl, baseID))
				if part.Role != deployment.System {
					tmpDir, err := sn.mountPartitionToTempDir(part)
					if err != nil {
						return fmt.Errorf("mounting partition '%s' to temp dir: %w", part.UUID, err)
					}
					rootPath = filepath.Join(tmpDir, fmt.Sprintf(snapshotPathTmpl, trans.ID))
					basePath = filepath.Join(tmpDir, fmt.Sprintf(snapshotPathTmpl, baseID))
				}
				sn.s.Logger().Debug("Creating snapshotted subvolume for '%s'", rwVol.Path)
				merge, err := sn.createSnapshottedVol(baseID, rwVol, rootPath, basePath)
				if err != nil {
					return fmt.Errorf("creating snapshotted subvolume '%s': %w", rwVol.Path, err)
				}
				if merge != nil {
					trans.Merges[rwVol.Path] = merge
				}
				if part.Role != deployment.System {
					err = sn.mountVol(part, filepath.Join(fmt.Sprintf(snapshotPathTmpl, trans.ID), rwVol.Path), volumePath)
					if err != nil {
						return fmt.Errorf("mounting partition '%s': %w", part.UUID, err)
					}
				}
			} else {
				err = sn.mountVol(part, rwVol.Path, volumePath)
				if err != nil {
					return fmt.Errorf("mounting partition '%s': %w", part.UUID, err)
				}
			}
		}
	}
	return nil
}

// createNewSnapshot creates a new snapshot based on the given baseID. In case basedID == 0, this method
// assumes it will be creating the first snapshot.
func (sn snapperT) createNewSnapshot(baseID int) (*Transaction, error) {
	var newID int
	var path string
	var err error

	if baseID == 0 {
		newID, err = sn.snap.FirstRootSnapshot(sn.rootDir, map[string]string{updateProgress: "yes"})
		if err != nil {
			return nil, err
		}
		path = filepath.Join(sn.rootDir, fmt.Sprintf(snapshotPathTmpl, newID))
	} else {
		desc := fmt.Sprintf("snapshot created from parent snapshot %d", baseID)
		newID, err = sn.snap.CreateSnapshot(sn.rootDir, "", baseID, true, desc, map[string]string{updateProgress: "yes"})
		if err != nil {
			return nil, err
		}
		path = filepath.Join(sn.rootDir, fmt.Sprintf(snapshotPathTmpl, newID))
	}

	return &Transaction{
		ID:     newID,
		Path:   path,
		Merges: map[string]*Merge{},
		status: started,
	}, nil
}
