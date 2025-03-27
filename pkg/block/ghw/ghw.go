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

package ghw

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jaypipes/ghw"
	ghwblock "github.com/jaypipes/ghw/pkg/block"
	ghwUtil "github.com/jaypipes/ghw/pkg/util"
	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/mounter"
)

type ghwDevice struct {
	runner  sys.Runner
	mounter mounter.Interface
}

func NewGhwDevice(s *sys.System) *ghwDevice { //nolint:revive
	return &ghwDevice{runner: s.Runner(), mounter: s.Mounter()}
}

var _ block.Device = (*ghwDevice)(nil)

// ghwPartitionToInternalPartition transforms a block.Partition from ghw lib to our types.Partition type
func ghwPartitionToInternalPartition(m mounter.Interface, partition *ghwblock.Partition) (*block.Partition, error) {
	mnts := []string{partition.MountPoint}
	if partition.MountPoint != "" {
		extraMnts, err := m.GetMountRefs(partition.MountPoint)
		if err != nil {
			return nil, err

		}
		mnts = append(mnts, extraMnts...)
	}
	return &block.Partition{
		Label:       partition.FilesystemLabel,
		Size:        uint(partition.SizeBytes / (1024 * 1024)), // Converts B to MB
		Name:        partition.Label,
		FileSystem:  partition.Type,
		UUID:        partition.UUID,
		Flags:       nil,
		MountPoints: mnts,
		Path:        filepath.Join("/dev", partition.Name),
		Disk:        filepath.Join("/dev", partition.Disk.Name),
	}, nil
}

// GetAllPartitions returns all partitions in the system for all disks
func (b ghwDevice) GetAllPartitions() (block.PartitionList, error) {
	var parts []*block.Partition
	blockDevices, err := ghwblock.New(ghw.WithDisableTools(), ghw.WithDisableWarnings())
	if err != nil {
		return nil, err
	}
	for _, d := range blockDevices.Disks {
		for _, part := range d.Partitions {
			p, err := ghwPartitionToInternalPartition(b.mounter, part)
			if err != nil {
				return nil, err
			}
			parts = append(parts, p)
		}
	}

	return parts, nil
}

// GetDevicePartitions gets the partitions for the given disk
func (b ghwDevice) GetDevicePartitions(device string) (block.PartitionList, error) {
	var parts []*block.Partition
	// We want to have the device always prefixed with a /dev
	if !strings.HasPrefix(device, "/dev") {
		device = filepath.Join("/dev", device)
	}
	blockDevices, err := ghwblock.New(ghw.WithDisableTools(), ghw.WithDisableWarnings())
	if err != nil {
		return parts, err
	}

	for _, disk := range blockDevices.Disks {
		if filepath.Join("/dev", disk.Name) == device {
			for _, part := range disk.Partitions {
				p, err := ghwPartitionToInternalPartition(b.mounter, part)
				if err != nil {
					return nil, err
				}
				parts = append(parts, p)
			}
		}
	}
	return parts, nil
}

// GetPartitionFS gets the FS of a partition given
func (b ghwDevice) GetPartitionFS(partition string) (string, error) {
	// We want to have the device always prefixed with a /dev
	if !strings.HasPrefix(partition, "/dev") {
		partition = filepath.Join("/dev", partition)
	}
	blockDevices, err := ghwblock.New(ghw.WithDisableTools(), ghw.WithDisableWarnings())
	if err != nil {
		return "", err
	}

	for _, disk := range blockDevices.Disks {
		for _, part := range disk.Partitions {
			if filepath.Join("/dev", part.Name) == partition {
				if part.Type == ghwUtil.UNKNOWN {
					return "", fmt.Errorf("could not find filesystem for partition %s", partition)
				}
				return part.Type, nil
			}
		}
	}
	return "", fmt.Errorf("could not find partition %s", partition)
}
