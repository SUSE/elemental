/*
Copyright © 2022-2026 SUSE LLC
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

package mock

import (
	"errors"
	"fmt"

	"github.com/suse/elemental/v3/pkg/sys/mounter"
	"k8s.io/mount-utils"
)

var _ mounter.Interface = (*Mounter)(nil)

// FakeMounter is a fake mounter for tests that can error out.
type Mounter struct {
	ErrorOnMount   bool
	ErrorOnUnmount bool
	FakeMounter    mount.Interface
}

// NewFakeMounter returns an FakeMounter with an instance of FakeMounter inside so we can use its functions
func NewMounter() *Mounter {
	return &Mounter{
		FakeMounter: &mount.FakeMounter{},
	}
}

// Mount will return an error if ErrorOnMount is true
func (e Mounter) Mount(source string, target string, fstype string, options []string) error {
	if e.ErrorOnMount {
		return errors.New("mount error")
	}
	return e.FakeMounter.Mount(source, target, fstype, options)
}

// Unmount will return an error if ErrorOnUnmount is true
func (e Mounter) Unmount(target string) error {
	if e.ErrorOnUnmount {
		return errors.New("unmount error")
	}
	return e.FakeMounter.Unmount(target)
}

func (e Mounter) IsMountPoint(file string) (bool, error) {
	mnts, err := e.List()
	if err != nil {
		return false, err
	}

	for _, mnt := range mnts {
		if file == mnt.Path {
			return true, nil
		}
	}
	return false, nil
}

func (e Mounter) GetMountRefs(pathname string) ([]string, error) {
	var device string
	mntPaths := []string{}

	mnts, _ := e.List()
	for _, mnt := range mnts {
		if pathname == mnt.Path {
			device = mnt.Device
			break
		}
	}
	if device == "" {
		return mntPaths, fmt.Errorf("no mountpoint found for '%s'", pathname)
	}
	for _, mnt := range mnts {
		if device == mnt.Device && pathname != mnt.Path {
			mntPaths = append(mntPaths, mnt.Path)
		}
	}
	return mntPaths, nil
}

func (e Mounter) GetMountPoints(device string) ([]mounter.MountPoint, error) {
	lst, err := e.List()
	if err != nil {
		return nil, err
	}
	var mntLst []mounter.MountPoint
	for _, mnt := range lst {
		if device == mnt.Device {
			mntLst = append(mntLst, mnt)
		}
	}
	return mntLst, nil
}

func (e Mounter) List() ([]mounter.MountPoint, error) {
	lst, err := e.FakeMounter.List()
	if err != nil {
		return nil, err
	}
	var mntList []mounter.MountPoint
	for _, mnt := range lst {
		mntList = append(mntList, mounter.MountPoint{
			Device: mnt.Device,
			Path:   mnt.Path,
			Type:   mnt.Type,
			Opts:   mnt.Opts,
		})
	}
	return mntList, nil
}
