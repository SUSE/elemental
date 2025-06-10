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

package snapper

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/suse/elemental/v3/pkg/btrfs"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	SnapshotsPath = ".snapshots"
	Installer     = "/usr/lib/snapper/installation-helper"

	snapperDefaultConfig = "/etc/default/snapper"
	snapperSysconfig     = "/etc/sysconfig/snapper"
	snapperRootConfig    = "/etc/snapper/configs/" + rootConfig
	rootConfig           = "root"
)

type Snapper struct {
	s *sys.System
}

type Snapshot struct {
	Number   int      `json:"number"`
	Default  bool     `json:"default"`
	Active   bool     `json:"active"`
	UserData Metadata `json:"userdata,omitempty"`
}

type Metadata map[string]string

type Snapshots []*Snapshot

func (s Snapshots) GetDefault() int {
	for _, snap := range s {
		if snap.Default {
			return snap.Number
		}
	}
	return 0
}

func (s Snapshots) GetActive() int {
	for _, snap := range s {
		if snap.Active {
			return snap.Number
		}
	}
	return 0
}

func (s Snapshots) GetWithUserdata(key, value string) []int {
	ids := []int{}
	for _, snap := range s {
		if snap.UserData != nil && snap.UserData[key] == value {
			ids = append(ids, snap.Number)
		}
	}
	return ids
}

func (m Metadata) String() string {
	var str string
	for k, v := range m {
		str += fmt.Sprintf("%s=%s,", k, v)
	}
	return strings.TrimSuffix(str, ",")
}

func configTemplatesPaths() []string {
	return []string{
		"/etc/snapper/config-templates/default",
		"/usr/share/snapper/config-templates/default",
	}
}

func ConfigName(path string) string {
	if path == "/" || path == "" {
		return rootConfig
	}
	return strings.ReplaceAll(strings.Trim(path, "/"), "/", "_")
}

func New(s *sys.System) *Snapper {
	return &Snapper{s: s}
}

