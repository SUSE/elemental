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

package chroot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/mounter"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

// Chroot represents the struct that will allow us to run commands inside a given chroot
type Chroot struct {
	path          string
	defaultMounts []string
	extraMounts   map[string]string
	activeMounts  []string
	touchedFiles  []string
	fs            vfs.FS
	mounter       mounter.Interface
	logger        log.Logger
	runner        sys.Runner
	syscall       sys.Syscall
}

type Opts func(c *Chroot)

func NewChroot(s *sys.System, path string, opts ...Opts) *Chroot {
	c := &Chroot{
		path:          path,
		defaultMounts: []string{"/dev", "/dev/pts", "/proc", "/sys"},
		extraMounts:   map[string]string{},
		activeMounts:  []string{},
		touchedFiles:  []string{},
		runner:        s.Runner(),
		logger:        s.Logger(),
		mounter:       s.Mounter(),
		fs:            s.FS(),
		syscall:       s.Syscall(),
	}

	for _, o := range opts {
		o(c)
	}

	return c
}

func WithoutDefaultBinds() Opts {
	return func(c *Chroot) {
		c.defaultMounts = []string{}
	}
}

// ChrootedCallback runs the given callback in a chroot environment
func ChrootedCallback(s *sys.System, path string, bindMounts map[string]string, callback func() error, opts ...Opts) error {
	chroot := NewChroot(s, path, opts...)
	if bindMounts == nil {
		bindMounts = map[string]string{}
	}
	chroot.SetExtraMounts(bindMounts)
	return chroot.RunCallback(callback)
}

// Sets additional bind mounts for the chroot enviornment. They are represented
// in a map where the key is the path outside the chroot and the value is the
// path inside the chroot.
func (c *Chroot) SetExtraMounts(extraMounts map[string]string) {
	c.extraMounts = extraMounts
}

// Prepare will mount the defaultMounts as bind mounts, to be ready when we run chroot
func (c *Chroot) Prepare() (err error) {
	keys := []string{}

	if len(c.activeMounts) > 0 {
		return fmt.Errorf("there are already active mountpoints for this instance")
	}

	defer func() {
		if err != nil {
			_ = c.Close()
		}
	}()

	for _, mnt := range c.defaultMounts {
		err = c.bindMount(mnt, filepath.Join(c.path, mnt))
		if err != nil {
			return err
		}
	}

	for k := range c.extraMounts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		err = c.bindMount(k, filepath.Join(c.path, c.extraMounts[k]))
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Chroot) bindMount(source, mountPoint string) error {
	info, err := c.fs.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return c.bindMountDir(source, mountPoint)
	}
	return c.bindMountFile(source, mountPoint)
}

func (c *Chroot) bindMountDir(source, mountPoint string) error {
	err := vfs.MkdirAll(c.fs, mountPoint, vfs.DirPerm)
	if err != nil {
		return err
	}
	c.logger.Debug("Mounting %s to chroot", mountPoint)
	err = c.mounter.Mount(source, mountPoint, "", []string{"bind"})
	if err != nil {
		return err
	}
	c.activeMounts = append(c.activeMounts, mountPoint)
	return nil
}

func (c *Chroot) bindMountFile(source, target string) error {
	ok, err := vfs.Exists(c.fs, target)
	if err != nil {
		return err
	}
	if !ok {
		err = c.fs.WriteFile(target, []byte{}, vfs.FilePerm)
		if err != nil {
			return err
		}
		c.touchedFiles = append(c.touchedFiles, target)
	}
	c.logger.Debug("Mounting %s to chroot", target)
	err = c.mounter.Mount(source, target, "", []string{"bind"})
	if err != nil {
		return err
	}
	c.activeMounts = append(c.activeMounts, target)
	return nil
}

// Close will unmount all active mounts created in Prepare on reverse order
func (c *Chroot) Close() (err error) {
	uFailures := []string{}
	// syncing before unmounting chroot paths as it has been noted that on
	// empty, trivial or super fast callbacks unmounting fails with a device busy error.
	// Having lazy unmount could also fix it, but continuing without being sure they were
	// really unmounted is dangerous.
	_, _ = c.runner.Run("sync")
	slices.Reverse(c.activeMounts)
	for _, mnt := range c.activeMounts {
		c.logger.Debug("Unmounting %s from chroot", mnt)
		e := c.mounter.Unmount(mnt)
		if e != nil {
			uFailures = append(uFailures, mnt)
			err = errors.Join(err, fmt.Errorf("unmounting %s: %w", mnt, e))
			continue
		}
		if i := slices.Index(c.touchedFiles, mnt); i >= 0 {
			e = c.fs.Remove(mnt)
			if e != nil {
				err = errors.Join(err, fmt.Errorf("removing %s: %w", mnt, e))
			}
			c.touchedFiles = slices.Delete(c.touchedFiles, i, i)
		}
	}
	c.activeMounts = uFailures
	if err != nil {
		return fmt.Errorf("failed closing chroot environment, unmount or removal failures: %w", err)
	}
	return nil
}

// RunCallback runs the given callback in a chroot environment
func (c *Chroot) RunCallback(callback func() error) (err error) {
	var currentPath string
	var oldRootF *os.File

	// Store the current path
	currentPath, err = os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current path: %w", err)
	}
	defer func() {
		tmpErr := os.Chdir(currentPath)
		if err == nil && tmpErr != nil {
			err = tmpErr
		}
	}()

	// Chroot to an absolute path
	if !filepath.IsAbs(c.path) {
		oldPath := c.path
		c.path = filepath.Clean(filepath.Join(currentPath, c.path))
		c.logger.Warn("Requested chroot path %s is not absolute, changing it to %s", oldPath, c.path)
	}

	// Store current root
	oldRootF, err = c.fs.OpenFile("/", os.O_RDONLY, vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("opening current root: %w", err)
	}
	defer oldRootF.Close()

	if len(c.activeMounts) == 0 {
		err = c.Prepare()
		if err != nil {
			return fmt.Errorf("preparing default mounts: %w", err)
		}
		defer func() {
			tmpErr := c.Close()
			if err == nil {
				err = tmpErr
			}
		}()
	}
	// Change to new dir before running chroot!
	err = c.syscall.Chdir(c.path)
	if err != nil {
		return fmt.Errorf("chdir %s: %w", c.path, err)
	}

	err = c.syscall.Chroot(c.path)
	if err != nil {
		return fmt.Errorf("chroot %s: %w", c.path, err)
	}

	// Restore to old root
	defer func() {
		tmpErr := oldRootF.Chdir()
		if tmpErr != nil {
			c.logger.Error("can't change to old root dir")
			if err == nil {
				err = tmpErr
			}
		} else {
			tmpErr = c.syscall.Chroot(".")
			if tmpErr != nil {
				c.logger.Error("can't chroot back to old root")
				if err == nil {
					err = tmpErr
				}
			}
		}
	}()

	return callback()
}

// Run executes a command inside a chroot
func (c *Chroot) Run(command string, args ...string) (out []byte, err error) {
	callback := func() error {
		out, err = c.runner.Run(command, args...)
		return err
	}
	err = c.RunCallback(callback)
	if err != nil {
		c.logger.Error("can't run command %s with args %v on chroot: %s", command, args, err)
		c.logger.Debug("Output from command: %s", out)
	}
	return out, err
}
