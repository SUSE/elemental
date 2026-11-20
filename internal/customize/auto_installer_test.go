/*
Copyright © 2026 SUSE LLC
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

package customize_test

import (
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/customize"
	"github.com/suse/elemental/v3/pkg/installer"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
)

var _ = Describe("Auto installer script", Label("auto-installer"), func() {
	// override returns the systemd drop-in the script writes for the given unit
	override := func(script, unit string) string {
		start := strings.Index(script, "/etc/systemd/system/"+unit+".service.d/override.conf << EOF\n")
		Expect(start).To(BeNumerically(">=", 0), "no override for %s", unit)
		block := script[start:]
		return block[:strings.Index(block, "\nEOF\n")]
	}

	It("shows the journal on tty1 and keeps root shells on tty2 and the serial console", func() {
		fs, cleanup, err := sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanup)

		path, err := customize.WriteAutoInstaller(fs, "/", installer.ISO)
		Expect(err).NotTo(HaveOccurred())
		data, err := fs.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		script := string(data)

		Expect(override(script, "getty@tty1")).To(HaveSuffix("ExecStart=-/usr/bin/journalctl --follow --no-pager"))
		Expect(override(script, "getty@tty2")).To(HaveSuffix("ExecStart=-/sbin/agetty --autologin root --noclear %I $TERM"))
		Expect(override(script, "serial-getty@ttyS0")).To(HaveSuffix("ExecStart=-/sbin/agetty --autologin root --noclear %I $TERM"))
		Expect(script).To(ContainSubstring("\nsystemctl enable getty@tty2.service\n"))

		// The script is valid bash
		Expect(exec.Command("bash", "-n", "-c", script).Run()).To(Succeed())
	})
})
