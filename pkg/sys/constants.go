/*
Copyright © 2022 - 2025 SUSE LLC

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

package sys

import "os"

const (
	DirPerm        = os.ModeDir | os.ModePerm
	FilePerm       = 0666
	NoWriteDirPerm = 0555 | os.ModeDir
	TempDirPerm    = os.ModePerm | os.ModeSticky | os.ModeDir
	MountBinary    = "/usr/bin/mount"

	// Acrhitectures
	ArchAmd64   = "amd64"
	Archx86     = "x86_64"
	ArchArm64   = "arm64"
	ArchAarch64 = "aarch64"
	ArchRiscV64 = "riscv64"
)
