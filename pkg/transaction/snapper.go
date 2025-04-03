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
	"github.com/suse/elemental/v3/pkg/chroot"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/fstab"
	"github.com/suse/elemental/v3/pkg/rsync"
	"github.com/suse/elemental/v3/pkg/selinux"
	"github.com/suse/elemental/v3/pkg/snapper"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

const (
	snapshotPathTmpl = ".snapshots/%d/snapshot"
	updateProgress   = "update-in-progress"
	maxSnapshots     = 8
)

type snapperT struct {
	ctx          context.Context
	s            *sys.System
	snap         *snapper.Snapper
	defaultID    int
	activeID     int
	rootDir      string
	partitions   deployment.Partitions
	fullSync     bool
	cleanStack   *cleanstack.CleanStack
	maxSnapshots int
	hwPartitions block.PartitionList
}

func NewSnapperTransaction(ctx context.Context, s *sys.System) Interface {
	return &snapperT{
		ctx:          ctx,
		s:            s,
		cleanStack:   cleanstack.NewCleanStack(),
		snap:         snapper.New(s),
		maxSnapshots: maxSnapshots,
	}
}

// Init checks initial snapshotter configuration and sets it if needed
func (sn *snapperT) Init(d deployment.Deployment) (err error) {
	defer func() { err = sn.checkCancelled(err) }()

	for _, disk := range d.Disks {
		sn.partitions = append(sn.partitions, disk.Partitions...)
	}

	if ok, err := sn.isInitiated(d); ok {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to determine snapshots state: %w", err)
	}
	sn.fullSync = true
	return sn.snap.InitSnapperRootVolumes(sn.rootDir)
}

// Start starts a transaction for this snapper instance and returns the work in progress snapshot object.
// This method already syncs the given image source to the just started transaction.
func (sn snapperT) Start(imgSrc *deployment.ImageSource) (trans *Transaction, err error) {
	defer func() { err = sn.checkCancelled(err) }()

	sn.s.Logger().Info("Starting a btrfs snapshotter transaction")

	if len(sn.hwPartitions) == 0 {
		sn.s.Logger().Error("snapshotter should have been initalized before starting a transaction")
		return nil, fmt.Errorf("uninitialized snapshotter")
	}

	sn.s.Logger().Info("Creating new snapshot")
	trans, err = sn.createNewSnapshot(sn.defaultID)
	if err != nil {
		sn.s.Logger().Error("failed creating new snapshot")
		return nil, err
	}

	sn.s.Logger().Info("Setting RW subvolumes")
	err = sn.preparePartitions(sn.defaultID, trans)
	if err != nil {
		sn.s.Logger().Error("failed setting rw volumes")
		return trans, sn.CloseOnError(trans, err)
	}

	sn.s.Logger().Info("Dump image source to new snapshot")
	err = sn.syncImageContent(imgSrc, trans.Path)
	if err != nil {
		sn.s.Logger().Error("failed dumping source to snapshot")
		return trans, sn.CloseOnError(trans, err)
	}

	sn.s.Logger().Info("Configure snapper")
	err = sn.configureSnapper(trans)
	if err != nil {
		sn.s.Logger().Error("failed configuring snapper")
		return trans, sn.CloseOnError(trans, err)
	}

	return trans, nil
}

// Merge runs a 3 way merge for snapshotted RW volumes
// Current implemementation is dumb, there is no check on potential conflicts
func (sn snapperT) Merge(trans *Transaction) (err error) {
	defer func() { err = sn.checkCancelled(err) }()

	if !trans.inProgress {
		return fmt.Errorf("given transaction '%d' not in progress", trans.ID)
	}

	sn.s.Logger().Info("Startin 3 way merge of snapshotted rw volumes")
	for _, rwVol := range sn.partitions.GetSnaphsottedVolumes() {
		m := trans.Merges[rwVol.Path]
		if m == nil {
			continue
		}
		r := rsync.NewRsync(sn.s, rsync.WithContext(sn.ctx))
		err = r.SyncData(m.Modified, m.New, snapper.SnapshotsPath)
		if err != nil {
			sn.s.Logger().Error("failed merging content for volume '%s'", rwVol.Path)
			return err
		}
	}
	return nil
}

