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

package action

import (
	"log"

	"github.com/urfave/cli/v2"

	"github.com/suse/elemental/v3/internal/cli/elemental/cmd"
)

func Build(*cli.Context) error {
	args := &cmd.BuildArgs

	log.Printf("args: %+v", args)

	// Perform args & input validation, initial setup and branch off to the actual business logic

	return nil
}
