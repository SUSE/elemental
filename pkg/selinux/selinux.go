/*
Copyright © 2025-2026 SUSE LLC
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

package selinux

import (
	"container/ring"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/suse/elemental/v3/pkg/chroot"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	SelinuxTargetedContextFile = selinuxTargetedPath + "/contexts/files/file_contexts"

	selinuxTargetedPath = "/etc/selinux/targeted"
	debugLines          = 10
)

// Relabel will relabel the system if it finds the context
func Relabel(ctx context.Context, s *sys.System, rootDir string, extraPaths ...string) error {
	contextFile := filepath.Join(rootDir, SelinuxTargetedContextFile)
	contextExists, _ := vfs.Exists(s.FS(), contextFile)

	if contextExists {
		var err error
		args := []string{"-i", "-F"}

		// We only keep last 10 lines of the stdout and stderr for debugging purposes
		stdOut := ring.New(debugLines)
		stdErr := ring.New(debugLines)

		if rootDir == "/" || rootDir == "" {
			args = append(args, contextFile, "/")
		} else {
			args = append(args, "-r", rootDir, contextFile, rootDir)
		}
		args = append(args, extraPaths...)
		err = s.Runner().RunContextParseOutput(ctx, stdHander(stdOut), stdHander(stdErr), "setfiles", args...)
		logOutput(s, stdOut, stdErr)
		return err
	}

	s.Logger().Debug("Not relabelling SELinux, no context found")
	return nil
}

// ChrootedRelabel relables with setfiles the given root in a chroot env. Additionally after the first
// chrooted call it runs a non chrooted call to relabel any mountpoint used within the chroot.
func ChrootedRelabel(ctx context.Context, s *sys.System, rootDir string, bind map[string]string, additionalPaths ...string) (err error) {
	extraPaths := make([]string, 0, len(additionalPaths)+len(bind))
	extraPaths = append(extraPaths, additionalPaths...)

	for _, v := range bind {
		if !canAddForRelabel(v, extraPaths) {
			return fmt.Errorf("failed adding bind mount path '%s' for relabel: path already exists in or overlaps with existing relabel paths: '%s'", v, extraPaths)
		}
		extraPaths = append(extraPaths, v)
	}

	callback := func() error { return Relabel(ctx, s, "/", extraPaths...) }
	err = chroot.ChrootedCallback(s, rootDir, bind, callback, chroot.WithoutDefaultBinds())
	if err != nil {
		return err
	}

	contextsFile := filepath.Join(rootDir, SelinuxTargetedContextFile)
	existsCon, _ := vfs.Exists(s.FS(), contextsFile)

	if existsCon && len(extraPaths) > 0 {
		stdOut := ring.New(debugLines)
		stdErr := ring.New(debugLines)

		args := []string{"-i", "-F", "-r", rootDir, contextsFile}
		for _, path := range extraPaths {
			args = append(args, filepath.Join(rootDir, path))
		}
		err = s.Runner().RunContextParseOutput(ctx, stdHander(stdOut), stdHander(stdErr), "setfiles", args...)
		logOutput(s, stdOut, stdErr)
	}

	return err
}

func stdHander(r *ring.Ring) func(string) {
	return func(line string) {
		r.Value = line
		r = r.Next()
	}
}

func logOutput(s *sys.System, stdOut, stdErr *ring.Ring) {
	output := "\n------- stdOut -------\n"
	stdOut.Do(func(v any) {
		if v != nil {
			output += v.(string) + "\n"
		}
	})
	output += "------- stdErr -------\n"
	stdErr.Do(func(v any) {
		if v != nil {
			output += v.(string) + "\n"
		}
	})
	output += "----------------------\n"
	s.Logger().Debug("SELinux setfile call stdout: %s", output)
}

// canAddForRelabel checks whether the provided src can be included
// to an already existing list of relabel paths.
//
// To add src to existingRelabels, src must introduce a path that is
// not already explicitly, or implicitly defined in existingRelabels.
//
// This ensures that src will not accidentally compromise the state of
// an already defined relabel path.
func canAddForRelabel(src string, existingRelabels []string) bool {
	srcPath := filepath.Clean(src)
	for _, p := range existingRelabels {
		relabelPath := filepath.Clean(p)

		if srcPath == relabelPath {
			return false
		}

		if strings.HasPrefix(srcPath, relabelPath+string(os.PathSeparator)) {
			return false
		}
	}

	return true
}
