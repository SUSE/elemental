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

package platform

import (
	"fmt"
	"runtime"
	"strings"
)

const (
	// Architectures
	ArchAmd64   = "amd64"
	Archx86     = "x86_64"
	ArchArm64   = "arm64"
	ArchAarch64 = "aarch64"
)

type Platform struct {
	OS         string
	Arch       string
	GolangArch string
}

func NewPlatform(os, arch string) (*Platform, error) {
	golangArch, err := archToGolangArch(arch)
	if err != nil {
		return nil, err
	}

	arch, err = golangArchToArch(arch)
	if err != nil {
		return nil, err
	}

	return &Platform{
		OS:         os,
		Arch:       arch,
		GolangArch: golangArch,
	}, nil
}

func NewPlatformFromArch(arch string) (*Platform, error) {
	return NewPlatform("linux", arch)
}

func NewDefaultPlatform() (*Platform, error) {
	return NewPlatformFromArch(runtime.GOARCH)
}

// ParsePlatform parses a string representing a Platform, if possible.
// code ported from go-containerregistry library
func ParsePlatform(s string) (*Platform, error) {
	var architecture, os string
	parts := strings.Split(strings.TrimSpace(s), ":")
	// We ignore parts[1] if any, as it represents the OS Version
	parts = strings.Split(parts[0], "/")
	if len(parts) > 0 {
		os = parts[0]
	}
	if len(parts) > 1 {
		architecture = parts[1]
	}
	// We ignore parts[2] if any, as this represents the arch variant
	if len(parts) > 3 {
		return nil, fmt.Errorf("too many slashes in platform spec: %s", s)
	}
	return NewPlatform(os, architecture)
}

func (p *Platform) String() string {
	if p == nil {
		return ""
	}

	return fmt.Sprintf("%s/%s", p.OS, p.GolangArch)
}

var errInvalidArch = fmt.Errorf("invalid arch")

func archToGolangArch(arch string) (string, error) {
	switch strings.ToLower(arch) {
	case ArchAmd64:
		return ArchAmd64, nil
	case Archx86:
		return ArchAmd64, nil
	case ArchArm64:
		return ArchArm64, nil
	case ArchAarch64:
		return ArchArm64, nil
	default:
		return "", errInvalidArch
	}
}

func golangArchToArch(arch string) (string, error) {
	switch strings.ToLower(arch) {
	case Archx86:
		return Archx86, nil
	case ArchAmd64:
		return Archx86, nil
	case ArchArm64:
		return ArchArm64, nil
	case ArchAarch64:
		return ArchArm64, nil
	default:
		return "", errInvalidArch
	}
}
