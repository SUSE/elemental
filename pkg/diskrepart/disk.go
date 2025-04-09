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

package diskrepart

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/block/lsblk"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/diskrepart/partitioner"
	"github.com/suse/elemental/v3/pkg/diskrepart/partitioner/gdisk"
	"github.com/suse/elemental/v3/pkg/diskrepart/partitioner/parted"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	partitionTries = 10
	// Parted warning substring for expanded disks without fixing GPT headers
	partedWarn    = "Not all of the space available"
	sgdiskProblem = "Problem: The secondary header"

	// Paritioner backend
	partedBack = "parted"
	gdiskBack  = "sgdisk"
)

var unallocatedRegexp = regexp.MustCompile(fmt.Sprintf("(%s|%s)", partedWarn, sgdiskProblem))

type Disk struct {
	device      string
	sectorS     uint
	lastS       uint
	parts       []partitioner.Partition
	label       string
	sys         *sys.System
	partBackend string
	blockDevice block.Device
}

type DiskOptions func(d *Disk) error

func WithGdisk() func(d *Disk) error {
	return func(d *Disk) error {
		d.partBackend = gdiskBack
		return nil
	}
}

func WithParted() func(d *Disk) error {
	return func(d *Disk) error {
		d.partBackend = partedBack
		return nil
	}
}

func WithBlockDevice(bd block.Device) func(d *Disk) error {
	return func(d *Disk) error {
		d.blockDevice = bd
		return nil
	}
}

func MiBToSectors(size uint, sectorSize uint) uint {
	return size * 1048576 / sectorSize
}

func NewDisk(s *sys.System, device string, opts ...DiskOptions) *Disk {
	dev := &Disk{
		device: device,
		sys:    s,
	}

	for _, opt := range opts {
		if err := opt(dev); err != nil {
			return nil
		}
	}

	if dev.blockDevice == nil {
		dev.blockDevice = lsblk.NewLsDevice(s)
	}

	return dev
}

func (dev Disk) String() string {
	return dev.device
}

func (dev Disk) GetSectorSize() uint {
	return dev.sectorS
}

func (dev Disk) GetLastSector() uint {
	return dev.lastS
}

func (dev Disk) GetLabel() string {
	return dev.label
}

func (dev *Disk) Exists() bool {
	fi, err := dev.sys.FS().Stat(dev.device)
	if err != nil {
		return false
	}
	// resolve symlink if any
	if fi.Mode()&os.ModeSymlink != 0 {
		d, err := dev.sys.FS().Readlink(dev.device)
		if err != nil {
			return false
		}
		dev.device = d
	}
	return true
}

func (dev *Disk) Reload() error {
	pc, err := dev.newPartitioner(dev.String())
	if err != nil {
		return err
	}

	prnt, err := pc.Print()
	if err != nil {
		return err
	}

	// if the unallocated space warning is found it is assumed GPT headers
	// are not properly located to match disk size, so we use sgdisk
	// to expand the partition table to fully match disk size.
	// It is expected that in upcoming parted releases (>3.4) there will be
	// --fix flag to solve this issue transparently on the fly on any parted call.
	// However this option is not yet present in all major distros.
	// TODO: Need to experiment with "parted --script --fix /dev/$block_device p"
	if unallocatedRegexp.Match([]byte(prnt)) {
		// Parted has not a proper way to doing it in non interactive mode,
		// because of that we use sgdisk for that...
		_, err = dev.sys.Runner().Run("sgdisk", "-e", dev.device)
		if err != nil {
			return err
		}
		// Reload disk data with fixed headers
		prnt, err = pc.Print()
		if err != nil {
			return err
		}
	}

	sectorS, err := pc.GetSectorSize(prnt)
	if err != nil {
		return err
	}
	lastS, err := pc.GetLastSector(prnt)
	if err != nil {
		return err
	}
	label, err := pc.GetPartitionTableLabel(prnt)
	if err != nil {
		return err
	}
	partitions := pc.GetPartitions(prnt)
	dev.sectorS = sectorS
	dev.lastS = lastS
	dev.parts = partitions
	dev.label = label
	return nil
}

// Size is expressed in MiB here
func (dev *Disk) CheckDiskFreeSpaceMiB(minSpace uint) bool {
	freeS, err := dev.GetFreeSpace()
	if err != nil {
		dev.sys.Logger().Warn("Could not calculate disk free space")
		return false
	}
	minSec := MiBToSectors(minSpace, dev.sectorS)

	return freeS >= minSec
}

