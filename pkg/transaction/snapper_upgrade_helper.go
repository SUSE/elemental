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
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/btrfs"
	"github.com/suse/elemental/v3/pkg/chroot"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/fstab"
	"github.com/suse/elemental/v3/pkg/rsync"
	"github.com/suse/elemental/v3/pkg/snapper"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

// SyncImageContent syncs the given image tree to given transaction. For the first transaction all content
// is synced regardless if some paths are under a persistent path or not. On upgrades it only syncs the immutable
// content and snapshotted paths.
func (sc snapperContext) SyncImageContent(imgSrc *deployment.ImageSource, trans *Transaction) (err error) {
	defer func() { err = sc.checkCancelled(err) }()
	if trans.status != started {
		return fmt.Errorf("given transaction '%d' is not started", trans.ID)
	}
	var unpacker unpack.Interface

	sc.s.Logger().Info("Unpacking image source: %s", imgSrc.String())
	unpacker, err = unpack.NewUnpacker(sc.s, imgSrc)
	if err != nil {
		sc.s.Logger().Error("failed initatin image unpacker")
		return err
	}
	// The very first transaction requires full synchronization (e.g. /var, /etc, etc.). First transaction ID is 1
	digest, err := unpacker.SynchedUnpack(sc.ctx, trans.Path, sc.syncSnapshotExcludes(trans.ID == 1), sc.syncSnapshotDeleteExcludes())
	if err != nil {
		sc.s.Logger().Error("failed unpacking image to '%s'", trans.Path)
		return err
	}
	imgSrc.SetDigest(digest)

	return nil
}

// Merge performs a three way merge of snapshotted customizable paths
func (sc snapperContext) Merge(trans *Transaction) (err error) {
	defer func() { err = sc.checkCancelled(err) }()
	if trans.status != started {
		return fmt.Errorf("given transaction '%d' is not started", trans.ID)
	}

	sc.s.Logger().Info("Configure snapper")
	err = sc.configureSnapper(trans)
	if err != nil {
		sc.s.Logger().Error("failed configuring snapper")
		return err
	}

	sc.s.Logger().Info("Starting 3 way merge of snapshotted rw volumes")
	err = sc.merge(trans)
	if err != nil {
		sc.s.Logger().Error("failed merging content of snapshotted rw volumes")
	}
	return err
}

// UpdateFstab updates fstab file including the new snapshots
func (sc snapperContext) UpdateFstab(trans *Transaction) (err error) {
	defer func() { err = sc.checkCancelled(err) }()
	if trans.status != started {
		return fmt.Errorf("given transaction '%d' is not started", trans.ID)
	}

	sc.s.Logger().Info("Update fstab")
	if ok, _ := vfs.Exists(sc.s.FS(), filepath.Join(trans.Path, fstab.File)); ok {
		err = sc.updateFstab(trans)
		if err != nil {
			sc.s.Logger().Error("failed updating fstab file")

		}
		return err
	}
	err = sc.createFstab(trans)
	if err != nil {
		sc.s.Logger().Error("failed creatingpdating fstab file")

	}
	return err
}

// Lock sets the main transaction snapshot to readonly mode
func (sc snapperContext) Lock(trans *Transaction) (err error) {
	defer func() { err = sc.checkCancelled(err) }()
	if trans.status != started {
		return fmt.Errorf("given transaction '%d' is not started", trans.ID)
	}

	sc.s.Logger().Info("Setting new snapshot as read-only")
	err = sc.snap.SetPermissions(trans.Path, trans.ID, false)
	if err != nil {
		sc.s.Logger().Error("failed setting new snapshot as RO")
	}
	return err
}

// syncSnapshotExcludes sets the excluded directories for the image source sync.
// non snapshotted rw volumes are excluded on upgrades, but included for the very first
// snapshots at installation time.
func (sc snapperContext) syncSnapshotExcludes(fullSync bool) []string {
	excludes := []string{filepath.Join("/", snapper.SnapshotsPath)}
	for _, part := range sc.partitions {
		if !fullSync && part.Role != deployment.System && part.MountPoint != "" {
			excludes = append(excludes, part.MountPoint)
		}
		for _, rwVol := range part.RWVolumes {
			if rwVol.Snapshotted {
				excludes = append(excludes, filepath.Join(rwVol.Path, snapper.SnapshotsPath))
			} else if !fullSync {
				excludes = append(excludes, rwVol.Path)
			}
		}
	}
	return excludes
}

