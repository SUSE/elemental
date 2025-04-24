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
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/sys"
)

const (
	// Partition table type
	gpt = "gpt"

	// ESP partition type
	esp = "esp"
)

// WipeFSOnPartition removes any pre-existing filesystem on the given partition
func WipeFSOnPartition(s *sys.System, device string) error {
	_, err := s.Runner().Run("wipefs", "--all", device)
	return err
}

// FormatDevice formats a block device with the given parameters
func FormatDevice(s *sys.System, device string, fileSystem string, label string, uuid string, opts ...string) error {
	mkfs := NewMkfsCall(s, device, fileSystem, label, uuid, opts...)
	return mkfs.Apply()
}

// PartitionAndFormatDevice creates a new empty partition table on target disk
// and applies the configured disk layout by creating and formatting all
// required partitions
func PartitionAndFormatDevice(s *sys.System, d *deployment.Disk) error {
	disk := NewDisk(s, d.Device, WithStartSector(d.StartSector))

	if !disk.Exists() {
		return fmt.Errorf("disk %s does not exist", d.Device)
	}

	s.Logger().Info("Partitioning device '%s'", d.Device)
	out, err := disk.NewPartitionTable(gpt)
	if err != nil {
		s.Logger().Error("failed creating new partition table: %s", out)
		return err
	}
	for _, part := range d.Partitions {
		err := AddAndFormatPartition(s, disk, part)
		if err != nil {
			s.Logger().Error("failed creating new %s partition", part.Role.String())
			return err
		}
	}
	return nil
}

// AddAndFormatPartition adds the given partition to the given disk. The partition is appended
// after last already existing partition. The partition is also formatted with the given
// parameters.
func AddAndFormatPartition(s *sys.System, disk *Disk, part *deployment.Partition) error {
	s.Logger().Debug("Adding %s partition with label %s", part.Role.String(), part.Label)

	var pLabel string
	var flags []string
	switch part.Role {
	case deployment.EFI:
		flags = append(flags, esp)
		pLabel = part.Role.String()
	case deployment.System, deployment.Recovery:
		pLabel = part.Role.String()
	}

	num, err := disk.AddPartition(uint(part.Size), part.FileSystem.String(), pLabel, flags...)
	if err != nil {
		s.Logger().Error("failed creating %s partition", part.Label)
		return err
	}
	partDev, err := disk.FindPartitionDevice(num)
	if err != nil {
		return err
	}

	fuuid := part.UUID
	if fuuid == "" {
		fuuid = generateUUID(part.FileSystem)
	}

	s.Logger().Debug("Formatting partition with uuid %s", fuuid)
	err = FormatDevice(s, partDev, part.FileSystem.String(), part.Label, fuuid)
	if err != nil {
		s.Logger().Error("failed formatting partition %s", part.Label)
		return err
	}
	part.UUID = vfatUUIDSanitize(fuuid, part.FileSystem)
	return nil
}

func checkUUID(id string, f deployment.FileSystem) bool {
	if f == deployment.VFat {
		id = strings.ReplaceAll(id, "-", "")
		if len([]rune(id)) != 8 {
			return false
		}
		_, err := hex.DecodeString(id)
		return err == nil
	}
	_, err := uuid.Parse(id)
	return err == nil
}

func generateUUID(f deployment.FileSystem) string {
	id := uuid.Must(uuid.NewRandom()).String()
	if f == deployment.VFat {
		return strings.Split(id, "-")[0]
	}
	return id
}

func vfatUUIDSanitize(id string, f deployment.FileSystem) string {
	if f == deployment.VFat {
		id = strings.ToUpper(id)
		runes := []rune(id)[0:4]
		runes = append(runes, []rune("-")...)
		runes = append(runes, []rune(id)[4:]...)
		return string(runes)
	}
	return id
}
