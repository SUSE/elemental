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
	rootType = "root"
	dataType = "linux-generic"
	espType  = "esp"
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
func PartitionAndFormatDevice(s *sys.System, d *deployment.Disk) (err error) {
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
		"--empty=force", fmt.Sprintf("--sector-size=%d", sSize),
	}
	return runSystemdRepart(s, d.Device, parts, flags...)
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
	err = CreatePartitionConf(file, p)
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
func CreatePartitionConf(wr io.Writer, p Partition) error {
	pType := roleToType(p.Partition.Role)
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

	for i, part := range parts {
		if part.Partition == nil {
			return fmt.Errorf("cannot configure a nil partition")
		}
		partConf := fmt.Sprintf("%d-%s.conf", i, part.Partition.Role.String())
		err = CreatePartitionConfFile(s, filepath.Join(dir, partConf), part)
		if err != nil {
			return fmt.Errorf("failed generation of '%s' systemd-repart configuration file: %w", partConf, err)
		}
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
	s.Logger().Debug("systemd-repart output:\n%s", string(out))
	if err != nil {
		return fmt.Errorf("failed partitioning disk '%s' with systemd-repart: %w", target, err)
	}
	uuids := []struct {
		UUID    string `json:"uuid,omitempty"`
		PartNum uint   `json:"partno,omitempty"`
	}{}

	err = json.Unmarshal(out, &uuids)
	if err != nil || len(uuids) != len(parts) {
		return fmt.Errorf("failed parsing systemd-repart JSON output: %w", err)
	}

	for _, uuid := range uuids {
		parts[uuid.PartNum].Partition.UUID = uuid.UUID
	}
	return nil
}

func roleToType(role deployment.PartRole) string {
	switch role {
	case deployment.Data, deployment.Recovery:
		return dataType
	case deployment.EFI:
		return espType
	case deployment.System:
		return rootType
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