// Close closes the current transaction (fstab update, SELinux relabel, hook execution, set new default...)
// The hook callback is executed in a chroot environment where the new snapshot is already set to RO mode.
func (sn snapperT) Close(trans *Transaction, hook Hook, binds HookBinds) (err error) {
	defer func() { err = sn.checkCancelled(err) }()

	if !trans.inProgress {
		return fmt.Errorf("given transaction '%d' not in progress", trans.ID)
	}

	sn.s.Logger().Info("Closing transaction")

	sn.s.Logger().Info("Update fstab")
	if ok, _ := vfs.Exists(sn.s.FS(), filepath.Join(trans.Path, fstab.File)); ok {
		err = sn.updateFstab(trans)
		if err != nil {
			sn.s.Logger().Error("failed updating fstab file")
			return err
		}
	} else {
		err = sn.createFstab(trans)
		if err != nil {
			sn.s.Logger().Error("failed creatingpdating fstab file")
			return err
		}
	}

	err = selinux.ChrootedRelabel(sn.ctx, sn.s, trans.Path, nil)
	if err != nil {
		sn.s.Logger().Error("failed relabelling snapshot path: %s", trans.Path)
		return err
	}

	err = sn.createPostSnapshots(trans.Path)
	if err != nil {
		sn.s.Logger().Error("failed creating post transaction snapshots")
		return err
	}

	sn.s.Logger().Info("Setting new snapshot as read-only")
	err = sn.snap.SetPermissions(trans.Path, trans.ID, false)
	if err != nil {
		sn.s.Logger().Error("failed setting new snapshot as RO")
		return err
	}

	if hook != nil {
		sn.s.Logger().Info("Running transaction hook")
		err = chroot.ChrootedCallback(sn.s, trans.Path, binds, hook)
		if err != nil {
			sn.s.Logger().Error("failed to run transaction hook")
			return err
		}
	}

	sn.s.Logger().Info("Setting new default snapshot")
	err = sn.snap.SetDefault(trans.Path, trans.ID, map[string]string{updateProgress: ""})
	if err != nil {
		sn.s.Logger().Error("failed setting new default snapshot")
		return err
	}

	// TODO bootloader installation/upgrade could be handled here: efivars, shim image install, esp partition sync

	// We are ignoring these errors as the default snapshot is already changed
	// so rebooting is expected to succeed to the new system already
	iErr := sn.snap.Cleanup(trans.Path, sn.maxSnapshots)
	if iErr != nil {
		sn.s.Logger().Warn("failed to clear old snapshots")
	}
	iErr = sn.cleanStack.Cleanup(err)
	if iErr != nil {
		sn.s.Logger().Warn("failed to cleanup transaction resources")
	}
	sn.s.Logger().Info("Transaction closed")

	return nil
}

// CloseOnError closes the ginve in progress transaction by deleting the
// associated resources. This is a cleanup method in case occurs during a transaction.
func (sn snapperT) CloseOnError(trans *Transaction, e error) (err error) {
	sn.s.Logger().Error("closing transaction due to a failure: %v", e)
	err = sn.cleanStack.Cleanup(e)
	err = errors.Join(err, sn.snap.DeleteByPath(trans.Path))
	trans.inProgress = false
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
		return fmt.Errorf("failed probing host partitions: %w", err)
	}
	sysPart := d.GetSystemPartition()
	if sysPart == nil {
		return fmt.Errorf("no system partition defined in deployment")
	}
	part := sn.hwPartitions.GetByUUIDNameOrLabel(sysPart.UUID, sysPart.Role.String(), sysPart.Label)
	if part == nil {
		return fmt.Errorf("system partition not found: %+v", sysPart)
	}
	mnts, err := sn.s.Mounter().GetMountPoints(part.Path)
	if err != nil {
		return err
	}
	if len(mnts) == 0 {
		return fmt.Errorf("no mountpoints found for device '%s'", part.Path)
	}
	r := regexp.MustCompile(fmt.Sprintf(`%s/.snapshots/\d+/snapshot`, btrfs.TopSubVol))
	for _, mnt := range mnts {
		for _, opt := range mnt.Opts {
			if r.Match([]byte(opt)) {
				sn.rootDir = mnt.Path
			}
		}
	}
	if sn.rootDir != "" {
		snaps, err := sn.snap.ListSnapshots(sn.rootDir, "root")
		if err != nil {
			return err
		}
		sn.defaultID = snaps.GetDefault()
		sn.activeID = snaps.GetActive()
	} else {
		// Assume a freshly formatted partition
		// setting root over top level volume
		sn.rootDir = filepath.Join(mnts[0].Path, btrfs.TopSubVol)
	}

	return nil
}