func (dev *Disk) GetFreeSpace() (uint, error) {
	// Check we have loaded partition table data
	if dev.sectorS == 0 {
		err := dev.Reload()
		if err != nil {
			dev.sys.Logger().Error("failed analyzing disk: %v", err)
			return 0, err
		}
	}

	return dev.computeFreeSpace(), nil
}

func (dev Disk) computeFreeSpace() uint {
	if len(dev.parts) > 0 {
		lastPart := dev.parts[len(dev.parts)-1]
		return dev.lastS - (lastPart.StartS + lastPart.SizeS - 1)
	}
	// First partition starts at a 1MiB offset
	return dev.lastS - (1*1024*1024/dev.sectorS - 1)
}

func (dev Disk) computeFreeSpaceWithoutLast() uint {
	if len(dev.parts) > 1 {
		part := dev.parts[len(dev.parts)-2]
		return dev.lastS - (part.StartS + part.SizeS - 1)
	}
	// Assume first partitions is alined to 1MiB
	return dev.lastS - (1024*1024/dev.sectorS - 1)
}

func (dev *Disk) NewPartitionTable(label string) (string, error) {
	pc, err := dev.newPartitioner(dev.String())
	if err != nil {
		return "", err
	}

	err = pc.SetPartitionTableLabel(label)
	if err != nil {
		return "", err
	}
	pc.WipeTable(true)
	out, err := pc.WriteChanges()
	if err != nil {
		return out, err
	}
	err = dev.Reload()
	if err != nil {
		dev.sys.Logger().Error("failed analyzing disk: %v", err)
		return "", err
	}
	return out, nil
}

// AddPartition adds a partition. Size is expressed in MiB here
func (dev *Disk) AddPartition(size uint, fileSystem string, pLabel string, flags ...string) (int, error) {
	pc, err := dev.newPartitioner(dev.String())
	if err != nil {
		return 0, err
	}

	// Check we have loaded partition table data
	if dev.sectorS == 0 {
		err = dev.Reload()
		if err != nil {
			dev.sys.Logger().Error("failed analyzing disk: %v", err)
			return 0, err
		}
	}

	err = pc.SetPartitionTableLabel(dev.label)
	if err != nil {
		return 0, err
	}

	var partNum int
	var startS uint
	if len(dev.parts) > 0 {
		lastP := len(dev.parts) - 1
		partNum = dev.parts[lastP].Number
		startS = dev.parts[lastP].StartS + dev.parts[lastP].SizeS
	} else {
		// First partition is aligned at 1MiB
		startS = 1024 * 1024 / dev.sectorS
	}

	size = MiBToSectors(size, dev.sectorS)
	freeS := dev.computeFreeSpace()
	if size > freeS {
		return 0, fmt.Errorf("not enough free space in disk. Required: %d sectors; Available %d sectors", size, freeS)
	}

	partNum++
	var part = partitioner.Partition{
		Number:     partNum,
		StartS:     startS,
		SizeS:      size,
		PLabel:     pLabel,
		FileSystem: fileSystem,
	}

	pc.CreatePartition(&part)
	for _, flag := range flags {
		pc.SetPartitionFlag(partNum, flag, true)
	}

	_, err = pc.WriteChanges()
	if err != nil {
		dev.sys.Logger().Error("failed creating partition: %v", err)
		return 0, err
	}

	// Reload new partition in dev
	err = dev.Reload()
	if err != nil {
		dev.sys.Logger().Error("failed analyzing disk: %v", err)
		return 0, err
	}
	return partNum, nil
}

func (dev Disk) FormatPartition(partNum int, fileSystem string, label string, uuid string) error {
	pDev, err := dev.FindPartitionDevice(partNum)
	if err != nil {
		return err
	}

	mkfs := NewMkfsCall(dev.sys, pDev, fileSystem, label, uuid)
	return mkfs.Apply()
}

func (dev Disk) FindPartitionDevice(partNum int) (string, error) {
	re := regexp.MustCompile(`.*\d+$`)
	var device string

	if match := re.Match([]byte(dev.device)); match {
		device = fmt.Sprintf("%sp%d", dev.device, partNum)
	} else {
		device = fmt.Sprintf("%s%d", dev.device, partNum)
	}

	for tries := 0; tries <= partitionTries; tries++ {
		dev.sys.Logger().Debug("Trying to find the partition device %d of device %s (try number %d)", partNum, dev, tries+1)
		_, _ = dev.sys.Runner().Run("udevadm", "settle")
		if exists, _ := vfs.Exists(dev.sys.FS(), device); exists {
			return device, nil
		}
		time.Sleep(1 * time.Second)
	}
	errMsg := "partition '%d' not found in '%s' device"
	dev.sys.Logger().Error(errMsg, partNum, device)
	return "", fmt.Errorf(errMsg, partNum, device)
}

