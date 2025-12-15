/*
Copyright © 2025-2026 SUSE LLC
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

package repart

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/suse/elemental/v3/pkg/block/lsblk"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	// Recognized identifier types by systemd-repart based on UAPI's Discoverable Partitions Specification (DPS)
	rootArchType = "root-%s"
	genericType  = "linux-generic"
	espType      = "esp"

	// Custom types defined by Elemental as none of the predefined types is a clear match to those partition roles
	// Do not change these values as this could break backward compatibility on already installed systems (e.g. reseting a system)
	configType   = "2ecf8b13-6846-4e8a-9bc3-284ff5e2ac22"
	recoveryType = "3265f37b-3105-4777-bd97-cfcd9cc7cf99"
)

//go:embed templates/partition.conf.tpl
var partTpl []byte

type Partition struct {
	Partition *deployment.Partition
	// CopyFiles is list of paths to copy into the partition, uses CopyFiles syntax as defined
	// in repart.d(5) man pages
	CopyFiles []string
	// Excludes is a list of paths to exclude from the host to be copied into the partition, uses
	// ExcludeFiles syntax as defined in repart.d(5) man pages
	Excludes []string
}

// PartitionAndFormatDevice creates a new empty partition table on target disk
// and applies the configured disk layout by creating and formatting all
// required partitions.
func PartitionAndFormatDevice(s *sys.System, d *deployment.Disk) error {
	err := repartDisk(s, d, "force")
	if err != nil {
		return fmt.Errorf("failed creating the new partition table: %w", err)
	}

	notifyKernel(s, d.Device)
	return nil
}

// ReconcileDevicePartitions attempts to match the given disk layout with the current device.
// It attempts to extend an existing partition table or create a new one if none exists. It does not
// remove any pre-existing partition.
func ReconcileDevicePartitions(s *sys.System, d *deployment.Disk) error {
	err := repartDisk(s, d, "allow")
	if err != nil {
		return fmt.Errorf("failed updating the current partition table: %w", err)
	}

	notifyKernel(s, d.Device)
	return nil
}

// CreateDiskImage creates a disk image file with the given size and partitions
func CreateDiskImage(s *sys.System, filename string, size deployment.MiB, partitions []Partition) error {
	s.Logger().Info("Partitioning image '%s'", filename)

	var sizeFlag string
	if size == 0 {
		sizeFlag = "--size=auto"
	} else {
		sizeFlag = fmt.Sprintf("--size=%dM", size)
	}
	flags := []string{"--empty=create", sizeFlag}
	return runSystemdRepart(s, filename, partitions, flags...)
}

// CreatePartitionConfFile writes a partition configuration for systemd-repart for the given partition into the given file
func CreatePartitionConfFile(s *sys.System, filename string, p Partition) error {
	file, err := s.FS().Create(filename)
	if err != nil {
		return fmt.Errorf("failed creating systemd-repart configuration file '%s': %w", filename, err)
	}
	err = CreatePartitionConf(s, file, p)
	if err != nil {
		return fmt.Errorf("failed generation of '%s' systemd-repart configuration file: %w", filename, err)
	}
	err = file.Close()
	if err != nil {
		return fmt.Errorf("failed closing systemd-repart configuration file '%s': %w", filename, err)
	}
	return nil
}

// CreatePartitionConf writes a partition configuration for systemd-repart for the given partition into the given io.Writer
func CreatePartitionConf(s *sys.System, wr io.Writer, p Partition) error {
	pType := roleToType(s, p.Partition.Role)
	if pType == deployment.Unknown {
		return fmt.Errorf("invalid partition role: %s", p.Partition.Role.String())
	}

	for _, copy := range p.CopyFiles {
		path := strings.Split(copy, ":")[0]
		if path != "" && !filepath.IsAbs(path) {
			return fmt.Errorf("requires an absolute path to copy files from, given path is '%s'", p.CopyFiles)
		}
	}

	values := struct {
		Type      string
		Format    string
		Size      deployment.MiB
		Label     string
		UUID      string
		CopyFiles []string
		Excludes  []string
		ReadOnly  string
	}{
		Type:      pType,
		Format:    fileSystemToFormat(p.Partition.FileSystem),
		Size:      p.Partition.Size,
		Label:     p.Partition.Label,
		UUID:      p.Partition.UUID,
		CopyFiles: p.CopyFiles,
		Excludes:  p.Excludes,
		ReadOnly:  readOnlyPart(p.Partition),
	}

	partCfg := template.New("partition")
	partCfg = template.Must(partCfg.Parse(string(partTpl)))
	err := partCfg.Execute(wr, values)
	if err != nil {
		return fmt.Errorf("failed parsing systemd-repart partition template: %w", err)
	}
	return nil
}

// notifyKernel asks the kernel to reread the partition table. It is just a best effort call, does not return error.
// In recent versions of systemd-repart this step is already performed by the tool, however, as of today this is required
// for GH public runners (November 2025)
func notifyKernel(s *sys.System, device string) {
	_, _ = s.Runner().Run("partx", "-u", device)
	_, _ = s.Runner().Run("udevadm", "settle")
}

// repartDisk generates the systemd-repart configuration according to the given disk and runs systemd-repart with the given
// empty flag.
func repartDisk(s *sys.System, d *deployment.Disk, empty string) (err error) {
	lsblkWrapper := lsblk.NewLsDevice(s)
	sSize, err := lsblkWrapper.GetDeviceSectorSize(d.Device)
	if err != nil {
		return err
	}

	parts := make([]Partition, len(d.Partitions))
	for i, part := range d.Partitions {
		parts[i] = Partition{Partition: part}
	}

	flags := []string{
		fmt.Sprintf("--empty=%s", empty), fmt.Sprintf("--sector-size=%d", sSize),
	}
	return runSystemdRepart(s, d.Device, parts, flags...)
}

// runSystemdRepart runs systemd-repart for the given partitions and target device. It appends to the generated command the
// the optional given flags. On success it parses systemd-repart output to get the generated partition UUIDs and update the
// given partitions list with them.
func runSystemdRepart(s *sys.System, target string, parts []Partition, flags ...string) error {
	dir, err := vfs.TempDir(s.FS(), "", "elemental-repart.d")
	if err != nil {
		return fmt.Errorf("failed creating a temporary directory for systemd-repart configuration: %w", err)
	}
	defer func() {
		nErr := s.FS().RemoveAll(dir)
		if err == nil && nErr != nil {
			err = nErr
		}
	}()

	partsMap := map[string]*deployment.Partition{}
	for i, part := range parts {
		if part.Partition == nil {
			return fmt.Errorf("cannot configure a nil partition")
		}

		partConf := filepath.Join(dir, fmt.Sprintf("%d-%s.conf", i, part.Partition.Role.String()))
		err = CreatePartitionConfFile(s, partConf, part)
		if err != nil {
			return fmt.Errorf("failed generation of '%s' systemd-repart configuration file: %w", partConf, err)
		}
		partsMap[partConf] = part.Partition
	}

	args := []string{"--json=pretty", fmt.Sprintf("--definitions=%s", dir), "--dry-run=no"}
	reg := regexp.MustCompile(`(--json|--definitions|--dry-run)`)
	for _, flag := range flags {
		if reg.MatchString(flag) {
			return fmt.Errorf("json, definitions and dry-run flags are not configurable by repart.runSystemdRepart method")
		}
		args = append(args, flag)
	}
	args = append(args, target)

	out, err := s.Runner().RunEnv("systemd-repart", []string{"PATH=/sbin:/usr/sbin:/usr/bin:/bin"}, args...)
	if err != nil {
		return fmt.Errorf("failed partitioning disk '%s' with systemd-repart: %w", target, err)
	}
	uuids := []struct {
		UUID string `json:"uuid,omitempty"`
		File string `json:"file,omitempty"`
	}{}

	s.Logger().Debug("systemd-repart output to parse:\n%s", string(out))
	err = json.Unmarshal(out, &uuids)
	if err != nil {
		return fmt.Errorf("failed parsing systemd-repart JSON output: %w", err)
	}

	for _, uuid := range uuids {
		// Pre-existing partitions and not necessarily listed in the repart configuration, ignore
		// unmatched partitions
		if uuid.File == "" {
			continue
		}
		part := partsMap[uuid.File]
		if part == nil {
			return fmt.Errorf("matching partitions and systemd-repart JSON output")
		}
		partsMap[uuid.File].UUID = uuid.UUID
	}
	return nil
}

func roleToType(s *sys.System, role deployment.PartRole) string {
	switch role {
	case deployment.Generic:
		return genericType
	case deployment.EFI:
		return espType
	case deployment.System:
		return fmt.Sprintf(rootArchType, s.Platform().Arch)
	case deployment.Recovery:
		return recoveryType
	case deployment.Config:
		return configType
	default:
		return deployment.Unknown
	}
}

func fileSystemToFormat(f deployment.FileSystem) string {
	switch {
	case f.String() == deployment.Unknown:
		return ""
	default:
		return f.String()
	}
}

func readOnlyPart(part *deployment.Partition) string {
	for _, opt := range part.MountOpts {
		if strings.HasPrefix(opt, "ro") {
			return "on"
		}
	}
	return ""
}
