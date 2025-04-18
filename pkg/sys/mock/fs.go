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

package mock

import (
	"fmt"

	"github.com/suse/elemental/v3/pkg/sys/vfs"
	gvfs "github.com/twpayne/go-vfs/v4"
	"github.com/twpayne/go-vfs/v4/vfst"
)

func TestFS(root any) (vfs.FS, func(), error) {
	return vfst.NewTestFS(root)
}

func ReadOnlyTestFS(fs vfs.FS) (vfs.FS, error) {
	if tfs, isTestFs := fs.(*vfst.TestFS); isTestFs {
		return gvfs.NewReadOnlyFS(tfs), nil
	}
	return nil, fmt.Errorf("provided FS is not a vfst instance")
}
