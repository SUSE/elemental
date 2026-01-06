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

package action

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"

	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/unpack"
)

func Unpack(ctx *cli.Context) error {
	var s *sys.System
	args := &cmd.UnpackArgs
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	s = ctx.App.Metadata["system"].(*sys.System)

	s.Logger().Info("Starting unpack action with args: %+v", args)

	unpacker := unpack.NewOCIUnpacker(s, args.Image,
		unpack.WithLocalOCI(args.Local),
		unpack.WithPlatformRefOCI(args.Platform),
		unpack.WithVerifyOCI(args.Verify))

	ctxSignal, stop := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		stop()
	}()

	_, err := unpacker.Unpack(ctxSignal, args.TargetDir)
	if err != nil {
		s.Logger().Error("Failed to unpack image %s", args.Image)
		return err
	}

	s.Logger().Info("Image %s unpacked", args.Image)

	return nil
}
