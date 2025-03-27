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

package mounter

const (
	Binary = "/usr/bin/mount"
)

type Interface interface {
	Mount(source string, target string, fstype string, options []string) error
	Unmount(target string) error
	// IsMountPoint check /proc/mounts or equivalent data to check if the given path is listed there
	IsMountPoint(path string) (bool, error)
	// GetMountRefs finds all mount references to pathname, returning a slice of
	// paths. The returned slice does not include the given path.
	GetMountRefs(pathname string) ([]string, error)
	// GetMountPoints parses /proc/mounts or equivalent data to fetch all available mountpoints for the given device
	GetMountPoints(device string) ([]MountPoint, error)
}

// MountPoint represents a single line in /proc/mounts or /etc/fstab.
type MountPoint struct {
	Device string
	Path   string
	Type   string
	Opts   []string // Opts may contain sensitive mount options (like passwords) and MUST be treated as such (e.g. not logged).
}
