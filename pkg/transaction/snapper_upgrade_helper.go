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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
func (sc snapperContext) SyncImageContent(imgSrc *deployment.ImageSource, trans *Transaction, opts ...unpack.Opt) (err error) {
	defer func() { err = sc.checkCancelled(err) }()
	if trans.status != started {
		return fmt.Errorf("given transaction '%d' is not started", trans.ID)
	}
	var unpacker unpack.Interface

	sc.s.Logger().Info("Unpacking image source: %s", imgSrc.String())
	unpacker, err = unpack.NewUnpacker(sc.s, imgSrc, opts...)
	if err != nil {
		return fmt.Errorf("initializing unpacker: %w", err)
	}
	// The very first transaction requires full synchronization (e.g. /var, /etc, etc.).
	// Its ID is 1.
	digest, err := unpacker.SynchedUnpack(sc.ctx, trans.Path, sc.syncSnapshotExcludes(trans.ID == 1), sc.syncSnapshotDeleteExcludes())
	if err != nil {
		return fmt.Errorf("unpacking image to '%s': %w", trans.Path, err)
	}
	imgSrc.SetDigest(digest)

	return nil
}

// Merge performs a three way merge of snapshotted customizable paths
func (sc snapperContext) Merge(trans *Transaction) (err error) {
	defer func() { err = sc.checkCancelled(err) }()
	if trans.status != started {
		return fmt.Errorf("transaction '%d' is not started", trans.ID)
	}

	sc.s.Logger().Info("Configure snapper")
	err = sc.configureSnapper(trans)
	if err != nil {
		return fmt.Errorf("configuring snapper: %w", err)
	}

	sc.s.Logger().Info("Starting 3 way merge of snapshotted rw volumes")
	err = sc.merge(trans)
	if err != nil {
		return fmt.Errorf("merging content of snapshotted rw volumes: %w", err)
	}
	return nil
}

// UpdateFstab updates fstab file including the new snapshots
func (sc snapperContext) UpdateFstab(trans *Transaction) (err error) {
	defer func() { err = sc.checkCancelled(err) }()
	if trans.status != started {
		return fmt.Errorf("transaction '%d' is not started", trans.ID)
	}

	sc.s.Logger().Info("Updating fstab")
	if ok, _ := vfs.Exists(sc.s.FS(), filepath.Join(trans.Path, fstab.File)); ok {
		return sc.updateFstab(trans)
	}

	err = sc.createFstab(trans)
	if err != nil {
		return fmt.Errorf("creating fstab: %w", err)
	}
	return nil
}

// Lock sets the main transaction snapshot to readonly mode
func (sc snapperContext) Lock(trans *Transaction) (err error) {
	defer func() { err = sc.checkCancelled(err) }()
	if trans.status != started {
		return fmt.Errorf("transaction '%d' is not started", trans.ID)
	}

	sc.s.Logger().Info("Setting new snapshot as read-only")
	err = sc.snap.SetPermissions(trans.Path, trans.ID, false)
	if err != nil {
		return fmt.Errorf("configuring new snapshot as read-only: %w", err)
	}
	return nil
}

