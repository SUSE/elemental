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
)

type system struct {
	Logger   Logger
	FS       FS
	Mounter  Mounter
	Runner   Runner
	Syscall  SyscallInterface
	Platform *Platform
}

var sysInstance *system
var lock = &sync.Mutex{}

type SystemOpts func(a *system)

func WithFS(fs FS) func(r *system) {
	return func(s *system) {
		s.FS = fs
	}
}

func WithLogger(logger Logger) func(r *system) {
	return func(s *system) {
		s.Logger = logger
	}
}

func WithSyscall(syscall SyscallInterface) func(r *system) {
	return func(s *system) {
		s.Syscall = syscall
	}
}

func WithMounter(mounter Mounter) func(r *system) {
	return func(r *system) {
		r.Mounter = mounter
	}
}

func WithRunner(runner Runner) func(r *system) {
	return func(r *system) {
		r.Runner = runner
	}
}

func WithPlatform(platform string) func(r *system) {
	return func(s *system) {
		p, err := ParsePlatform(platform)
		if err != nil {
			s.Logger.Errorf("error parsing provided platform (%s): %s", platform, err.Error())
			return
		}
		s.Platform = p
	}
}

func SetSystem(opts ...SystemOpts) {
	if sysInstance == nil {
		lock.Lock()
		defer lock.Unlock()

		log := NewLogger()
		sysInstance = &system{
			FS:      OSFS(),
			Logger:  log,
			Syscall: &RealSyscall{},
			Runner:  &RealRunner{Logger: log},
			Mounter: NewMounter(MountBinary),
		}
		for _, o := range opts {
			o(sysInstance)
		}

		// Now check if the runner has a logger inside, otherwise point our logger into it
		// This can happen if we set the WithRunner option as that doesn't set a logger
		if sysInstance.Runner.GetLogger() == nil {
			sysInstance.Runner.SetLogger(sysInstance.Logger)
		}

		if sysInstance.Platform == nil {
			defaultPlatform, err := NewPlatformFromArch(runtime.GOARCH)
			if err != nil {
				sysInstance.Logger.Errorf("error parsing default platform (%s): %s", runtime.GOARCH, err.Error())
				sysInstance = nil
				return
			}
			sysInstance.Platform = defaultPlatform
		}
		return
	}
	sysInstance.Logger.Debug("can't set system instance, it is already initalized")
}

func GetSystem() *system {
	lock.Lock()
	defer lock.Unlock()

	if sysInstance == nil {
		panic("system instance not initialized")
	}
	return sysInstance
}

// ClearSystem clears the system singleton varible, this is meant to be used ONLY in tests
func ClearSystem() {
	sysInstance = nil
}
