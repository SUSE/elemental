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

// PartitionAndFormatDevice creates a new empty partition table on target disk
// and applies the configured disk layout by creating and formatting all
// required partitions.
func PartitionAndFormatDevice(s *sys.System, d *deployment.Disk) (err error) {
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

	s.Logger().Info("Creating systemd-repart configuration at %s", dir)

	lsblkWrapper := lsblk.NewLsDevice(s)
	sSize, err := lsblkWrapper.GetDeviceSectorSize(d.Device)
	if err != nil {
		return err
	}

	for i, part := range d.Partitions {
		partConf := fmt.Sprintf("%d-%s.conf", i, part.Role.String())
		file, err := s.FS().Create(filepath.Join(dir, partConf))
		if err != nil {
			return fmt.Errorf("failed creating systemd-repart configuration file '%s': %w", partConf, err)
		}
		err = CreatePartitionConf(file, *part, "")
		if err != nil {
			return fmt.Errorf("failed generation of '%s' systemd-repart configuration file: %w", partConf, err)
		}
		err = file.Close()
		if err != nil {
			return fmt.Errorf("failed closing systemd-repart configuration file '%s': %w", partConf, err)
		}
	}

	s.Logger().Info("Partitioning device '%s'", d.Device)
	args := []string{
		"--empty=force", "--json=pretty", fmt.Sprintf("--definitions=%s", dir),
		"--dry-run=no", fmt.Sprintf("--sector-size=%d", sSize), d.Device,
	}
	out, err := s.Runner().RunEnv("systemd-repart", []string{"PATH=/sbin:/usr/sbin:/usr/bin:/bin"}, args...)
	s.Logger().Debug("systemd-repart output:\n%s", string(out))
	if err != nil {
		return fmt.Errorf("failed partitioning disk '%s' with systemd-repart: %w", d.Device, err)
	}
	uuids := []struct {
		UUID    string `json:"uuid,omitempty"`
		PartNum uint   `json:"partno,omitempty"`
	}{}

	err = json.Unmarshal(out, &uuids)
	if err != nil || len(uuids) != len(d.Partitions) {
		return fmt.Errorf("failed parsing systemd-repart JSON output: %w", err)
	}

	for _, uuid := range uuids {
		d.Partitions[uuid.PartNum].UUID = uuid.UUID
	}

	// Notify kernel of partition table changes, swallows errors, just a best effort call
	_, _ = s.Runner().Run("partx", "-u", d.Device)
	_, _ = s.Runner().Run("udevadm", "settle")
	return nil
}

// CreatePartitionConf writes a partition configuration for systemd-repart for the given partition
func CreatePartitionConf(wr io.Writer, part deployment.Partition, copyFiles string) error {
	pType := roleToType(part.Role)
	if pType == deployment.Unknown {
		return fmt.Errorf("invalid partition role: %s", part.Role.String())
	}

	if copyFiles != "" && !filepath.IsAbs(copyFiles) {
		return fmt.Errorf("requires an absolute path to copy files from, given path is '%s'", copyFiles)
	}

	values := struct {
		Type      string
		Format    string
		Size      deployment.MiB
		Label     string
		UUID      string
		CopyFiles string
	}{
		Type:      pType,
		Format:    fileSystemToFormat(part.FileSystem),
		Size:      part.Size,
		Label:     part.Label,
		UUID:      part.UUID,
		CopyFiles: copyFiles,
	}

	partCfg := template.New("partition")
	partCfg = template.Must(partCfg.Parse(string(partTpl)))
	err := partCfg.Execute(wr, values)
	if err != nil {
		return fmt.Errorf("failed parsing systemd-repart partition template: %w", err)
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
