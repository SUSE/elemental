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

package mock

import (
	"fmt"

	"github.com/suse/elemental/v3/pkg/block"
)

var _ block.Device = (*Device)(nil)

type Device struct {
	partitions block.PartitionList
	err        error
}

func NewBlockDevice(partitions ...*block.Partition) *Device {
	return &Device{partitions: partitions}
}

func (m *Device) SetPartitions(partitions block.PartitionList) {
	m.partitions = partitions
}

func (m *Device) SetError(err error) {
	m.err = err
}

func (m Device) GetAllPartitions() (block.PartitionList, error) {
	return m.partitions, m.err
}

func (m Device) GetDevicePartitions(device string) (block.PartitionList, error) {
	var parts block.PartitionList
	for _, part := range m.partitions {
		if part.Disk == device {
			parts = append(parts, part)
		}
	}
	return parts, m.err
}

func (m Device) GetPartitionFS(partition string) (string, error) {
	if m.err != nil {
		return "", m.err
	}

	for _, part := range m.partitions {
		if part.Path == partition {
			return part.FileSystem, nil
		}
	}
	return "", fmt.Errorf("MockBlockDevice: partition '%s' not found", partition)
}
