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

package block

import (
	"errors"
	"time"

	"github.com/suse/elemental/v3/pkg/sys"
)

const Ghw = "ghw"
const Lsblk = "lsblk"

type Device interface {
	GetAllPartitions() (PartitionList, error)
	GetDevicePartitions(device string) (PartitionList, error)
	GetPartitionFS(partition string) (string, error)
}

// Partition struct represents a partition with its commonly configurable values, size in MiB
type Partition struct {
	Name        string
	Label       string
	Size        uint
	FileSystem  string
	UUID        string
	Flags       []string
	MountPoints []string
	Path        string
	Disk        string
}

type PartitionList []*Partition

// GetByName gets a partitions by its name from the PartitionList
func (pl PartitionList) GetByName(name string) *Partition {
	var part *Partition

	for _, p := range pl {
		if p.Name == name {
			part = p
			// Prioritize mounted partitions if there are multiple matches
			if len(part.MountPoints) > 0 {
				return part
			}
		}
	}
	return part
}

// GetByLabel gets a partition by its label from the PartitionList
func (pl PartitionList) GetByLabel(label string) *Partition {
	var part *Partition

	for _, p := range pl {
		if p.Label == label {
			part = p
			// Prioritize mounted partitions if there are multiple matches
			if len(part.MountPoints) > 0 {
				return part
			}
		}
	}
	return part
}

// GetByUUID gets a partition by its filesystem UUID from the PartitionList
func (pl PartitionList) GetByUUID(uuid string) *Partition {
	var part *Partition

	for _, p := range pl {
		if p.UUID == uuid {
			return part
		}
	}
	return part
}

// GetByNameOrLabel gets a partition by its uuid, name or label. It tries by uuid first and label as the option.
func (pl PartitionList) GetByUUIDNameOrLabel(uuid, name, label string) *Partition {
	part := pl.GetByUUID(uuid)
	if part == nil {
		part = pl.GetByName(name)
		if part == nil {
			part = pl.GetByLabel(label)
		}
	}
	return part
}

// GetPartitionByLabel tries to return the partition that matches the given label.
// attempts value sets the number of attempts to find the device, it
// waits a second between attempts.
func GetPartitionByLabel(s *sys.System, b Device, label string, attempts int) (*Partition, error) {
	for range attempts {
		_, _ = s.Runner().Run("udevadm", "settle")
		parts, err := b.GetAllPartitions()
		if err != nil {
			return nil, err
		}
		part := parts.GetByLabel(label)
		if part != nil {
			return part, nil
		}
		time.Sleep(1 * time.Second)
	}
	return nil, errors.New("no device found")
}

// GetPartitionByUUID tries to return the partition that matches the given filesystem UUID.
// attempts value sets the number of attempts to find the device, it
// waits a second between attempts.
func GetPartitionByUUID(s *sys.System, b Device, uuid string, attempts int) (*Partition, error) {
	for range attempts {
		_, _ = s.Runner().Run("udevadm", "settle")
		parts, err := b.GetAllPartitions()
		if err != nil {
			return nil, err
		}
		part := parts.GetByUUID(uuid)
		if part != nil {
			return part, nil
		}
		time.Sleep(1 * time.Second)
	}
	return nil, errors.New("no device found")
}

// GetPartitionDeviceByLabel will try to return the device that matches the given label.
// attempts value sets the number of attempts to find the device, it
// waits a second between attempts.
func GetPartitionDeviceByLabel(s *sys.System, b Device, label string, attempts int) (string, error) {
	part, err := GetPartitionByLabel(s, b, label, attempts)
	if err != nil {
		return "", err
	}
	return part.Path, nil
}
