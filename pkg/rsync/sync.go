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

package rsync

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/suse/elemental/v3/pkg/sys"
)

type Rsync struct {
	ctx   context.Context
	flags []string
	s     *sys.System
}

type Opts func(r *Rsync)

func WithFlags(flags ...string) Opts {
	return func(r *Rsync) {
		r.flags = flags
	}
}

func WithContext(ctx context.Context) Opts {
	return func(r *Rsync) {
		r.ctx = ctx
	}
}

func NewRsync(s *sys.System, opts ...Opts) *Rsync {
	rsync := &Rsync{
		flags: DefaultFlags(),
		s:     s,
	}

	for _, o := range opts {
		o(rsync)
	}
	return rsync
}

// SyncData rsync's source folder contents to a target folder content,
// both are expected to exist before hand.
func (r Rsync) SyncData(source string, target string, excludes ...string) error {
	flags := r.flags
	for _, e := range excludes {
		flags = append(flags, fmt.Sprintf("--exclude=%s", e))
	}

	return r.rsyncWrapper(source, target, flags)
}

// MirrorData rsync's source folder contents to a target folder content, in contrast, to SyncData this
// method adds the --delete flag which forces the deletion of files in target that are missing in source.
func (r Rsync) MirrorData(source string, target string, excludes ...string) error {
	flags := r.flags
	if !slices.Contains(flags, "--delete") {
		flags = append(flags, "--delete")
	}
	for _, e := range excludes {
		flags = append(flags, fmt.Sprintf("--exclude=%s", e))
	}

	return r.rsyncWrapper(source, target, flags)
}

func (r Rsync) rsyncWrapper(source string, target string, flags []string) error {
	var err error

	fs := r.s.FS()
	log := r.s.Logger()

	if s, err := fs.RawPath(source); err == nil {
		source = s
	}
	if t, err := fs.RawPath(target); err == nil {
		target = t
	}

	if !strings.HasSuffix(source, "/") {
		source = fmt.Sprintf("%s/", source)
	}

	if !strings.HasSuffix(target, "/") {
		target = fmt.Sprintf("%s/", target)
	}

	log.Info("Starting rsync...")

	args := flags
	args = append(args, source, target)

	if r.ctx != nil {
		err = r.s.Runner().RunContextParseOutput(r.ctx, func(msg string) {
			log.Debug("synchronizing: %s", msg)
		}, func(msg string) {
			log.Debug("rsync stderr: %s", msg)
		}, "rsync", args...)
	} else {
		_, err = r.s.Runner().Run("rsync", args...)
	}

	if err != nil {
		log.Error("rsync finished with errors: %s", err.Error())
		return err
	}

	log.Info("Finished syncing")
	return nil
}

func DefaultFlags() []string {
	return []string{"--info=progress2", "--human-readable", "--partial", "--archive", "--xattrs", "--acls", "--filter=-x security.selinux"}
}
