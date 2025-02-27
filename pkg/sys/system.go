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
	"sync"

	"github.com/suse/elemental/v3/pkg/sys/log"
	"github.com/suse/elemental/v3/pkg/sys/mounter"
	"github.com/suse/elemental/v3/pkg/sys/platform"
	"github.com/suse/elemental/v3/pkg/sys/runner"
	"github.com/suse/elemental/v3/pkg/sys/syscall"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type system struct {
	Logger   log.Logger
	FS       vfs.FS
	Mounter  mounter.Mounter
	Runner   runner.Runner
	Syscall  syscall.SyscallInterface
	Platform *platform.Platform

	initiated bool
}

var sysObj system
var lock = &sync.Mutex{}

type SystemOpts func(a *system)

func WithFS(fs vfs.FS) func(r *system) {
	return func(s *system) {
		s.FS = fs
	}
}

func WithLogger(logger log.Logger) func(r *system) {
	return func(s *system) {
		s.Logger = logger
	}
}

func WithSyscall(syscall syscall.SyscallInterface) func(r *system) {
	return func(s *system) {
		s.Syscall = syscall
	}
}

func WithMounter(mounter mounter.Mounter) func(r *system) {
	return func(r *system) {
		r.Mounter = mounter
	}
}

func WithRunner(runner runner.Runner) func(r *system) {
	return func(r *system) {
		r.Runner = runner
	}
}

func WithPlatform(pf string) func(r *system) {
	return func(s *system) {
		p, err := platform.ParsePlatform(pf)
		if err != nil {
			s.Logger.Errorf("error parsing provided platform (%s): %s", pf, err.Error())
			return
		}
		s.Platform = p
	}
}

func SetSystem(opts ...SystemOpts) {
	lock.Lock()
	defer lock.Unlock()

	if !sysObj.initiated {
		logger := log.NewLogger()
		sysObj.FS = vfs.OSFS()
		sysObj.Logger = logger
		sysObj.Syscall = &syscall.RealSyscall{}
		sysObj.Runner = &runner.RealRunner{Logger: logger}
		sysObj.Mounter = mounter.NewMounter(mounter.Binary)

		for _, o := range opts {
			o(&sysObj)
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
				return
			}
			sysObj.Platform = defaultPlatform
		}
		sysObj.initiated = true
		return
	}
	sysObj.Logger.Debug("can't set system instance, it is already initalized")
}

func GetSystem() *system {
	lock.Lock()
	defer lock.Unlock()

	if !sysObj.initiated {
		panic("system instance not initialized")
	}
	return &sysObj
}

// ClearSystem clears the system singleton varible, this is meant to be used ONLY in tests
func ClearSystem() {
	lock.Lock()
	defer lock.Unlock()

	sysObj.FS = nil
	sysObj.Logger = nil
	sysObj.Syscall = nil
	sysObj.Runner = nil
	sysObj.Mounter = nil
	sysObj.initiated = false
}