// configureSnapper sets the snapper configuration for root and any snapshotted
// volume.
func (sn snapperT) configureSnapper(trans *Transaction) error {
	err := sn.snap.ConfigureRoot(trans.Path, sn.maxSnapshots)
	if err != nil {
		sn.s.Logger().Error("failed setting root configuration for snapper")
		return err
	}
	src := filepath.Join(sn.rootDir, snapper.SnapshotsPath)
	target := filepath.Join(trans.Path, snapper.SnapshotsPath)
	err = vfs.MkdirAll(sn.s.FS(), target, vfs.DirPerm)
	if err != nil {
		sn.s.Logger().Error("failed creating snapshots folder into the new root")
		return err
	}
	err = sn.s.Mounter().Mount(src, target, "", []string{"bind"})
	if err != nil {
		sn.s.Logger().Error("failed bind mounting snapshots volume to the new root")
		return err
	}
	sn.cleanStack.Push(func() error { return sn.s.Mounter().Unmount(target) })
	err = sn.configureRWVolumes(trans)
	if err != nil {
		sn.s.Logger().Error("failed setting snapshotted subvolumes for snapper")
		return err
	}
	return nil
}

// syncImageContent unpacks the given image source to the given destination
func (sn snapperT) syncImageContent(imgSrc *deployment.ImageSource, destPath string) error {
	sn.s.Logger().Info("Unpacking image source: %s", imgSrc.String())
	unpacker, err := unpack.NewUnpacker(sn.s, imgSrc)
	if err != nil {
		sn.s.Logger().Error("failed initatin image unpacker")
		return err
	}
	digest, err := unpacker.SynchedUnpack(sn.ctx, destPath, sn.syncSnapshotExcludes(), sn.syncSnapshotDeleteExcludes())
	if err != nil {
		sn.s.Logger().Error("failed unpacking image to '%s'", destPath)
		return err
	}
	imgSrc.SetDigest(digest)

	return nil
}

// syncSnapshotExcludes sets the excluded directories for the image source sync.
// non snapshotted rw volumes are excluded on upgrades, but included for the very first
// snapshots at installation time.
func (sn snapperT) syncSnapshotExcludes() []string {
	excludes := []string{filepath.Join("/", snapper.SnapshotsPath)}
	for _, part := range sn.partitions {
		if !sn.fullSync && part.Role != deployment.System && part.MountPoint != "" {
			excludes = append(excludes, part.MountPoint)
		}
		for _, rwVol := range part.RWVolumes {
			if rwVol.Snapshotted {
				excludes = append(excludes, filepath.Join(rwVol.Path, snapper.SnapshotsPath))
			} else if !sn.fullSync {
				excludes = append(excludes, rwVol.Path)
			}
		}
	}
	return excludes
}

// syncSnapshotDeleteExcludes sets the protected paths at sync destination. RW volume
// paths can't be deleted as part of sync, as they are likely to be mountpoints.
func (sn snapperT) syncSnapshotDeleteExcludes() []string {
	excludes := []string{filepath.Join("/", snapper.SnapshotsPath)}
	for _, part := range sn.partitions {
		if part.Role != deployment.System && part.MountPoint != "" {
			excludes = append(excludes, part.MountPoint)
		}
		for _, rwVol := range part.RWVolumes {
			excludes = append(excludes, rwVol.Path)
		}
	}
	return excludes
}

