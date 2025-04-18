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

package partitioner

type Partitioner interface {
	WriteChanges() (string, error)
	SetPartitionTableLabel(label string) error
	CreatePartition(p *Partition)
	DeletePartition(num int)
	SetPartitionFlag(num int, flag string, active bool)
	WipeTable(wipe bool)
	GetLastSector(printOut string) (uint, error)
	Print() (string, error)
	GetSectorSize(printOut string) (uint, error)
	GetPartitionTableLabel(printOut string) (string, error)
	GetPartitions(printOut string) []Partition
}

// We only manage sizes in sectors unit for the Partition structre in parted wrapper
// FileSystem here is only used by parted to determine the partition ID or type
type Partition struct {
	Number     int
	StartS     uint
	SizeS      uint
	PLabel     string
	FileSystem string
}
