/*
Copyright © 2022-2026 SUSE LLC
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
	"context"

	"github.com/suse/elemental/v3/pkg/containerd"
)

var _ containerd.Interface = (*ContainerdMock)(nil)

type ContainerdMock struct {
	MntRootFS     string
	EFind         error
	ERunOnMounted error
	Img           containerd.ImgMeta
}

func (c ContainerdMock) FindUnpackedImage(_ context.Context, _ string) (containerd.ImgMeta, error) {
	if c.EFind != nil {
		return containerd.ImgMeta{}, c.EFind
	}
	return c.Img, nil
}

func (c ContainerdMock) RunOnMountedROSnapshot(_ context.Context, _ containerd.ImgMeta, callback func(rootfs string) error) (err error) {
	if c.ERunOnMounted != nil {
		return c.ERunOnMounted
	}
	return callback(c.MntRootFS)
}
