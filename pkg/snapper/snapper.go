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

	"github.com/joho/godotenv"
)

const (
	SnapshotsPath    = ".snapshots"
	SnapperInstaller = "/usr/lib/snapper/installation-helper"

	snapperDefaultconfig = "/etc/default/snapper"
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

func (sn Snapper) InitSnapperRootVolumes(root string) error {
	out, err := sn.s.Runner().Run(SnapperInstaller, "--root-prefix", root, "--step", "filesystem")
	if err != nil {
		sn.s.Logger().Error("failed initiating btrfs subvolumes to work with snapper: %s", strings.TrimSpace(string(out)))
		return err
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
		sn.s.Logger().Error("failed collecting snapshots: %s", string(cmdOut))
		return nil, err
	}
	return unmarshalSnapperList(cmdOut, config)
}

func (sn Snapper) FirstRootSnapshot(root string, metadata Metadata) (int, error) {
	sn.s.Logger().Debug("Creating first root filesystem as a snapshot")
	cmdOut, err := sn.s.Runner().Run(
		SnapperInstaller, "--root-prefix", root, "--step",
		"config", "--description", "first root filesystem, snapshot 1",
		"--userdata", metadata.String(),
	)
	if err != nil {
		sn.s.Logger().Error("failed creating initial snapshot: %s", strings.TrimSpace(string(cmdOut)))
		return 0, err
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
	if err != nil {
		sn.s.Logger().Error("failed creating config for '%s'", volumePath)
		return err
	}
	return nil
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
		sn.s.Logger().Error("snapper failed to create a new snapshot: %w", err)
		return 0, err
	}
	newSnap, err = strconv.Atoi(strings.TrimSpace(string(cmdOut)))
	if err != nil {
		sn.s.Logger().Error("failed parsing new snapshot ID")
		return 0, err
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
	if err != nil {
		sn.s.Logger().Error("snapper failed set snapshot permissions: %v", err)
		return err
	}
	return nil
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
	if err != nil {
		sn.s.Logger().Error("snapper failed to set default snapshot: %v", err)
		return err
	}
	return nil
}

func (sn Snapper) Cleanup(root string, maxSnaps int) error {
	// TODO instead of relaying on manual cleanup we could provide a snapper pluging
	// to handle cleanup and relay on 'snapper cleanup' command
	snaps, err := sn.ListSnapshots(root, rootConfig)
	if err != nil {
		sn.s.Logger().Error("cannot proceed with snapshots cleanup")
		return err
	}
	deletes := len(snaps) - maxSnaps
	i := 0
	for deletes > 0 {
		if !snaps[i].Active && !snaps[i].Default {
			path := filepath.Join(root, SnapshotsPath, strconv.Itoa(snaps[i].Number), "snapshot")
			err = sn.DeleteByPath(path)
			if err != nil {
				sn.s.Logger().Error("could not clean up snapshot %s", path)
				return err
			}
			deletes--
		}
		i++
	}
	return nil
}

// Delete removes the given snapshot path including any nested RO subvolume
func (sn Snapper) DeleteByPath(path string) error {
	// TODO instead of relaying on btrfs manual calls we could provide a snapper pluging
	// to handle deletion and cleanup
	err := btrfs.DeleteSubvolume(sn.s, path)
	if err != nil {
		sn.s.Logger().Error("failed deleting snapshot '%s'", path)
		return err
	}
	err = sn.s.FS().RemoveAll(filepath.Dir(path))
	if err != nil {
		sn.s.Logger().Error("failed deleting snapshot '%s' parent directory", path)
		return err
	}
	return nil
}

// ConfigureRoot sets the 'root' configuration for snapper
func (sn Snapper) ConfigureRoot(snapshotPath string, maxSnapshots int) error {
	defaultTmpl, err := vfs.FindFile(sn.s.FS(), snapshotPath, configTemplatesPaths()...)
	if err != nil {
		sn.s.Logger().Error("failed to find default snapper configuration template")
		return err
	}

	sysconfigData := map[string]string{}
	sysconfig := filepath.Join(snapshotPath, snapperDefaultconfig)
	if ok, _ := vfs.Exists(sn.s.FS(), sysconfig); !ok {
		sysconfig = filepath.Join(snapshotPath, snapperSysconfig)
	}

	if ok, _ := vfs.Exists(sn.s.FS(), sysconfig); ok {
		sysconfigData, err = loadEnvFile(sn.s.FS(), sysconfig)
		if err != nil {
			sn.s.Logger().Error("failed to load global snapper sysconfig")
			return err
		}
	}
	sysconfigData["SNAPPER_CONFIGS"] = rootConfig

	sn.s.Logger().Debug("Creating sysconfig snapper configuration at '%s'", sysconfig)
	err = writeEnvFile(sn.s.FS(), sysconfigData, sysconfig)
	if err != nil {
		sn.s.Logger().Error("failed writing snapper global configuration file: %v", err)
		return err
	}

	snapCfg, err := loadEnvFile(sn.s.FS(), defaultTmpl)
	if err != nil {
		sn.s.Logger().Error("failed to load default snapper templage configuration")
		return err
	}

	snapCfg["TIMELINE_CREATE"] = "no"
	snapCfg["QGROUP"] = "1/0"
	snapCfg["NUMBER_LIMIT"] = fmt.Sprintf("%d-%d", maxSnapshots/4, maxSnapshots)
	snapCfg["NUMBER_LIMIT_IMPORTANT"] = fmt.Sprintf("%d-%d", maxSnapshots/2, maxSnapshots)

	rootCfg := filepath.Join(snapshotPath, snapperRootConfig)
	sn.s.Logger().Debug("Creating 'root' snapper configuration at '%s'", rootCfg)
	err = writeEnvFile(sn.s.FS(), snapCfg, rootCfg)
	if err != nil {
		sn.s.Logger().Error("failed writing snapper root configuration file: %v", err)
		return err
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

// loadEnvFile will try to parse the file given and return a map with the key/values
func loadEnvFile(fs vfs.FS, file string) (map[string]string, error) {
	var envMap map[string]string
	var err error

	f, err := fs.Open(file)
	if err != nil {
		return envMap, err
	}
	defer f.Close()

	envMap, err = godotenv.Parse(f)
	if err != nil {
		return envMap, err
	}

	return envMap, err
}

// writeEnvFile will write the given environment file with the given key/values
func writeEnvFile(fs vfs.FS, envs map[string]string, filename string) error {
	var bkFile string

	rawPath, err := fs.RawPath(filename)
	if err != nil {
		return err
	}

	if ok, _ := vfs.Exists(fs, filename, true); ok {
		bkFile = filename + ".bk"
		err = fs.Rename(filename, bkFile)
		if err != nil {
			return err
		}
	}

	err = godotenv.Write(envs, rawPath)
	if err != nil {
		if bkFile != "" {
			// try to restore renamed file
			_ = fs.Rename(bkFile, filename)
		}
		return err
	}
	if bkFile != "" {
		_ = fs.Remove(bkFile)
	}
	return nil
}
