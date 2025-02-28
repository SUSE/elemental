/*
Copyright © 2022 - 2025 SUSE LLC

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
	InitCmd(string, ...string) *exec.Cmd
	Run(string, ...string) ([]byte, error)
	RunCmd(cmd *exec.Cmd) ([]byte, error)
	CommandExists(command string) bool
}

type Syscall interface {
	Chroot(string) error
	Chdir(string) error
}

type System struct {
	Logger   log.Logger
	FS       vfs.FS
	Mounter  Mounter
	Runner   Runner
	Syscall  Syscall
	Platform *platform.Platform
}

type SystemOpts func(a *System) error

func WithFS(fs vfs.FS) SystemOpts {
	return func(s *System) error {
		s.FS = fs
		return nil
	}
}

func WithLogger(logger log.Logger) SystemOpts {
	return func(s *System) error {
		s.Logger = logger
		return nil
	}
}

func WithSyscall(syscall Syscall) SystemOpts {
	return func(s *System) error {
		s.Syscall = syscall
		return nil
	}
}

func WithMounter(mounter Mounter) SystemOpts {
	return func(r *System) error {
		r.Mounter = mounter
		return nil
	}
}

func WithRunner(runner Runner) SystemOpts {
	return func(r *System) error {
		r.Runner = runner
		return nil
	}
}

func WithPlatform(pf string) SystemOpts {
	return func(s *System) error {
		p, err := platform.ParsePlatform(pf)
		if err != nil {
			return err
		}
		s.Platform = p
		return nil
	}
}

func NewSystem(opts ...SystemOpts) (*System, error) {
	logger := log.New()
	sysObj := &System{
		FS:      vfs.OSFS(),
		Logger:  logger,
		Syscall: syscall.Syscall(),
		Runner:  runner.NewRunner(),
		Mounter: mounter.NewMounter(mounter.Binary),
	}

	for _, o := range opts {
		err := o(sysObj)
		if err != nil {
			return nil, err
		}
	}

	// Defer the runner creation in case the caller set a custom logger
	if sysObj.Runner == nil {
		sysObj.Runner = runner.NewRunner(runner.WithLogger(sysObj.Logger))
	}

	if sysObj.Platform == nil {
		defaultPlatform, err := platform.NewPlatformFromArch(runtime.GOARCH)
		if err != nil {
			return nil, err
		}
		sysObj.Platform = defaultPlatform
	}
	return sysObj, nil
}
