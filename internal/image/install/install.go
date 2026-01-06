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

package install

import (
	"math/bits"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/docker/go-units"
	"github.com/suse/elemental/v3/pkg/crypto"
)

type DiskSize string

func (d DiskSize) IsValid() bool {
	return regexp.MustCompile(`^[1-9]\d*[KMGT]$`).MatchString(string(d))
}

// returns the size in MiB
func (d DiskSize) ToMiB() (uint, error) {
	size := strings.TrimSpace(string(d))
	dimension := uint(1)
	switch unicode.ToUpper(rune(size[len(size)-1])) {
	case 'K':
		dimension = units.KiB
	case 'M':
		dimension = units.MiB
	case 'G':
		dimension = units.GiB
	case 'T':
		dimension = units.TiB
	}

	value, err := strconv.ParseUint(size[:len(size)-1], 10, bits.UintSize)
	if err != nil {
		return 0, err
	}

	return uint(value) * dimension / units.MiB, nil
}

type Installation struct {
	Bootloader    string        `yaml:"bootloader"`
	KernelCmdLine string        `yaml:"kernelCmdLine"`
	RAW           RAW           `yaml:"raw"`
	ISO           ISO           `yaml:"iso"`
	CryptoPolicy  crypto.Policy `yaml:"cryptoPolicy"`
}

type RAW struct {
	DiskSize DiskSize `yaml:"diskSize"`
}

type ISO struct {
	Device string `yaml:"device"`
}