// GenerateKernelCmdline generates the kernel cmdline needed to boot into the snapshot generated by the passed in transaction.
func (sc snapperContext) GenerateKernelCmdline(trans *Transaction) string {
	return fmt.Sprintf("rootfstype=btrfs rootflags=subvol=@/.snapshots/%d/snapshot", trans.ID)
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

// configureSnapper sets the snapper configuration for root and any snapshotted volume.
func (sc snapperContext) configureSnapper(trans *Transaction) error {
	err := sc.snap.ConfigureRoot(trans.Path, sc.maxSnapshots)
	if err != nil {
		return fmt.Errorf("setting root configuration: %w", err)
	}

	src := filepath.Join(trans.Path, "../../")
	target := filepath.Join(trans.Path, snapper.SnapshotsPath)
	err = vfs.MkdirAll(sc.s.FS(), target, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("creating snapshots dir: %w", err)
	}

	err = sc.s.Mounter().Mount(src, target, "", []string{"bind"})
	if err != nil {
		return fmt.Errorf("mounting snapshots volume: %w", err)
	}

	sc.cleanStack.Push(func() error { return sc.s.Mounter().Unmount(target) })
	err = sc.configureRWVolumes(trans)
	if err != nil {
		return fmt.Errorf("configuring snapshotted subvolumes: %w", err)
	}
	return nil
}

// configureRWVolumes sets the configuration for the nested snapshotted paths
func (sc snapperContext) configureRWVolumes(trans *Transaction) error {
	callback := func() error {
		for _, rwVol := range sc.partitions.GetSnapshottedVolumes() {
			err := sc.snap.CreateConfig("/", rwVol.Path)
			if err != nil {
				return fmt.Errorf("creating config for '%s': %w", rwVol.Path, err)
			}

			config := snapper.ConfigName(rwVol.Path)
			description := fmt.Sprintf("stock %s contents", rwVol.Path)
			metadata := map[string]string{"stock": "true"}

			_, err = sc.snap.CreateSnapshot("/", config, 0, false, description, metadata)
			if err != nil {
				return fmt.Errorf("creating snapshot '%s': %w", rwVol.Path, err)
			}

			if _, ok := trans.Merges[rwVol.Path]; ok {
				trans.Merges[rwVol.Path].New = filepath.Join(trans.Path, rwVol.Path)
			}
		}
		return nil
	}
	return chroot.ChrootedCallback(sc.s, trans.Path, nil, callback, chroot.WithoutDefaultBinds())
}

// merge runs a 3 way merge for snapshotted RW volumes.
// Current implementation solves potential conflicts by always keeping
// custom changes over changes coming from the OS image.
func (sc snapperContext) merge(trans *Transaction) (err error) {
	var status, tmpDir string

	for _, rwVol := range sc.partitions.GetSnapshottedVolumes() {
		m := trans.Merges[rwVol.Path]
		if m == nil {
			continue
		}

		tmpDir, err = vfs.TempDir(sc.s.FS(), "", "snapStatus")
		if err != nil {
			return fmt.Errorf("failed creating temporary directory to store snapper output: %w", err)
		}
		defer func() {
			e := sc.s.FS().RemoveAll(tmpDir)
			if err == nil {
				err = e
			}
		}()

		status = filepath.Join(tmpDir, fmt.Sprintf("snap_status_%s", snapper.ConfigName(rwVol.Path)))
		err = sc.customChangesStatus(rwVol.Path, m, status)
		if err != nil {
			return err
		}

		err = sc.applyCustomChanges(status, rwVol.Path, m)
		if err != nil {
			return err
		}
	}
	return nil
}

// customChangesStatus checks the status between the old stock content and the current customized content
// and stores the result in the given output file
func (sc snapperContext) customChangesStatus(volPath string, merge *Merge, output string) (err error) {
	oldID, err := snapshotIDFromPath(merge.Old)
	if err != nil {
		return err
	}

	modifiedID, err := snapshotIDFromPath(merge.Modified)
	if err != nil {
		return err
	}

	root, err := rootFromMerge(volPath, merge)
	if err != nil {
		return err
	}

	err = sc.snap.Status(root, snapper.ConfigName(volPath), output, oldID, modifiedID)
	if err != nil {
		return err
	}

	return nil
}

// applyCustomChanges reads the given status file and applies reported changes in to the target destination.
// This method is the responsible of applying customizations to the new volume
func (sc snapperContext) applyCustomChanges(status, rwVolPath string, merge *Merge) (err error) {
	sc.s.Logger().Debug("rw volume path: %s", rwVolPath)
	statusF, err := sc.s.FS().OpenFile(status, os.O_RDONLY, vfs.FilePerm)
	if err != nil {
		return err
	}
	defer func() {
		e := statusF.Close()
		if err != nil {
			err = fmt.Errorf("failed closing status file: %w", e)
		}
	}()

	syncFiles := filepath.Join(filepath.Dir(status), fmt.Sprintf("sync_%s", snapper.ConfigName(rwVolPath)))
	syncF, err := sc.s.FS().OpenFile(syncFiles, os.O_CREATE|os.O_WRONLY, vfs.FilePerm)
	if err != nil {
		return fmt.Errorf("failed opening modified files list: %w", err)
	}

	r := regexp.MustCompile(`(([-+ct.])[p.][u.][g.][x.][a.])\s+(.*)`)

	scanner := bufio.NewScanner(statusF)
	for scanner.Scan() {
		line := scanner.Text()
		match := r.FindStringSubmatch(line)

		switch {
		case len(match) == 0:
			continue
		case match[1] == "....":
			// Ignore extended attributes changes because the stock snapshot used for
			// comparison was taken before SELINUX relabelling, hence this is likely to
			// list almost every single file.
			continue
		case match[2] == "-":
			err = sc.s.FS().RemoveAll(filepath.Join(merge.New, match[3]))
			if err != nil {
				_ = syncF.Close()
				return err
			}
		default:
			_, err = fmt.Fprintln(syncF, strings.TrimPrefix(match[3], rwVolPath))
			if err != nil {
				_ = syncF.Close()
				return err
			}
		}
	}
	err = syncF.Close()
	if err != nil {
		return fmt.Errorf("failed closing modified files list: %w", err)
	}

	syncFlags := append(rsync.DefaultFlags(), "--files-from", syncFiles)

	sync := rsync.NewRsync(sc.s, rsync.WithContext(sc.ctx), rsync.WithFlags(syncFlags...))
	err = sync.SyncData(merge.Modified, merge.New, snapper.SnapshotsPath)
	if err != nil {
		return err
	}

	return nil
}

// snapshotIDFromPath determines the snapshot ID form the snapshot root path
func snapshotIDFromPath(path string) (int, error) {
	r := regexp.MustCompile(`.*/.snapshots/(\d+)/snapshot$`)
	match := r.FindStringSubmatch(path)
	if match == nil {
		return 0, fmt.Errorf("could not determine snapshot ID to diff")
	}
	id, _ := strconv.Atoi(match[1])
	return id, nil
}

// rootFromMerge determines the snapper root based on the merge snapshots paths
func rootFromMerge(volPath string, merge *Merge) (string, error) {
	r := regexp.MustCompile(fmt.Sprintf(`(.*)%s/.snapshots/\d+/snapshot$`, volPath))
	matchOld := r.FindStringSubmatch(merge.Old)
	if matchOld == nil {
		return "", fmt.Errorf("could not determine snapper root for path %s", merge.Old)
	}
	matchModified := r.FindStringSubmatch(merge.Modified)
	if matchModified == nil {
		return "", fmt.Errorf("could not determine snapper root for path %s", merge.Modified)
	}
	if matchModified[1] != matchOld[1] {
		return "", fmt.Errorf("could not determine snapper root, inconsistent merge")
	}
	return matchModified[1], nil
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
func (sc snapperContext) createFstab(trans *Transaction) error {
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

	return fstab.WriteFstab(sc.s, filepath.Join(trans.Path, fstab.File), fstabLines)
}
