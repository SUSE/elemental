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

package hardening_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/hardening"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	root       = "/snapshot"
	datastream = "/usr/share/xml/scap/ssg/content/ssg-test-ds.xml"
)

var _ = Describe("Apply", func() {
	var runner *sysmock.Runner
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var scanXML, resultsXML []byte

	BeforeEach(func() {
		var err error
		scanXML, err = os.ReadFile("testdata/scan-before.xml")
		Expect(err).NotTo(HaveOccurred())
		resultsXML, err = os.ReadFile("testdata/results.xml")
		Expect(err).NotTo(HaveOccurred())
		tailoringXML, err := os.ReadFile("testdata/tailoring.xml")
		Expect(err).NotTo(HaveOccurred())

		runner = sysmock.NewRunner()
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/dev/pts/empty":                    []byte{},
			"/proc/empty":                       []byte{},
			"/sys/empty":                        []byte{},
			"/host/tailoring.xml":               tailoringXML,
			root + "/usr/bin/oscap":             []byte{},
			root + datastream:                   []byte{},
			root + "/var/log/elemental/.keep":   []byte{},
			root + "/etc/elemental/placeholder": []byte{},
		})
		Expect(err).NotTo(HaveOccurred())

		s, err = sys.NewSystem(
			sys.WithMounter(sysmock.NewMounter()), sys.WithRunner(runner),
			sys.WithFS(fs), sys.WithLogger(log.New(log.WithDiscardAll())),
			sys.WithSyscall(&sysmock.Syscall{}),
		)
		Expect(err).NotTo(HaveOccurred())

		// Fake oscap: write the canned results file wherever --results points,
		// relative to the chroot root, like the real oscap would.
		runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			if cmd != "/usr/bin/oscap" {
				return nil, nil
			}
			i := slices.Index(args, "--results")
			if i < 0 || i+1 >= len(args) {
				return nil, fmt.Errorf("no --results given")
			}
			data := scanXML
			if slices.Contains(args, "--remediate") {
				data = resultsXML
			}
			return nil, fs.WriteFile(filepath.Join(root, args[i+1]), data, vfs.FilePerm)
		}
	})
	AfterEach(func() {
		cleanup()
	})

	It("fails when oscap is not in the image", func() {
		Expect(fs.Remove(root + "/usr/bin/oscap")).To(Succeed())
		_, err := hardening.Apply(context.Background(), s, root, &hardening.Config{Profile: "stig", Datastream: datastream})
		Expect(err).To(MatchError(ContainSubstring("openscap")))
	})

	It("finds the only datastream when none is configured", func() {
		res, err := hardening.Apply(context.Background(), s, root, &hardening.Config{Profile: "stig"})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Profile).To(Equal(hardening.SSGProfilePrefix + "stig"))
		cmds := oscapCmds(runner)
		Expect(cmds[0]).To(HaveLen(8))
		Expect(cmds[0][7]).To(Equal(datastream))
	})

	It("requires a datastream to be chosen when there are several", func() {
		Expect(fs.WriteFile(root+"/usr/share/xml/scap/ssg/content/ssg-other-ds.xml", []byte{}, vfs.FilePerm)).To(Succeed())
		_, err := hardening.Apply(context.Background(), s, root, &hardening.Config{Profile: "stig"})
		Expect(err).To(MatchError(ContainSubstring("several SCAP datastreams")))
	})

	It("copies the tailoring file, skips handled rules and returns kernel arguments", func() {
		cfg := &hardening.Config{Tailoring: "/host/tailoring.xml", Datastream: datastream}
		res, err := hardening.Apply(context.Background(), s, root, cfg)
		Expect(err).NotTo(HaveOccurred())

		Expect(vfs.Exists(fs, root+hardening.InImageTailoring)).To(BeTrue())
		Expect(res.Tailoring).To(Equal(hardening.InImageTailoring))
		Expect(res.Profile).To(Equal(hardening.SSGProfilePrefix + "stig_elemental"))

		Expect(res.Plan.KernelArgs).To(ConsistOf("audit=1", "slub_debug=FZP"))
		Expect(res.Plan.RequiresFIPS).To(BeTrue())
		Expect(res.Failed).To(ConsistOf(rule + "partition_for_var_log_audit"))
		Expect(res.Counts).To(HaveKeyWithValue(hardening.ResultFixed, 1))

		cmds := oscapCmds(runner)
		Expect(cmds).To(HaveLen(2))
		Expect(cmds[0]).NotTo(ContainElement("--remediate"))
		Expect(cmds[1]).To(ContainElement("--remediate"))
		Expect(cmds[1]).To(ContainElements("--tailoring-file", hardening.InImageTailoring))
		for _, id := range []string{
			"grub2_audit_argument", "grub2_slub_debug_argument", "grub2_uefi_password",
			"grub2_enable_fips_mode", "package_aide_installed",
		} {
			Expect(cmds[1]).To(ContainElement(rule+id), id)
		}
		Expect(cmds[1]).NotTo(ContainElement(rule + "sshd_disable_root_login"))
		Expect(cmds[1]).NotTo(ContainElement(rule + "package_telnet_removed"))
		Expect(res.Plan.Packages).To(ConsistOf(rule + "package_aide_installed"))
	})

	It("does not copy the tailoring file again on upgrade", func() {
		tailoringXML, err := os.ReadFile("testdata/tailoring.xml")
		Expect(err).NotTo(HaveOccurred())
		Expect(vfs.MkdirAll(fs, filepath.Dir(root+hardening.InImageTailoring), vfs.DirPerm)).To(Succeed())
		Expect(fs.WriteFile(root+hardening.InImageTailoring, tailoringXML, vfs.FilePerm)).To(Succeed())
		Expect(fs.Remove("/host/tailoring.xml")).To(Succeed())

		cfg := &hardening.Config{Tailoring: hardening.InImageTailoring, Datastream: datastream}
		_, err = hardening.Apply(context.Background(), s, root, cfg)
		Expect(err).NotTo(HaveOccurred())
	})

	It("propagates oscap errors", func() {
		runner.SideEffect = func(string, ...string) ([]byte, error) { return nil, fmt.Errorf("oscap broke") }
		_, err := hardening.Apply(context.Background(), s, root, &hardening.Config{Profile: "stig", Datastream: datastream})
		Expect(err).To(MatchError(ContainSubstring("oscap broke")))
	})
})

// oscapCmds returns the oscap invocations recorded by the runner, leaving out
// other commands such as the 'sync' the chroot helper runs on exit.
func oscapCmds(r *sysmock.Runner) [][]string {
	var cmds [][]string
	for _, c := range r.GetCmds() {
		if len(c) > 0 && c[0] == "/usr/bin/oscap" {
			cmds = append(cmds, c)
		}
	}
	return cmds
}
