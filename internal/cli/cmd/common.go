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

package cmd

const (
	// --local flag name and description
	localFlg  = "local"
	localDesc = "Load OCI images from the local container storage instead of a remote registry"

	// --verify flag name and description
	verifyFlg  = "verify"
	verifyDesc = "Verify OCI ssl"

	// --os-image flag name and description
	osImgFlg  = "os-image"
	osImgDesc = "URI to the image containing the operating system"

	// --config flag name and description
	configFlg  = "config"
	configDesc = "Path to OS image post-commit script"

	// --overlay flag name and description
	overlayFlg  = "overlay"
	overlayDesc = "URI of the overlay content for the OS image"

	// --create-boot-entry flag name and description
	createBootFlg  = "create-boot-entry"
	createBootDesc = "Create EFI boot entry"

	// --platform flag name and description
	platformFlg  = "platform"
	platformDesc = "Target platform"

	// --cmdline flag name and description
	cmdlineFlg  = "cmdline"
	cmdlineDesc = "Kernel cmdline for installed system"

	// --output flag name and description
	outputFlg  = "output"
	outputDesc = "File/Path for the generated files"
)
