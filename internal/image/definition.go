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

package image

import (
	"github.com/suse/elemental/v3/internal/image/install"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"

	"github.com/suse/elemental/v3/pkg/sys/platform"
)

const (
	TypeRAW = "raw"
)

type Definition struct {
	Image         Image
	Configuration *Configuration
}

type Configuration struct {
	Installation install.Installation
	Release      release.Release
	Kubernetes   kubernetes.Kubernetes
	Network      Network
	ButaneConfig map[string]any
}

type Image struct {
	ImageType       string
	Platform        *platform.Platform
	OutputImageName string
}

type Network struct {
	CustomScript string
	ConfigDir    string
}
