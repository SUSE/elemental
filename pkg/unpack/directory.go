/*
Copyright © 2025 SUSE LLC
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

package unpack

import (
	"context"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/rsync"
	"github.com/suse/elemental/v3/pkg/sys"
)

type Directory struct {
	s    *sys.System
	path string
}

func NewDirectoryUnpacker(s *sys.System, path string) *Directory {
	return &Directory{s: s, path: path}
}

func (d Directory) Unpack(ctx context.Context, destination string) (string, error) {
	sync := rsync.NewRsync(d.s, rsync.WithContext(ctx))
	digest := findDeploymentDigest(d.s, d.path)
	return digest, sync.SyncData(d.path, destination)
}

func (d Directory) SynchedUnpack(ctx context.Context, destination string, excludes []string, deleteExcludes []string) (string, error) {
	sync := rsync.NewRsync(d.s, rsync.WithContext(ctx))
	digest := findDeploymentDigest(d.s, d.path)
	return digest, sync.MirrorData(d.path, destination, excludes, deleteExcludes)
}

// findDeploymentDigest attemps to read a deployment file from the source directory tree
// and read the source digest if any. This is helpful to get the original image digest
// if the source is already a deployment.
func findDeploymentDigest(s *sys.System, path string) string {
	var digest string
	d, _ := deployment.ReadDeployment(s, path)
	if d != nil && d.SourceOS != nil {
		digest = d.SourceOS.GetDigest()
	}
	return digest
}
