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

package os

import "regexp"

type OperatingSystem struct {
	DiskSize DiskSize `yaml:"diskSize"`
	Users    []User   `yaml:"users"`
}

type User struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type DiskSize string

func (d DiskSize) IsValid() bool {
	return regexp.MustCompile(`^[1-9]\d*[KMGT]$`).MatchString(string(d))
}