// mountPartition mounts the given partition to the given mount point. In addition it also
// sets the umount cleanup task.
func (sn snapperT) mountPartition(part *deployment.Partition, mountPoint string) error {
	err := vfs.MkdirAll(sn.s.FS(), mountPoint, vfs.DirPerm)
	if err != nil {
		sn.s.Logger().Error("failed creating partition mountpoint path '%s'", mountPoint)
		return err
	}
	bPart := sn.hwPartitions.GetByUUID(part.UUID)
	if bPart == nil {
		sn.s.Logger().Error("failed to find partition '%s'", part.UUID)
		return fmt.Errorf("partition not found")
	}
	err = sn.s.Mounter().Mount(bPart.Path, mountPoint, "", []string{"rw"})
	if err != nil {
		sn.s.Logger().Error("failed mounting partition at '%s'", mountPoint)
		return err
	}
	sn.cleanStack.Push(func() error { return sn.s.Mounter().Unmount(mountPoint) })
	return nil
}

// mountPartitionToTempDir mounts the given partition to a temporary directory. In addition
// it also sets the umount cleanup task and the temporary directory removal task.
func (sn snapperT) mountPartitionToTempDir(part *deployment.Partition) (string, error) {
	mountPoint, err := vfs.TempDir(sn.s.FS(), "", "elemental_"+part.Role.String())
	if err != nil {
		sn.s.Logger().Error("failed creating a temporary directory to mount partition %s", part.UUID)
		return "", err
	}
	sn.cleanStack.PushSuccessOnly(func() error { return vfs.RemoveAll(sn.s.FS(), mountPoint) })

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
		sn.s.Logger().Error("failed to find partition '%s'", part.UUID)
		return fmt.Errorf("partition not found")
	}
	err := vfs.MkdirAll(sn.s.FS(), mountPoint, vfs.DirPerm)
	if err != nil {
		sn.s.Logger().Error("failed to create mountpoint", mountPoint)
		return err
	}
	err = sn.s.Mounter().Mount(
		bPart.Path, mountPoint, "",
		[]string{"rw", fmt.Sprintf("subvol=%s", filepath.Join(btrfs.TopSubVol, volumePath))},
	)
	if err != nil {
		sn.s.Logger().Error("failed mounting rw volume '%s'", mountPoint)
		return err
	}
	sn.cleanStack.Push(func() error { return sn.s.Mounter().Unmount(mountPoint) })
	return nil
}