// ExpandLastPartition expands the latest partition in the disk. Size is expressed in MiB here
// Size is expressed in MiB here
func (dev *Disk) ExpandLastPartition(size uint) error {
	pc, err := dev.newPartitioner(dev.String())
	if err != nil {
		return err
	}

	// Check we have loaded partition table data
	if dev.sectorS == 0 {
		err = dev.Reload()
		if err != nil {
			dev.sys.Logger().Error("failed analyzing disk: %v", err)
			return err
		}
	}

	err = pc.SetPartitionTableLabel(dev.label)
	if err != nil {
		return err
	}

	if len(dev.parts) == 0 {
		return errors.New("there is no partition to expand")
	}

	part := dev.parts[len(dev.parts)-1]
	if size > 0 {
		size = MiBToSectors(size, dev.sectorS)
		part := dev.parts[len(dev.parts)-1]
		if size < part.SizeS {
			return errors.New("layout plugin can only expand a partition, not shrink it")
		}
		freeS := dev.computeFreeSpaceWithoutLast()
		if size > freeS {
			return fmt.Errorf("not enough free space for to expand last partition up to %d sectors", size)
		}
	}
	part.SizeS = size
	pc.DeletePartition(part.Number)
	pc.CreatePartition(&part)
	out, err := pc.WriteChanges()
	if err != nil {
		dev.sys.Logger().Error("failed writing partition changes: %s", out)
		return err
	}
	err = dev.Reload()
	if err != nil {
		return err
	}
	pDev, err := dev.FindPartitionDevice(part.Number)
	if err != nil {
		return err
	}
	return dev.expandFilesystem(pDev)
}

func (dev Disk) expandFilesystem(device string) (err error) {
	var out []byte
	var tmpDir, fs string

	fs, err = dev.blockDevice.GetPartitionFS(device)
	if err != nil {
		return err
	}

	f, _ := deployment.ParseFileSystem(fs)

	switch f {
	case deployment.Ext2, deployment.Ext4:
		out, err = dev.sys.Runner().Run("e2fsck", "-fy", device)
		if err != nil {
			dev.sys.Logger().Error("failed e2fsck call: %s", string(out))
			return err
		}
		out, err = dev.sys.Runner().Run("resize2fs", device)
		if err != nil {
			dev.sys.Logger().Error("failed resize2fs call: %s", string(out))
			return err
		}
	case deployment.XFS, deployment.Btrfs:
		// to grow an xfs or btrfs fs it needs to be mounted :/
		tmpDir, err = vfs.TempDir(dev.sys.FS(), "", "partitioner")
		defer func(fs vfs.FS, path string) {
			_ = fs.RemoveAll(path)
		}(dev.sys.FS(), tmpDir)
		if err != nil {
			return err
		}
		err = dev.sys.Mounter().Mount(device, tmpDir, "", []string{})
		if err != nil {
			return err
		}
		defer func() {
			err2 := dev.sys.Mounter().Unmount(tmpDir)
			if err2 != nil && err == nil {
				err = err2
			}
		}()
		if f == deployment.XFS {
			out, err = dev.sys.Runner().Run("xfs_growfs", tmpDir)
			if err != nil {
				dev.sys.Logger().Error("failed xfs_growfs call: %s", string(out))
				return err
			}
		} else {
			out, err = dev.sys.Runner().Run("btrfs", "filesystem", "resize", "max", tmpDir)
			if err != nil {
				dev.sys.Logger().Error("failed btrfs call: %s", string(out))
				return err
			}
		}
	default:
		return fmt.Errorf("could not find filesystem for %s, not resizing the filesystem", device)
	}

	return nil
}

func (dev Disk) newPartitioner(device string) (partitioner.Partitioner, error) {
	switch dev.partBackend {
	case partedBack:
		return parted.NewPartedCall(dev.sys, device), nil
	case gdiskBack, "":
		return gdisk.NewGdiskCall(dev.sys, device), nil
	default:
		return nil, fmt.Errorf("backend '%s' not supported", dev.partBackend)
	}
}
