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

package unpack

import (
	"context"
	"fmt"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/sys"
)

type Interface interface {
	// Unpack extracts the contents to the provided destination
	Unpack(ctx context.Context, destination string) (string, error)

	// SynchedUnpack extracts the contents to the provided destination ensuring origin and
	// destination are perfectly synched, that is any preexisting content in destination which
	// is not part of the original image source will be deleted. In addition, it allows to exclude
	// paths from the synchronization.
	SynchedUnpack(ctx context.Context, destination string, excludes ...string) (string, error)
}

func NewUnpacker(s *sys.System, src *deployment.ImageSource) (Interface, error) {
	switch {
	case src.IsEmpty():
		return nil, fmt.Errorf("can't create an unpacker for an empty source")
	case src.IsDir():
		return NewDirectoryUnpacker(s, src.URI()), nil
	case src.IsOCI():
		return NewOCIUnpacker(s, src.URI()), nil
	case src.IsRaw():
		return NewRawUnpacker(s, src.URI()), nil
	default:
		return nil, fmt.Errorf("unsupported type of image source")
	}
}
