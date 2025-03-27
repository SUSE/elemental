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

package k8smounter

import (
	"github.com/suse/elemental/v3/pkg/sys/mounter"
	"k8s.io/mount-utils"
)

type Mounter struct {
	mnt mount.Interface
}

func NewMounter(binary string) mounter.Interface {
	return &Mounter{
		mnt: mount.NewWithoutSystemd(binary),
	}
}

func NewDummyMounter() *Mounter {
	return &Mounter{
		mnt: mount.NewFakeMounter([]mount.MountPoint{}),
	}
}

func (m Mounter) Mount(source string, target string, fstype string, options []string) error {
	return m.mnt.Mount(source, target, fstype, options)
}

func (m Mounter) Unmount(target string) error {
	return m.mnt.Unmount(target)
}

func (m Mounter) IsMountPoint(path string) (bool, error) {
	return m.mnt.IsMountPoint(path)
}

func (m Mounter) GetMountRefs(path string) ([]string, error) {
	return m.mnt.GetMountRefs(path)
}

func (m Mounter) GetMountPoints(device string) ([]mounter.MountPoint, error) {
	mntLst, err := m.mnt.List()
	if err != nil {
		return nil, err
	}
	var lst []mounter.MountPoint
	for _, mp := range mntLst {
		if mp.Device == device {
			lst = append(lst, mounter.MountPoint{
				Device: mp.Device,
				Path:   mp.Path,
				Opts:   mp.Opts,
				Type:   mp.Type,
			})
		}
	}
	return lst, nil
}
