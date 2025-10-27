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
	// Unpack extracts the contents to the provided destination. It allows to exclude
	// certain given paths. All exclude paths are assumed to be tied to source root
	Unpack(ctx context.Context, destination string, excludes ...string) (string, error)

	// SynchedUnpack extracts the contents to the provided destination ensuring origin and
	// destination are perfectly synched, that is any preexisting content in destination which
	// is not part of the original image source will be deleted. In addition, it allows to exclude
	// paths from the synchronization.
	SynchedUnpack(ctx context.Context, destination string, excludes []string, deleteExcludes []string) (string, error)
}

type options struct {
	ociOpts []OCIOpt
	dirOpts []DirectoryOpt
	tarOpts []TarOpt
	rawOpts []RawOpt
}

type Opt func(deployment.ImageSrcType, *options)

func WithLocal(local bool) Opt {
	return func(srcType deployment.ImageSrcType, o *options) {
		switch srcType {
		case deployment.OCI:
			o.ociOpts = append(o.ociOpts, WithLocalOCI(local))
		default:
		}
	}
}

func WithVerify(verify bool) Opt {
	return func(srcType deployment.ImageSrcType, o *options) {
		switch srcType {
		case deployment.OCI:
			o.ociOpts = append(o.ociOpts, WithVerifyOCI(verify))
		default:
		}
	}
}

func WithPlatformRef(platform string) Opt {
	return func(srcType deployment.ImageSrcType, o *options) {
		switch srcType {
		case deployment.OCI:
			o.ociOpts = append(o.ociOpts, WithPlatformRefOCI(platform))
		default:
		}
	}
}

func WithRsyncFlags(flags ...string) Opt {
	return func(srcType deployment.ImageSrcType, o *options) {
		switch srcType {
		case deployment.Dir:
			o.dirOpts = append(o.dirOpts, WithRsyncFlagsDir(flags...))
		case deployment.OCI:
			o.ociOpts = append(o.ociOpts, WithRsyncFlagsOCI(flags...))
		case deployment.Raw:
			o.rawOpts = append(o.rawOpts, WithRsyncFlagsRaw(flags...))
		case deployment.Tar:
			o.tarOpts = append(o.tarOpts, WithRsyncFlagsTar(flags...))
		default:
		}
	}
}

func NewUnpacker(s *sys.System, src *deployment.ImageSource, opts ...Opt) (Interface, error) {
	o := &options{}
	switch {
	case src.IsEmpty():
		return nil, fmt.Errorf("can't create an unpacker for an empty source")
	case src.IsDir():
		for _, opt := range opts {
			opt(deployment.Dir, o)
		}
		return NewDirectoryUnpacker(s, src.URI(), o.dirOpts...), nil
	case src.IsOCI():
		for _, opt := range opts {
			opt(deployment.OCI, o)
		}
		return NewOCIUnpacker(s, src.URI(), o.ociOpts...), nil
	case src.IsRaw():
		for _, opt := range opts {
			opt(deployment.Raw, o)
		}
		return NewRawUnpacker(s, src.URI(), o.rawOpts...), nil
	case src.IsTar():
		for _, opt := range opts {
			opt(deployment.Tar, o)
		}
		return NewTarUnpacker(s, src.URI(), o.tarOpts...), nil
	default:
		return nil, fmt.Errorf("unsupported type of image source")
	}
}
