/*
Copyright © 2025 SUSE LLC
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

package deployment

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type MiB uint

const (
	EfiLabel     = "EFI"
	EfiMnt       = "/boot/efi"
	EfiSize  MiB = 1024

	RecoveryLabel = "RECOVERY"
	RecoveryMnt   = "/run/elemental/recovery"
	RecoverySize  = 0

	SystemLabel          = "SYSTEM"
	SystemMnt            = "/"
	AllAvailableSize MiB = 0

	DeploymentFile = "/etc/elemental/deployment.yaml"

	Unknown = "unknown"
)

type PartRole int

const (
	EFI PartRole = iota + 1
	System
	Recovery
	Data
)

type FileSystem int

const (
	Btrfs FileSystem = iota + 1
	Ext2
	Ext4
	XFS
	VFat
)

func ParseFileSystem(f string) (FileSystem, error) {
	switch f {
	case "btrfs":
		return Btrfs, nil
	case "ext2":
		return Ext2, nil
	case "ext4":
		return Ext4, nil
	case "xfs":
		return XFS, nil
	case "vfat":
		return VFat, nil
	default:
		return FileSystem(0), fmt.Errorf("filesystem not supported: %s", f)
	}
}

func (f FileSystem) String() string {
	switch f {
	case Btrfs:
		return "btrfs"
	case Ext2:
		return "ext2"
	case Ext4:
		return "ext4"
	case XFS:
		return "xfs"
	case VFat:
		return "vfat"
	default:
		return Unknown
	}
}

func (f FileSystem) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.String())
}

func (f *FileSystem) UnmarshalJSON(data []byte) (err error) {
	var function string
	if err = json.Unmarshal(data, &function); err != nil {
		return err
	}

	*f, err = ParseFileSystem(function)
	return err
}

func ParseRole(function string) (PartRole, error) {
	switch function {
	case "efi":
		return EFI, nil
	case "system":
		return System, nil
	case "recovery":
		return Recovery, nil
	case "data":
		return Data, nil
	default:
		return PartRole(0), fmt.Errorf("unknown partition function: %s", function)
	}
}

func (p PartRole) String() string {
	switch p {
	case EFI:
		return "efi"
	case System:
		return "system"
	case Recovery:
		return "recovery"
	case Data:
		return "data"
	default:
		return Unknown
	}
}

func (p PartRole) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

func (p *PartRole) UnmarshalJSON(data []byte) (err error) {
	var role string
	if err = json.Unmarshal(data, &role); err != nil {
		return err
	}

	*p, err = ParseRole(role)
	return err
}

type RWVolume struct {
	Path          string   `json:"path"`
	Snapshotted   bool     `json:"snapshotted,omitempty"`
	NoCopyOnWrite bool     `json:"noCopyOnWrite,omitempty"`
	MountOpts     []string `json:"mountOpts,omitempty"`
}

type RWVolumes []RWVolume

type Partition struct {
	Label       string     `json:"label,omitempty"`
	FileSystem  FileSystem `json:"fileSystem,omitempty"`
	Size        MiB        `json:"size,omitempty"`
	Role        PartRole   `json:"role"`
	StartSector uint       `json:"startSector,omitempty"`
	MountPoint  string     `json:"mountPoint,omitempty"`
	MountOpts   []string   `json:"mountOpts,omitempty"`
	RWVolumes   RWVolumes  `json:"rwVolumes,omitempty"`
	UUID        string     `json:"uuid,omitempty"`
}

type Partitions []*Partition

type Disk struct {
	Device      string     `json:"device,omitempty"`
	Partitions  Partitions `json:"partitions"`
	StartSector uint       `json:"startSector,omitempty"`
}

// MarshalJSON on disks omits the device name as this is a runtime information
// which might not be consistent across reboots, there is no need to store it.
func (d Disk) MarshalJSON() ([]byte, error) {
	type diskAlias Disk
	disk := diskAlias(d)
	disk.Device = ""
	return json.Marshal(disk)
}

type Deployment struct {
	SourceOS *ImageSource `json:"sourceOS"`
	Disks    []*Disk      `json:"disks"`
	// Consider adding a systemd-sysext list here
	// All of them would extracted in the RO context, so only
	// additions to the RWVolumes would succeed.
	OverlayTree *ImageSource `json:"overlayTree"`
	CfgScript   string       `json:"configScript"`

	// Also bootloader details could be added here
}

// GetSnaphsottedVolumes returns a list of snapshotted rw volumes defined in the
// given partitions list.
func (p Partitions) GetSnaphsottedVolumes() RWVolumes {
	var volumes RWVolumes
	for _, part := range p {
		for _, rwVol := range part.RWVolumes {
			if rwVol.Snapshotted {
				volumes = append(volumes, rwVol)
			}
		}
	}
	return volumes
}

type SanitizeDeployment func(*sys.System, *Deployment) error

var sanitizers = []SanitizeDeployment{
	checkSystemPart, checkEFIPart, checkRecoveryPart,
	checkAllAvailableSize, checkDiskDeviceExists,
	checkPartitionsFS, checkRWVolumes,
}

// GetSystemPartition gets the data of the system partition.
// returns nil if not found
func (d Deployment) GetSystemPartition() *Partition {
	for _, disk := range d.Disks {
		for _, part := range disk.Partitions {
			if part.Role == System {
				return part
			}
		}
	}
	return nil
}

// GetSystemDisk gets the disk data including the system partition.
// returns nil if not found
func (d Deployment) GetSystemDisk() *Disk {
	for _, disk := range d.Disks {
		for _, part := range disk.Partitions {
			if part.Role == System {
				return disk
			}
		}
	}
	return nil
}

// Sanitize checks the consistency of the current Disk structure
func (d *Deployment) Sanitize(s *sys.System) error {
	for _, sanitize := range sanitizers {
		if err := sanitize(s, d); err != nil {
			return err
		}
	}
	return nil
}

func (d Deployment) WriteDeploymentFile(s *sys.System, root string) error {
	path := filepath.Join(root, DeploymentFile)
	if ok, _ := vfs.Exists(s.FS(), path); !ok {
		err := vfs.MkdirAll(s.FS(), filepath.Dir(path), vfs.DirPerm)
		if err != nil {
			s.Logger().Error("failed creating elemental directory")
			return err
		}
	} else {
		err := s.FS().Remove(path)
		if err != nil {
			s.Logger().Error("removing previous deployment file")
			return err
		}
	}

	data, err := yaml.Marshal(d)
	if err != nil {
		s.Logger().Error("failed marshalling deployment info")
		return err
	}

	dataStr := string(data)
	dataStr = "# self-generated content, do not edit\n\n" + dataStr

	err = s.FS().WriteFile(path, []byte(dataStr), 0444)
	if err != nil {
		s.Logger().Error("failed writing deployment file: %s", path)
		return err
	}
	return nil
}

func ReadDeployment(s *sys.System, root string) (*Deployment, error) {
	path := filepath.Join(root, DeploymentFile)
	if ok, err := vfs.Exists(s.FS(), path); !ok {
		s.Logger().Warn("deployment file not found '%s'", path)
		return nil, err
	}
	data, err := s.FS().ReadFile(path)
	if err != nil {
		s.Logger().Error("failed to read deployment file '%s'", path)
		return nil, err
	}
	d := &Deployment{}
	err = yaml.Unmarshal(data, d)
	if err != nil {
		s.Logger().Error("failed to unmarshal deployment file: %w", err.Error())
		return nil, err
	}
	return d, nil
}

// DefaultDeployment returns the simplest deployment setup in a single
// disk including only EFI and System partitions
func DefaultDeployment() *Deployment {
	return &Deployment{
		Disks: []*Disk{{
			Partitions: []*Partition{
				{
					Label:      EfiLabel,
					Role:       EFI,
					MountPoint: EfiMnt,
					FileSystem: VFat,
					Size:       EfiSize,
				}, {
					Label:      SystemLabel,
					Role:       System,
					MountPoint: SystemMnt,
					FileSystem: Btrfs,
					Size:       AllAvailableSize,
					RWVolumes: []RWVolume{
						{Path: "/var", NoCopyOnWrite: true},
						{Path: "/home"}, {Path: "/root"},
						{Path: "/opt"}, {Path: "/srv"},
						{Path: "/etc", Snapshotted: true},
					},
				},
			},
		}},
	}
}

// checkSystemPart verifies the system partition is properly defined and forces mandatory values
func checkSystemPart(s *sys.System, d *Deployment) error {
	var found bool
	for _, disk := range d.Disks {
		for _, part := range disk.Partitions {
			if part.Role == System && !found {
				found = true
				if part.FileSystem != Btrfs {
					s.Logger().Warn("filesystem types different to btrfs are not supported for the system partition")
					s.Logger().Info("system partition set to be formatted with btrfs")
					part.FileSystem = Btrfs
				}
				if part.MountPoint != SystemMnt {
					s.Logger().Warn("custom mountpoints for the system partition are not supported")
					s.Logger().Info("system partition mountpoint set to default '%s'", SystemMnt)
					part.MountPoint = SystemMnt
				}
				if part.Label == "" {
					part.Label = SystemLabel
				}
			} else if part.Role == System {
				return fmt.Errorf("multiple 'system' partitions defined, there must be only one")
			}
		}
	}
	if !found {
		return fmt.Errorf("no 'system' partition defined")
	}
	return nil
}

// checkEFIPart verifies the EFI partition is properly defined and forces mandatory values
func checkEFIPart(s *sys.System, d *Deployment) error {
	var found bool
	for _, disk := range d.Disks {
		for _, part := range disk.Partitions {
			if part.Role == EFI && !found {
				found = true
				if part.FileSystem != VFat {
					s.Logger().Warn("filesystem types different to vfat are not supported for the efi partition")
					s.Logger().Info("efi partition set to be formatted with vfat")
					part.FileSystem = VFat
				}
				if part.MountPoint != EfiMnt {
					s.Logger().Warn("custom mountpoints for the efi partition are not supported")
					s.Logger().Info("efi partition mountpoint set to default '%s'", EfiMnt)
					part.MountPoint = EfiMnt
				}
				if part.Label == "" {
					part.Label = EfiLabel
				}
				if part.Size < EfiSize {
					s.Logger().Warn("efi partition size cannot be less than %dMiB", EfiSize)
					s.Logger().Info("efi partition size set to %dMiB", EfiSize)
					part.Size = EfiSize
				}
				if len(part.RWVolumes) > 0 {
					s.Logger().Warn("efi partition does not support volumes")
					s.Logger().Info("cleared read-write volumes for efi")
					part.RWVolumes = []RWVolume{}
				}
			} else if part.Role == EFI {
				return fmt.Errorf("multiple 'efi' partitions defined, there must be only one")
			}
		}
	}
	if !found {
		return fmt.Errorf("no 'efi' partition defined")
	}
	return nil
}

// checkRecoveryPart verifies Recovery partition is properly defined if any
func checkRecoveryPart(s *sys.System, d *Deployment) error {
	var found bool
	for _, disk := range d.Disks {
		for _, part := range disk.Partitions {
			if part.Role == Recovery && !found {
				found = true
				if part.MountPoint != RecoveryMnt {
					s.Logger().Warn("custom mountpoints for the recovery partition are not supported")
					s.Logger().Info("recovery partition mountpoint set to defaults")
					part.MountPoint = RecoveryMnt
				}
				if len(part.RWVolumes) > 0 {
					s.Logger().Warn("recovery partition does not support volumes")
					s.Logger().Info("cleared read-write volumes for recovery")
					part.RWVolumes = []RWVolume{}
				}
				if part.FileSystem.String() == Unknown {
					part.FileSystem = Ext2
				}
			} else if part.Role == Recovery {
				return fmt.Errorf("multiple 'recovery' partitions defined, there can be only one")
			}
		}
	}
	return nil
}

// checkAllAvailableSize ensures only the last partition is eventually set to be as big as all
// available size in disk
func checkAllAvailableSize(_ *sys.System, d *Deployment) error {
	for _, disk := range d.Disks {
		pNum := len(disk.Partitions)
		for i, part := range disk.Partitions {
			if i < pNum-1 && part.Size == 0 {
				return fmt.Errorf("only last partition can be defined to be as big as available size in disk")
			}
		}
	}
	return nil
}

// checkDiskDeviceExists ensures the given device exists in the current host
func checkDiskDeviceExists(s *sys.System, d *Deployment) error {
	for _, disk := range d.Disks {
		if disk.Device == "" {
			return nil
		}
		ok, err := vfs.Exists(s.FS(), disk.Device)
		if err != nil {
			return fmt.Errorf("failed to check target device '%s' existence: %w", disk.Device, err)
		}
		if !ok {
			return fmt.Errorf("target device '%s' not found", disk.Device)
		}
	}
	return nil
}

// checkPartitionsFS ensures all partitions have a filesystem defined
func checkPartitionsFS(_ *sys.System, d *Deployment) error {
	for _, disk := range d.Disks {
		for _, part := range disk.Partitions {
			if part.FileSystem.String() == Unknown {
				part.FileSystem = Btrfs
			}
		}
	}
	return nil
}

// checkRWVolumes ensures all rw volumes are at a unique absolute path, not
// nested and defined on a btrfs partition
func checkRWVolumes(_ *sys.System, d *Deployment) error {
	pathMap := map[string]bool{}
	for _, disk := range d.Disks {
		for _, part := range disk.Partitions {
			if part.FileSystem != Btrfs && len(part.RWVolumes) > 0 {
				return fmt.Errorf("RW volumes are only supported in partitions formatted with btrfs")
			}
			for _, rwVol := range part.RWVolumes {
				if !filepath.IsAbs(rwVol.Path) {
					return fmt.Errorf("rw volume paths must be absolute")
				}
				if _, ok := pathMap[rwVol.Path]; !ok {
					pathMap[rwVol.Path] = true
					continue
				}
				return fmt.Errorf("rw volume paths must be unique. Duplicated '%s'", rwVol.Path)
			}
		}
	}

	paths := []string{}
	for k := range pathMap {
		paths = append(paths, k)
	}
	sort.Strings(paths)
	for i := range len(paths) - 1 {
		if strings.HasPrefix(paths[i+1], paths[i]) {
			return fmt.Errorf("nested rw volumes is not supported")
		}
	}
	return nil
}
