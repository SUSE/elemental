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
	"runtime"

	"github.com/suse/elemental/v3/pkg/sys/log"
	"github.com/suse/elemental/v3/pkg/sys/mounter"
	"github.com/suse/elemental/v3/pkg/sys/platform"
	"github.com/suse/elemental/v3/pkg/sys/runner"
	"github.com/suse/elemental/v3/pkg/sys/syscall"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type System struct {
	Logger   log.Logger
	FS       vfs.FS
	Mounter  mounter.Mounter
	Runner   runner.Runner
	Syscall  syscall.SyscallInterface
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

func WithSyscall(syscall syscall.SyscallInterface) SystemOpts {
	return func(s *System) error {
		s.Syscall = syscall
		return nil
	}
}

func WithMounter(mounter mounter.Mounter) SystemOpts {
	return func(r *System) error {
		r.Mounter = mounter
		return nil
	}
}

func WithRunner(runner runner.Runner) SystemOpts {
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
	logger := log.NewLogger()
	sysObj := &System{
		FS:      vfs.OSFS(),
		Logger:  logger,
		Syscall: &syscall.RealSyscall{},
		Runner:  &runner.RealRunner{},
		Mounter: mounter.NewMounter(mounter.Binary),
	}

	for _, o := range opts {
		err := o(sysObj)
		if err != nil {
			return nil, err
		}
	}

	// Now check if the runner has a logger inside, otherwise point our logger into it
	// This can happen if we set the WithRunner option as that doesn't set a logger
	if sysObj.Runner.GetLogger() == nil {
		sysObj.Runner.SetLogger(sysObj.Logger)
	}

	if sysObj.Platform == nil {
		defaultPlatform, err := platform.NewPlatformFromArch(runtime.GOARCH)
		if err != nil {
			sysObj.Logger.Errorf("error parsing default platform (%s): %s", runtime.GOARCH, err.Error())
			return nil, err
		}
		sysObj.Platform = defaultPlatform
	}
	return sysObj, nil
}
