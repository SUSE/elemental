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

package lsblk

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/sys"
)

type lsDevice struct {
	runner sys.Runner
}

func NewLsDevice(s *sys.System) *lsDevice { //nolint:revive
	return &lsDevice{runner: s.Runner()}
}

var _ block.Device = (*lsDevice)(nil)

type jPart struct {
	Label       string   `json:"label,omitempty"`
	Name        string   `json:"partlabel,omitempty"`
	UUID        string   `json:"partuuid,omitempty"`
	Size        uint64   `json:"size,omitempty"`
	FS          string   `json:"fstype,omitempty"`
	MountPoints []string `json:"mountpoints,omitempty"`
	Path        string   `json:"path,omitempty"`
	Disk        string   `json:"pkname,omitempty"`
	Type        string   `json:"type,omitempty"`
}

type jParts []*block.Partition

func (p jPart) Partition() *block.Partition {
	// Converts B to MB
	return &block.Partition{
		Label:       p.Label,
		Size:        uint(p.Size / (1024 * 1024)),
		FileSystem:  p.FS,
		UUID:        p.UUID,
		Flags:       []string{},
		MountPoints: p.MountPoints,
		Path:        p.Path,
		Disk:        p.Disk,
		Name:        p.Name,
	}
}

func (p *jParts) UnmarshalJSON(data []byte) error {
	var parts []jPart

	if err := json.Unmarshal(data, &parts); err != nil {
		return err
	}

	var partitions jParts
	for _, part := range parts {
		// filter only partition or loop devices
		if part.Type == "part" || part.Type == "loop" {
			partitions = append(partitions, part.Partition())
		}
	}
	*p = partitions
	return nil
}

func unmarshalLsblk(lsblkOut []byte) ([]*block.Partition, error) {
	var objmap map[string]*json.RawMessage
	err := json.Unmarshal(lsblkOut, &objmap)
	if err != nil {
		return nil, err
	}

	if _, ok := objmap["blockdevices"]; !ok {
		return nil, errors.New("invalid json object, no 'blockdevices' key found")
	}

	var parts jParts
	err = json.Unmarshal(*objmap["blockdevices"], &parts)
	if err != nil {
		return nil, err
	}

	return parts, nil
}

func unmarshalSectorSize(lsblkOut []byte) (uint, error) {
	var objmap map[string]*json.RawMessage
	err := json.Unmarshal(lsblkOut, &objmap)
	if err != nil {
		return 0, err
	}

	if _, ok := objmap["blockdevices"]; !ok {
		return 0, errors.New("invalid json object, no 'blockdevices' key found")
	}

	devices := []struct {
		Name       string `json:"name,omitempty"`
		SectorSize uint   `json:"phy-sec,omitempty"`
	}{}
	err = json.Unmarshal(*objmap["blockdevices"], &devices)
	if err != nil {
		return 0, err
	}
	if len(devices) == 0 {
		return 0, fmt.Errorf("no devices reported by lsblk")
	}
	return devices[0].SectorSize, nil
}

// GetAllPartitions gets a slice of all partition devices found in the host
// mapped into a v1.PartitionList object.
func (l lsDevice) GetAllPartitions() (block.PartitionList, error) {
	out, err := l.runner.Run("lsblk", "-p", "-b", "-n", "-J", "--output", "LABEL,PARTLABEL,PARTUUID,SIZE,FSTYPE,MOUNTPOINTS,PATH,PKNAME,TYPE")
	if err != nil {
		return nil, err
	}

	return unmarshalLsblk(out)
}

// GetDevicePartitions gets a slice of partitions found in the given device mapped
// into a v1.PartitionList object. If the device is a disk it will list all disk
// partitions, if the device is already a partition it will simply list a single partition.
func (l lsDevice) GetDevicePartitions(device string) (block.PartitionList, error) {
	out, err := l.runner.Run("lsblk", "-p", "-b", "-n", "-J", "--output", "LABEL,PARTLABEL,PARTUUID,SIZE,FSTYPE,MOUNTPOINTS,PATH,PKNAME,TYPE", device)
	if err != nil {
		return nil, err
	}

	return unmarshalLsblk(out)
}

// GetDeviceSectorSize returns the physical sector size for the given block device
func (l lsDevice) GetDeviceSectorSize(device string) (uint, error) {
	out, err := l.runner.Run("lsblk", "-J", "-d", "-o", "NAME,PHY-SEC", device)
	if err != nil {
		return 0, err
	}

	size, err := unmarshalSectorSize(out)
	if err != nil {
		return 0, err
	}

	if size == 0 {
		return 0, fmt.Errorf("no sector size reported by lsblk %v", device)
	}

	return size, err
}

// GetPartitionFS gets the filesystem type for the given partition device. If the given device
// is can't be parsed as a single partition by lsblk it will error out.
func (l lsDevice) GetPartitionFS(partition string) (string, error) {
	pLst, err := l.GetDevicePartitions(partition)
	if err != nil {
		return "", err
	}
	if len(pLst) != 1 {
		return "", fmt.Errorf("could not parse a single partition: %v", pLst)
	}
	return pLst[0].FileSystem, nil
}