// createSnapshottedVolWithMerge creates a new snapshotted rw volume with all the associated snapshots required
// for a 3 way merge process. It returns a new Merge struct.
func (sn snapperT) createSnapshottedVolWithMerge(root string, target string, rwVol deployment.RWVolume) (*Merge, error) {
	snaps, err := sn.snap.ListSnapshots(root, snapper.ConfigName(rwVol.Path))
	if err != nil {
		sn.s.Logger().Error("failed to list snapshots for rw subvolume '%s'", rwVol.Path)
		return nil, err
	}
	stock := snaps.GetWithUserdata("stock", "true")
	if len(stock) != 1 {
		sn.s.Logger().Error("failed to find stock snapshot for base '%d'and subvolume '%s'", rwVol.Path)
		return nil, fmt.Errorf("inconsistent number of stock rw snapshots: %d", len(stock))
	}
	preID, err := sn.snap.CreateSnapshot(
		root, snapper.ConfigName(rwVol.Path), 0, false,
		fmt.Sprintf("pre-transaction %s snapshot", rwVol.Path), map[string]string{"pre-transaction": "true"},
	)
	if err != nil {
		sn.s.Logger().Error("failed creating the pre-transaction snapshot for volume '%s': %s", rwVol.Path)
		return nil, err
	}
	oldStockPath := filepath.Join(root, rwVol.Path, fmt.Sprintf(snapshotPathTmpl, stock[0]))
	err = btrfs.CreateSnapshot(sn.s, oldStockPath, target, !rwVol.NoCopyOnWrite)
	if err != nil {
		sn.s.Logger().Error("failed creating the snapshotted volume '%s'", rwVol.Path)
		return nil, err
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
	err := vfs.RemoveAll(sn.s.FS(), fullVolPath)
	if err != nil {
		sn.s.Logger().Error("failed to clear the new subvolume path '%s'", fullVolPath)
		return nil, err
	}
	if baseID > 0 {
		root := filepath.Join(basePath, rwVol.Path)
		merge, err := sn.createSnapshottedVolWithMerge(root, fullVolPath, rwVol)
		if err != nil {
			sn.s.Logger().Error("failed to create snapshotted volume '%s'", fullVolPath)
			return nil, err
		}
		return merge, nil
	}
	err = btrfs.CreateSubvolume(sn.s, fullVolPath, !rwVol.NoCopyOnWrite)
	if err != nil {
		sn.s.Logger().Error("failed creating snapshotted subvolume '%s'", rwVol.Path)
		return nil, err
	}
	return nil, nil
}

// createPostSnapshots creates post transaction snapshots of RW volumes. These are the snapshots which include
// the changes applied by the transaction close hook callback.
func (sn snapperT) createPostSnapshots(root string) (err error) {
	for _, rwVol := range sn.partitions.GetSnaphsottedVolumes() {
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

// preparePartitions this method essentially prepares all the volumes and partitions to be mounted at the
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
				sn.s.Logger().Error("failed creating snapshotted subvolume  path for '%s'", rwVol.Path)
				return err
			}
			if rwVol.Snapshotted {
				rootPath := trans.Path
				basePath := filepath.Join(sn.rootDir, fmt.Sprintf(snapshotPathTmpl, baseID))
				if part.Role != deployment.System {
					tmpDir, err := sn.mountPartitionToTempDir(part)
					if err != nil {
						sn.s.Logger().Error("failed mounting partition %s", part.UUID)
						return err
					}
					rootPath = filepath.Join(tmpDir, fmt.Sprintf(snapshotPathTmpl, trans.ID))
					basePath = filepath.Join(tmpDir, fmt.Sprintf(snapshotPathTmpl, baseID))
				}
				sn.s.Logger().Debug("Creating snapshotted subvolume for '%s'", rwVol.Path)
				merge, err := sn.createSnapshottedVol(baseID, rwVol, rootPath, basePath)
				if err != nil {
					sn.s.Logger().Error("failed creating snapshotted subvolume '%s'", rwVol.Path)
					return err
				}
				if merge != nil {
					trans.Merges[rwVol.Path] = merge
				}
				if part.Role != deployment.System {
					err := sn.mountVol(part, filepath.Join(fmt.Sprintf(snapshotPathTmpl, trans.ID), rwVol.Path), volumePath)
					if err != nil {
						return err
					}
				}
			} else {
				err := sn.mountVol(part, rwVol.Path, volumePath)
				if err != nil {
					return err
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
		desc := fmt.Sprintf("creating snapsot from former %d snapshot", baseID)
		newID, err = sn.snap.CreateSnapshot(sn.rootDir, "", baseID, true, desc, map[string]string{updateProgress: "yes"})
		if err != nil {
			return nil, err
		}
		path = filepath.Join(sn.rootDir, fmt.Sprintf(snapshotPathTmpl, newID))
	}

	return &Transaction{
		ID:         newID,
		Path:       path,
		Merges:     map[string]*Merge{},
		inProgress: true,
	}, nil
}

// configureRWVolumes sets the configuration for the nested snapshotted paths
func (sn snapperT) configureRWVolumes(trans *Transaction) error {
	callback := func() error {
		for _, rwVol := range sn.partitions.GetSnaphsottedVolumes() {
			err := sn.snap.CreateConfig("/", rwVol.Path)
			if err != nil {
				return err
			}
			newID, err := sn.snap.CreateSnapshot(
				"/", snapper.ConfigName(rwVol.Path), 0, false,
				fmt.Sprintf("stock %s contents", rwVol.Path),
				map[string]string{"stock": "true"},
			)
			if err != nil {
				return err
			}
			if _, ok := trans.Merges[rwVol.Path]; ok {
				trans.Merges[rwVol.Path].New = filepath.Join(
					trans.Path, rwVol.Path, fmt.Sprintf(snapshotPathTmpl, newID),
				)
			}
		}
		return nil
	}
	return chroot.ChrootedCallback(sn.s, trans.Path, nil, callback, chroot.WithoutDefaultBinds())
}

// checkCancelled just checks if the current context is cancelled or not. Returns the context error if cancelled.
func (sn snapperT) checkCancelled(err error) error {
	if err != nil {
		return err
	}
	select {
	case <-sn.ctx.Done():
		return sn.ctx.Err()
	default:
		return nil
	}
}

// updateFstab updates the fstab file with the given transaction data
func (sn snapperT) updateFstab(trans *Transaction) error {
	var oldLines, newLines []fstab.Line
	for _, part := range sn.partitions {
		for _, rwVol := range part.RWVolumes {
			if !rwVol.Snapshotted {
				continue
			}
			subVol := filepath.Join(btrfs.TopSubVol, fmt.Sprintf(snapshotPathTmpl, trans.ID), rwVol.Path)
			opts := rwVol.MountOpts
			oldLines = append(oldLines, fstab.Line{MountPoint: rwVol.Path})
			newLines = append(newLines, fstab.Line{
				Device:     fmt.Sprintf("UUID=%s", part.UUID),
				MountPoint: rwVol.Path,
				Options:    append(opts, fmt.Sprintf("subvol=%s", subVol)),
				FileSystem: part.FileSystem.String(),
			})
		}
	}
	fstabFile := filepath.Join(trans.Path, fstab.File)
	return fstab.UpdateFstab(sn.s, fstabFile, oldLines, newLines)
}

// createFstab creates the fstab file with the given transaction data
func (sn snapperT) createFstab(trans *Transaction) (err error) {
	var fstabLines []fstab.Line
	for _, part := range sn.partitions {
		if part.MountPoint != "" {
			var line fstab.Line

			opts := part.MountOpts
			if part.Role == deployment.System {
				opts = append([]string{"ro"}, opts...)
				line.FsckOrder = 1
			} else {
				line.FsckOrder = 2
			}
			if len(opts) == 0 {
				opts = []string{"defaults"}
			}
			line.Device = fmt.Sprintf("UUID=%s", part.UUID)
			line.MountPoint = part.MountPoint
			line.Options = opts
			line.FileSystem = part.FileSystem.String()
			fstabLines = append(fstabLines, line)
		}
		for _, rwVol := range part.RWVolumes {
			var subVol string
			var line fstab.Line

			if rwVol.Snapshotted {
				subVol = filepath.Join(btrfs.TopSubVol, fmt.Sprintf(snapshotPathTmpl, trans.ID), rwVol.Path)
			} else {
				subVol = filepath.Join(btrfs.TopSubVol, rwVol.Path)
			}
			opts := rwVol.MountOpts
			opts = append(opts, fmt.Sprintf("subvol=%s", subVol))
			line.Device = fmt.Sprintf("UUID=%s", part.UUID)
			line.MountPoint = rwVol.Path
			line.Options = opts
			line.FileSystem = part.FileSystem.String()
			fstabLines = append(fstabLines, line)
		}
		if part.Role == deployment.System {
			var line fstab.Line
			subVol := filepath.Join(btrfs.TopSubVol, snapper.SnapshotsPath)
			line.Device = fmt.Sprintf("UUID=%s", part.UUID)
			line.MountPoint = filepath.Join("/", snapper.SnapshotsPath)
			line.Options = []string{fmt.Sprintf("subvol=%s", subVol)}
			line.FileSystem = part.FileSystem.String()
			fstabLines = append(fstabLines, line)
		}
	}
	err = fstab.WriteFstab(sn.s, filepath.Join(trans.Path, fstab.File), fstabLines)
	if err != nil {
		sn.s.Logger().Error("failed writing fstab file")
		return err
	}
	return nil
}