// syncSnapshotDeleteExcludes sets the protected paths at sync destination. RW volume
// paths can't be deleted as part of sync, as they are likely to be mountpoints.
func (sc snapperContext) syncSnapshotDeleteExcludes() []string {
	excludes := []string{filepath.Join("/", snapper.SnapshotsPath)}
	for _, part := range sc.partitions {
		if part.Role != deployment.System && part.MountPoint != "" {
			excludes = append(excludes, part.MountPoint)
		}
		for _, rwVol := range part.RWVolumes {
			excludes = append(excludes, rwVol.Path)
		}
	}
	return excludes
}

// configureSnapper sets the snapper configuration for root and any snapshotted
// volume.
func (sc snapperContext) configureSnapper(trans *Transaction) error {
	err := sc.snap.ConfigureRoot(trans.Path, sc.maxSnapshots)
	if err != nil {
		sc.s.Logger().Error("failed setting root configuration for snapper")
		return err
	}
	src := filepath.Join(trans.Path, "../../")
	target := filepath.Join(trans.Path, snapper.SnapshotsPath)
	err = vfs.MkdirAll(sc.s.FS(), target, vfs.DirPerm)
	if err != nil {
		sc.s.Logger().Error("failed creating snapshots folder into the new root")
		return err
	}
	err = sc.s.Mounter().Mount(src, target, "", []string{"bind"})
	if err != nil {
		sc.s.Logger().Error("failed bind mounting snapshots volume to the new root")
		return err
	}
	sc.cleanStack.Push(func() error { return sc.s.Mounter().Unmount(target) })
	err = sc.configureRWVolumes(trans)
	if err != nil {
		sc.s.Logger().Error("failed setting snapshotted subvolumes for snapper")
		return err
	}
	return nil
}

// configureRWVolumes sets the configuration for the nested snapshotted paths
func (sc snapperContext) configureRWVolumes(trans *Transaction) error {
	callback := func() error {
		for _, rwVol := range sc.partitions.GetSnapshottedVolumes() {
			err := sc.snap.CreateConfig("/", rwVol.Path)
			if err != nil {
				return err
			}
			_, err = sc.snap.CreateSnapshot(
				"/", snapper.ConfigName(rwVol.Path), 0, false,
				fmt.Sprintf("stock %s contents", rwVol.Path),
				map[string]string{"stock": "true"},
			)
			if err != nil {
				return err
			}
			if _, ok := trans.Merges[rwVol.Path]; ok {
				trans.Merges[rwVol.Path].New = filepath.Join(
					trans.Path, rwVol.Path,
				)
			}
		}
		return nil
	}
	return chroot.ChrootedCallback(sc.s, trans.Path, nil, callback, chroot.WithoutDefaultBinds())
}

// merge runs a 3 way merge for snapshotted RW volumes
// Current implemementation is dumb, there is no check on potential conflicts
func (sc snapperContext) merge(trans *Transaction) (err error) {
	for _, rwVol := range sc.partitions.GetSnapshottedVolumes() {
		m := trans.Merges[rwVol.Path]
		if m == nil {
			continue
		}
		r := rsync.NewRsync(sc.s, rsync.WithContext(sc.ctx))
		err = r.SyncData(m.Modified, m.New, snapper.SnapshotsPath)
		if err != nil {
			return err
		}
	}
	return nil
}

// updateFstab updates the fstab file with the given transaction data
func (sc snapperContext) updateFstab(trans *Transaction) error {
	var oldLines, newLines []fstab.Line
	for _, part := range sc.partitions {
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
	return fstab.UpdateFstab(sc.s, fstabFile, oldLines, newLines)
}

// createFstab creates the fstab file with the given transaction data
func (sc snapperContext) createFstab(trans *Transaction) (err error) {
	var fstabLines []fstab.Line
	for _, part := range sc.partitions {
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
	err = fstab.WriteFstab(sc.s, filepath.Join(trans.Path, fstab.File), fstabLines)
	if err != nil {
		sc.s.Logger().Error("failed writing fstab file")
		return err
	}
	return nil
}
