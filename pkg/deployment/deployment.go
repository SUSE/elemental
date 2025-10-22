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
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type MiB uint

const (
	EfiLabel     = "EFI"
	EfiMnt       = "/boot"
	EfiSize  MiB = 1024

	RecoveryLabel = "RECOVERY"
	RecoveryMnt   = "/run/elemental/recovery"
	RecoverySize  = 0

	SystemLabel          = "SYSTEM"
	SystemMnt            = "/"
	AllAvailableSize MiB = 0

	ConfigLabel = "ignition"
	ConfigMnt   = "/run/elemental/firstboot"

	deploymentFile = "/etc/elemental/deployment.yaml"

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

var (
	_ yaml.Marshaler   = FileSystem(0)
	_ yaml.Unmarshaler = (*FileSystem)(nil)
)

func (f FileSystem) MarshalYAML() (any, error) {
	if str := f.String(); str != Unknown {
		return str, nil
	}
	return nil, fmt.Errorf("unknown filesystem: %s", f)
}

func (f *FileSystem) UnmarshalYAML(data *yaml.Node) (err error) {
	var fs string
	if err = data.Decode(&fs); err != nil {
		return err
	}
	*f, err = ParseFileSystem(fs)
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

var (
	_ yaml.Marshaler   = PartRole(0)
	_ yaml.Unmarshaler = (*PartRole)(nil)
)

func (p PartRole) MarshalYAML() (any, error) {
	return p.String(), nil
}

func (p *PartRole) UnmarshalYAML(data *yaml.Node) (err error) {
	var role string
	if err = data.Decode(&role); err != nil {
		return err
	}

	*p, err = ParseRole(role)
	return err
}

type RWVolume struct {
	Path          string   `yaml:"path"`
	Snapshotted   bool     `yaml:"snapshotted,omitempty"`
	NoCopyOnWrite bool     `yaml:"noCopyOnWrite,omitempty"`
	MountOpts     []string `yaml:"mountOpts,omitempty"`
}

type RWVolumes []RWVolume

type Partition struct {
	Label       string     `yaml:"label,omitempty"`
	FileSystem  FileSystem `yaml:"fileSystem,omitempty"`
	Size        MiB        `yaml:"size,omitempty"`
	Role        PartRole   `yaml:"role"`
	StartSector uint       `yaml:"startSector,omitempty"`
	MountPoint  string     `yaml:"mountPoint,omitempty"`
	MountOpts   []string   `yaml:"mountOpts,omitempty"`
	RWVolumes   RWVolumes  `yaml:"rwVolumes,omitempty"`
	UUID        string     `yaml:"uuid,omitempty"`
	Hidden      bool       `yaml:"hidden,omitempty"`
}

type Partitions []*Partition

type Disk struct {
	Device      string     `yaml:"target,omitempty"`
	Partitions  Partitions `yaml:"partitions"`
	StartSector uint       `yaml:"startSector,omitempty"`
}

type BootConfig struct {
	Bootloader    string `yaml:"name"`
	KernelCmdline string `yaml:"kernelCmdline"`
}

type FirmwareConfig struct {
	BootEntries []*firmware.EfiBootEntry `yaml:"entries"`
}

type FipsConfig struct {
	Enabled bool `yaml:"enabled"`
}

type SnapshotterConfig struct {
	Name string `yaml:"name"`
}

type LiveInstaller struct {
	OverlayTree   *ImageSource `yaml:"overlayTree,omitempty"`
	CfgScript     string       `yaml:"configScript,omitempty"`
	KernelCmdline string       `yaml:"kernelCmdline,omitempty"`
}

type Deployment struct {
	SourceOS    *ImageSource       `yaml:"sourceOS"`
	Disks       []*Disk            `yaml:"disks"`
	Firmware    *FirmwareConfig    `yaml:"firmware"`
	BootConfig  *BootConfig        `yaml:"bootloader"`
	Fips        *FipsConfig        `yaml:"fips"`
	Snapshotter *SnapshotterConfig `yaml:"snapshotter"`
	OverlayTree *ImageSource       `yaml:"overlayTree,omitempty"`
	CfgScript   string             `yaml:"configScript,omitempty"`
	Installer   LiveInstaller      `yaml:"-"`
}

type Opt func(d *Deployment)

// GetSnapshottedVolumes returns a list of snapshotted rw volumes defined in the
// given partitions list.
func (p Partitions) GetSnapshottedVolumes() RWVolumes {
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
	checkAllAvailableSize, checkPartitionsFS, checkRWVolumes,
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

// GetEfiSystemPartition gets the data of the system partition.
// returns nil if not found
func (d Deployment) GetEfiSystemPartition() *Partition {
	for _, disk := range d.Disks {
		for _, part := range disk.Partitions {
			if part.Role == EFI {
				return part
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

// IsFipsEnabled returns true if FIPS is enabled for the deployment, otherwise false.
func (d *Deployment) IsFipsEnabled() bool {
	return d.Fips != nil && d.Fips.Enabled
}

// WriteDeploymentFile serialized the Deployment variable into a file. As part of the
// serialization it omits runtime information such as device paths, overlay and config
// script paths.
func (d Deployment) WriteDeploymentFile(s *sys.System, root string) error {
	path := filepath.Join(root, deploymentFile)
	if ok, _ := vfs.Exists(s.FS(), path); !ok {
		err := vfs.MkdirAll(s.FS(), filepath.Dir(path), vfs.DirPerm)
		if err != nil {
			return fmt.Errorf("creating elemental directory: %w", err)
		}
	} else {
		err := s.FS().Remove(path)
		if err != nil {
			return fmt.Errorf("removing previous deployment file: %w", err)
		}
	}

	data, err := yaml.Marshal(d)
	if err != nil {
		return fmt.Errorf("marshalling deployment: %w", err)
	}

	// Unmarshal it back for a deep copy
	dep := &Deployment{}
	_ = yaml.Unmarshal(data, dep)

	// omit the device name as this is a runtime information which might
	// not be consistent across reboots, there is no need to store it.
	for _, disk := range dep.Disks {
		disk.Device = ""
	}
	// omit the OverlayTree and CfgTree as this is a runtime information which might
	// not be consistent across reboots, there is no need to store it.
	dep.OverlayTree = nil
	dep.CfgScript = ""

	data, err = yaml.Marshal(dep)
	if err != nil {
		return fmt.Errorf("could not re-marshal deployment: %w", err)
	}

	dataStr := string(data)
	dataStr = "# self-generated content, do not edit\n\n" + dataStr

	err = s.FS().WriteFile(path, []byte(dataStr), 0444)
	if err != nil {
		return fmt.Errorf("writing deployment file '%s': %w", path, err)
	}
	return nil
}

// Parse reads a deployment yaml file from the given root and returns a
// Deployment object
func Parse(s *sys.System, root string) (*Deployment, error) {
	path := filepath.Join(root, deploymentFile)
	if ok, err := vfs.Exists(s.FS(), path); !ok {
		s.Logger().Warn("deployment file not found '%s'", path)
		return nil, err
	}
	data, err := s.FS().ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading deployment file '%s': %w", path, err)
	}
	d := &Deployment{}
	err = yaml.Unmarshal(data, d)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling deployment file '%s': %w: %s", path, err, string(data))
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
					MountOpts:  []string{"defaults", "x-systemd.automount"},
				}, {
					Label:      SystemLabel,
					Role:       System,
					MountPoint: SystemMnt,
					FileSystem: Btrfs,
					Size:       AllAvailableSize,
					MountOpts:  []string{"ro=vfs"},
					RWVolumes: []RWVolume{
						{Path: "/var", NoCopyOnWrite: true, MountOpts: []string{"x-initrd.mount"}},
						{Path: "/root", MountOpts: []string{"x-initrd.mount"}},
						{Path: "/etc", Snapshotted: true, MountOpts: []string{"x-initrd.mount"}},
						{Path: "/opt"}, {Path: "/srv"}, {Path: "/home"},
					},
				},
			},
		}},
		Firmware: &FirmwareConfig{},
		BootConfig: &BootConfig{
			Bootloader:    "none",
			KernelCmdline: fmt.Sprintf("root=LABEL=%s", SystemLabel),
		},
		Snapshotter: &SnapshotterConfig{
			Name: "snapper",
		},
	}
}

// New returns a new deployment based on the default setup with the given
// options applied on top.
func New(opts ...Opt) *Deployment {
	d := DefaultDeployment()

	for _, opt := range opts {
		opt(d)
	}
	return d
}

// WithPartitions inserts the given partitions to the default disk at the given
// position, where 0 is the first partition. Ignores out of range positions.
func WithPartitions(num int, parts ...*Partition) Opt {
	return func(d *Deployment) {
		disk := d.GetSystemDisk()
		if num >= 0 && num <= len(disk.Partitions) {
			disk.Partitions = slices.Insert(disk.Partitions, num, parts...)
		}
	}
}

// WithConfigParition inserts a configuration partition as the second partition
// to the systemd disk. The given size is the amount of data expected to store in
// the partition, then the partition is sized to be alined with 128MiB and to ensure
// at least 128MiB of free space is available.
func WithConfigPartition(size MiB) Opt {
	size = (size/128)*128 + 256
	part := &Partition{
		Label:      ConfigLabel,
		MountPoint: ConfigMnt,
		Role:       Data,
		FileSystem: Btrfs,
		Size:       size,
		Hidden:     true,
	}
	return WithPartitions(1, part)
}

// checkSystemPart verifies the system partition is properly defined and forces mandatory values
func checkSystemPart(s *sys.System, d *Deployment) error {
	var found bool
	for _, disk := range d.Disks {
		for _, part := range disk.Partitions {
			if part.Role == System && !found {
				found = true
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
