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

package sys

import (
	"context"
	"os/exec"
	"runtime"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys/mounter"
	"github.com/suse/elemental/v3/pkg/sys/platform"
	"github.com/suse/elemental/v3/pkg/sys/runner"
	"github.com/suse/elemental/v3/pkg/sys/syscall"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type Mounter interface {
	Mount(source string, target string, fstype string, options []string) error
	Unmount(target string) error
	IsMountPoint(path string) (bool, error)
	// GetMountRefs finds all mount references to pathname, returning a slice of
	// paths. The returned slice does not include the given path.
	GetMountRefs(pathname string) ([]string, error)
}

type Runner interface {
	Run(cmd string, args ...string) ([]byte, error)
	RunContext(cxt context.Context, cmd string, args ...string) ([]byte, error)
	RunContextParseOutput(ctx context.Context, stdoutH, stderrH func(line string), cmd string, args ...string) error
}

type Syscall interface {
	Chroot(string) error
	Chdir(string) error
}

type System struct {
	logger   log.Logger
	fs       FS
	mounter  Mounter
	runner   Runner
	syscall  Syscall
	platform *platform.Platform
}

type SystemOpts func(a *System) error

func WithFS(fs FS) SystemOpts {
	return func(s *System) error {
		s.fs = fs
		return nil
	}
}

func WithLogger(logger log.Logger) SystemOpts {
	return func(s *System) error {
		s.logger = logger
		return nil
	}
}

func WithSyscall(syscall Syscall) SystemOpts {
	return func(s *System) error {
		s.syscall = syscall
		return nil
	}
}

func WithMounter(mounter Mounter) SystemOpts {
	return func(r *System) error {
		r.mounter = mounter
		return nil
	}
}

func WithRunner(runner Runner) SystemOpts {
	return func(r *System) error {
		r.runner = runner
		return nil
	}
}

func WithPlatform(pf string) SystemOpts {
	return func(s *System) error {
		p, err := platform.ParsePlatform(pf)
		if err != nil {
			return err
		}
		s.platform = p
		return nil
	}
}

func NewSystem(opts ...SystemOpts) (*System, error) {
	logger := log.New()
	sysObj := &System{
		fs:      vfs.OSFS(),
		logger:  logger,
		syscall: syscall.Syscall(),
		mounter: mounter.NewMounter(mounter.Binary),
	}

	for _, o := range opts {
		err := o(sysObj)
		if err != nil {
			return nil, err
		}
	}

	// Defer the runner creation in case the caller set a custom logger
	if sysObj.runner == nil {
		sysObj.runner = runner.NewRunner(runner.WithLogger(sysObj.logger))
	}

	if sysObj.platform == nil {
		defaultPlatform, err := platform.NewPlatformFromArch(runtime.GOARCH)
		if err != nil {
			return nil, err
		}
		sysObj.platform = defaultPlatform
	}
	return sysObj, nil
}

func (s System) Platform() *platform.Platform {
	return s.platform
}

func (s System) FS() FS {
	return s.fs
}

func (s System) Syscall() Syscall {
	return s.syscall
}

func (s System) Mounter() Mounter {
	return s.mounter
}

func (s System) Runner() Runner {
	return s.runner
}

func (s System) Logger() log.Logger {
	return s.logger
}

// CommandExists
func CommandExists(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}