func (sn Snapper) InitRootVolumes(root string) error {
	out, err := sn.s.Runner().Run(Installer, "--root-prefix", root, "--step", "filesystem")
	if err != nil {
		return fmt.Errorf("initiating btrfs subvolumes: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (sn Snapper) ListSnapshots(root string, config string) (Snapshots, error) {
	args := []string{"--no-dbus"}
	if root != "" && root != "/" {
		args = append(args, "--root", root)
	}
	if config == "" {
		config = root
	}
	args = append(args, "-c", config, "--jsonout", "list", "--columns", "number,default,active,userdata")
	cmdOut, err := sn.s.Runner().Run("snapper", args...)
	if err != nil {
		return nil, fmt.Errorf("collecting snapshots: %s: %w", string(cmdOut), err)
	}

	snapshots, err := unmarshalSnapperList(cmdOut, config)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling snapshots: %w", err)
	}

	return snapshots, nil
}

func (sn Snapper) FirstRootSnapshot(root string, metadata Metadata) (int, error) {
	sn.s.Logger().Debug("Creating first root filesystem as a snapshot")
	cmdOut, err := sn.s.Runner().Run(
		Installer, "--root-prefix", root, "--step",
		"config", "--description", "first root filesystem, snapshot 1",
		"--userdata", metadata.String(),
	)
	if err != nil {
		return 0, fmt.Errorf("creating initial snapshot: %s: %w", strings.TrimSpace(string(cmdOut)), err)
	}
	return 1, nil
}

func (sn Snapper) CreateConfig(root, volumePath string) error {
	err := sn.s.FS().RemoveAll(filepath.Join(volumePath, SnapshotsPath))
	if err != nil {
		return err
	}
	conf := ConfigName(volumePath)
	args := []string{"--no-dbus"}
	if root != "" && root != "/" {
		args = append(args, "--root", root)
	}
	args = append(args, "-c", conf, "create-config", "--fstype", "btrfs", volumePath)
	_, err = sn.s.Runner().Run("snapper", args...)
	return err
}

// CreateSnapshot creates a new snapper snapshot by calling "snapper create"
func (sn Snapper) CreateSnapshot(root string, config string, base int, rw bool, description string, metadata Metadata) (int, error) {
	var newSnap int
	args := []string{"LC_ALL=C", "snapper", "--no-dbus"}

	if root != "" && root != "/" {
		args = append(args, "--root", root)
	}
	if config == "" {
		config = rootConfig
	}
	args = append(args, "-c", config, "create", "--print-number", "-c", "number")
	if len(metadata) > 0 {
		args = append(args, "--userdata", metadata.String())
	}
	if description != "" {
		args = append(args, "--description", description)
	}
	if rw {
		args = append(args, "--read-write")
	}
	if base > 0 {
		args = append(args, "--from", strconv.Itoa(base))
	}

	sn.s.Logger().Info("Creating a new snapshot")
	cmdOut, err := sn.s.Runner().Run("env", args...)
	if err != nil {
		return 0, fmt.Errorf("creating a new snapshot: %w", err)
	}
	newSnap, err = strconv.Atoi(strings.TrimSpace(string(cmdOut)))
	if err != nil {
		return 0, fmt.Errorf("parsing new snapshot ID: %w", err)
	}

	return newSnap, nil
}

func (sn Snapper) SetPermissions(root string, id int, rw bool) error {
	args := []string{"--no-dbus"}

	if root != "" && root != "/" {
		args = append(args, "--root", root)
	}
	args = append(args, "modify")
	if rw {
		args = append(args, "--read-write")
	} else {
		args = append(args, "--read-only")
	}
	args = append(args, strconv.Itoa(id))
	sn.s.Logger().Info("Setting permissions to snapshot")
	_, err := sn.s.Runner().Run("snapper", args...)
	return err
}

func (sn Snapper) SetDefault(root string, id int, metadata Metadata) error {
	args := []string{"--no-dbus"}

	if root != "" && root != "/" {
		args = append(args, "--root", root)
	}
	args = append(args, "modify", "--default")
	if len(metadata) > 0 {
		args = append(args, "--userdata", metadata.String())
	}
	args = append(args, strconv.Itoa(id))
	sn.s.Logger().Info("Setting default snapshot")
	_, err := sn.s.Runner().Run("snapper", args...)
	return err
}

func (sn Snapper) Cleanup(root string, maxSnaps int) error {
	// TODO instead of relying on manual cleanup we could provide a snapper plugin
	// to handle cleanup and rely on 'snapper cleanup' command
	snaps, err := sn.ListSnapshots(root, rootConfig)
	if err != nil {
		return fmt.Errorf("listing snapshots: %w", err)
	}
	deletes := len(snaps) - maxSnaps
	i := 0
	for deletes > 0 {
		if !snaps[i].Active && !snaps[i].Default {
			path := filepath.Join(root, SnapshotsPath, strconv.Itoa(snaps[i].Number), "snapshot")
			err = sn.DeleteByPath(path)
			if err != nil {
				return fmt.Errorf("cleaning up snapshot '%s': %w", path, err)
			}
			deletes--
		}
		i++
	}
	return nil
}

// DeleteByPath removes the given snapshot path including any nested RO subvolume
func (sn Snapper) DeleteByPath(path string) error {
	// TODO instead of relying on manual btrfs calls we could provide a snapper plugin
	// to handle deletion and cleanup
	err := btrfs.DeleteSubvolume(sn.s, path)
	if err != nil {
		return fmt.Errorf("deleting subvolume: %w", err)
	}
	err = sn.s.FS().RemoveAll(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("removing snapshot parent directory: %w", err)
	}
	return nil
}

// ConfigureRoot sets the 'root' configuration for snapper
func (sn Snapper) ConfigureRoot(snapshotPath string, maxSnapshots int) error {
	defaultTmpl, err := vfs.FindFile(sn.s.FS(), snapshotPath, configTemplatesPaths()...)
	if err != nil {
		return fmt.Errorf("finding default snapper configuration template: %w", err)
	}

	sysconfigData := map[string]string{}
	sysconfig := filepath.Join(snapshotPath, snapperDefaultConfig)
	if ok, _ := vfs.Exists(sn.s.FS(), sysconfig); !ok {
		sysconfig = filepath.Join(snapshotPath, snapperSysconfig)
	}

	if ok, _ := vfs.Exists(sn.s.FS(), sysconfig); ok {
		sysconfigData, err = vfs.LoadEnvFile(sn.s.FS(), sysconfig)
		if err != nil {
			return fmt.Errorf("loading global snapper sysconfig: %w", err)
		}
	}
	sysconfigData["SNAPPER_CONFIGS"] = rootConfig

	sn.s.Logger().Debug("Creating sysconfig snapper configuration at '%s'", sysconfig)
	err = vfs.WriteEnvFile(sn.s.FS(), sysconfigData, sysconfig)
	if err != nil {
		return fmt.Errorf("writing global snapper configuration: %w", err)
	}

	snapCfg, err := vfs.LoadEnvFile(sn.s.FS(), defaultTmpl)
	if err != nil {
		return fmt.Errorf("loading default snapper configuration template: %w", err)
	}

	snapCfg["TIMELINE_CREATE"] = "no"
	snapCfg["QGROUP"] = "1/0"
	snapCfg["NUMBER_LIMIT"] = fmt.Sprintf("%d-%d", maxSnapshots/4, maxSnapshots)
	snapCfg["NUMBER_LIMIT_IMPORTANT"] = fmt.Sprintf("%d-%d", maxSnapshots/2, maxSnapshots)

	rootCfg := filepath.Join(snapshotPath, snapperRootConfig)
	sn.s.Logger().Debug("Creating 'root' snapper configuration at '%s'", rootCfg)
	err = vfs.WriteEnvFile(sn.s.FS(), snapCfg, rootCfg)
	if err != nil {
		return fmt.Errorf("writing snapper root configuration: %w", err)
	}
	return nil
}

func unmarshalSnapperList(snapperOut []byte, config string) (Snapshots, error) {
	var objmap map[string]*json.RawMessage
	err := json.Unmarshal(snapperOut, &objmap)
	if err != nil {
		return nil, err
	}

	if _, ok := objmap[config]; !ok {
		return nil, fmt.Errorf("invalid json object, no '%s' key found", config)
	}

	var snaps Snapshots
	err = json.Unmarshal(*objmap[config], &snaps)
	if err != nil {
		return nil, err
	}

	// Skip snapshot 0 from the list
	var snapshots Snapshots
	for _, snap := range snaps {
		if snap.Number == 0 {
			continue
		}
		snapshots = append(snapshots, snap)
	}

	return snapshots, nil
}
