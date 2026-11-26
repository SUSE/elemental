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
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/hardening"
)

func TestHardeningSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hardening test suite")
}

const (
	rule  = hardening.SSGRulePrefix
	value = hardening.SSGValuePrefix
)

// failing returns scan results with every given rule selected and failing.
func failing(ids ...string) map[string]string {
	m := map[string]string{}
	for _, id := range ids {
		m[id] = hardening.ResultFail
	}
	return m
}

var _ = Describe("Config", func() {
	It("is disabled when empty or nil", func() {
		var nilCfg *hardening.Config
		Expect(nilCfg.Enabled()).To(BeFalse())
		Expect(nilCfg.Validate()).To(Succeed())
		Expect((&hardening.Config{}).Enabled()).To(BeFalse())
	})
	It("rejects a datastream without a profile", func() {
		Expect((&hardening.Config{Datastream: "/ds.xml"}).Validate()).NotTo(Succeed())
	})
	It("rejects relative paths and odd profile IDs", func() {
		Expect((&hardening.Config{Profile: "stig", Datastream: "ds.xml"}).Validate()).NotTo(Succeed())
		Expect((&hardening.Config{Tailoring: "t.xml"}).Validate()).NotTo(Succeed())
		Expect((&hardening.Config{Profile: "stig; rm -rf /"}).Validate()).NotTo(Succeed())
		Expect((&hardening.Config{Profile: "stig", Tailoring: "/t.xml", Datastream: "/ds.xml"}).Validate()).To(Succeed())
	})
	It("expands short profile IDs", func() {
		Expect(hardening.NormalizeProfileID("stig")).To(Equal("xccdf_org.ssgproject.content_profile_stig"))
		Expect(hardening.NormalizeProfileID("xccdf_com.example_profile_x")).To(Equal("xccdf_com.example_profile_x"))
		Expect(hardening.NormalizeProfileID("")).To(BeEmpty())
	})
	It("resolves the profile against a tailoring file", func() {
		tailored := []string{hardening.SSGProfilePrefix + "stig_elemental"}

		p, err := hardening.ResolveProfile("", tailored)
		Expect(err).NotTo(HaveOccurred())
		Expect(p).To(Equal(tailored[0]))

		p, err = hardening.ResolveProfile("stig_elemental", tailored)
		Expect(err).NotTo(HaveOccurred())
		Expect(p).To(Equal(tailored[0]))

		_, err = hardening.ResolveProfile("stig", tailored)
		Expect(err).To(MatchError(ContainSubstring("not defined in the tailoring file")))

		_, err = hardening.ResolveProfile("", []string{"a", "b"})
		Expect(err).To(MatchError(ContainSubstring("several profiles")))

		_, err = hardening.ResolveProfile("", nil)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("Classify", func() {
	It("maps kernel argument rules, flags FIPS and skips bootloader and package rules", func() {
		plan := hardening.Classify(failing(
			rule+"grub2_audit_argument",
			rule+"grub2_slub_debug_argument",
			rule+"grub2_spec_store_bypass_disable_argument", // variable without default and unset
			rule+"grub2_uefi_password",
			rule+"grub2_enable_fips_mode",
			rule+"package_aide_installed",
			rule+"package_telnet-server_removed",
			rule+"sshd_disable_root_login",
			"xccdf_com.example_rule_custom",
		), map[string]string{value + "var_slub_debug_options": "FZP"})

		Expect(plan.KernelArgs).To(ConsistOf("audit=1", "slub_debug=FZP"))
		Expect(plan.RequiresFIPS).To(BeTrue())
		Expect(plan.Unhandled).To(ConsistOf(
			rule+"grub2_uefi_password",
			rule+"grub2_spec_store_bypass_disable_argument",
		))
		Expect(plan.Packages).To(ConsistOf(
			rule+"package_aide_installed",
			rule+"package_telnet-server_removed",
		))
		Expect(plan.Skip).To(HaveLen(7))
		Expect(plan.Skip).NotTo(ContainElement(rule + "sshd_disable_root_login"))
		Expect(plan.Skip).NotTo(ContainElement("xccdf_com.example_rule_custom"))
	})
	It("collects arguments the profile forbids", func() {
		plan := hardening.Classify(failing(
			rule+"grub2_nosmap_argument_absent",
			rule+"grub2_enable_selinux",
			rule+"grub2_slab_nomerge_argument",
		), nil)
		Expect(plan.ForbiddenArgs).To(ConsistOf("nosmap", "selinux=0", "enforcing=0"))
		Expect(plan.KernelArgs).To(ConsistOf("slab_nomerge=yes"))
		Expect(plan.Unhandled).To(BeEmpty())
		Expect(plan.Skip).To(HaveLen(3))
	})
	It("uses the fixed default when the profile does not set the variable", func() {
		plan := hardening.Classify(failing(rule+"grub2_slub_debug_argument"), nil)
		Expect(plan.KernelArgs).To(ConsistOf("slub_debug=P"))
	})
	It("only skips and reports package rules the image does not satisfy", func() {
		plan := hardening.Classify(map[string]string{
			rule + "package_aide_installed":         hardening.ResultFail,
			rule + "package_sudo_installed":         hardening.ResultError,
			rule + "package_telnet_removed":         hardening.ResultPass,
			rule + "package_rsyslog_installed":      hardening.ResultNotApplicable,
			rule + "package_tftp-server_removed":    hardening.ResultNotSelected,
			rule + "grub2_audit_argument":           hardening.ResultPass,
			rule + "grub2_page_poison_argument":     hardening.ResultNotSelected,
			rule + "package_sendmail_removed":       hardening.ResultPass,
			rule + "package_chrony_installed":       hardening.ResultUnknown,
			rule + "sshd_disable_root_login":        hardening.ResultFail,
			rule + "grub2_slab_nomerge_argument":    hardening.ResultNotApplicable,
			rule + "package_logrotate_installed":    hardening.ResultFail,
			rule + "package_talk-server_removed":    hardening.ResultNotChecked,
			rule + "package_setroubleshoot_removed": hardening.ResultFail,
		}, nil)
		Expect(plan.Packages).To(ConsistOf(
			rule+"package_aide_installed",
			rule+"package_sudo_installed",
			rule+"package_chrony_installed",
			rule+"package_logrotate_installed",
			rule+"package_talk-server_removed",
			rule+"package_setroubleshoot_removed",
		))
		Expect(plan.Skip).To(ContainElements(plan.Packages))
		Expect(plan.Skip).NotTo(ContainElement(rule + "package_telnet_removed"))
		Expect(plan.Skip).NotTo(ContainElement(rule + "package_tftp-server_removed"))
		// Bootloader rules are always skipped, whatever their scan result,
		// because oscap must not edit a bootloader configuration Elemental manages.
		Expect(plan.Skip).To(ContainElements(rule+"grub2_audit_argument", rule+"grub2_slab_nomerge_argument"))
		Expect(plan.Skip).NotTo(ContainElement(rule + "grub2_page_poison_argument"))
		Expect(plan.KernelArgs).To(ConsistOf("audit=1", "slab_nomerge=yes"))
	})
})

var _ = Describe("AppendKernelArgs", func() {
	It("appends missing arguments and reports them", func() {
		out, appended, err := hardening.AppendKernelArgs("console=ttyS0 quiet", []string{"audit=1", "pti=on"})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("console=ttyS0 quiet audit=1 pti=on"))
		Expect(appended).To(Equal([]string{"audit=1", "pti=on"}))
	})
	It("is idempotent so upgrades do not duplicate arguments", func() {
		out, appended, err := hardening.AppendKernelArgs("quiet audit=1 pti=on", []string{"audit=1", "pti=on"})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("quiet audit=1 pti=on"))
		Expect(appended).To(BeEmpty())
	})
	It("fails on a conflicting value", func() {
		_, _, err := hardening.AppendKernelArgs("audit=0", []string{"audit=1"})
		Expect(err).To(MatchError(ContainSubstring(`sets "audit=0"`)))
	})
	It("does not confuse arguments sharing a prefix", func() {
		out, _, err := hardening.AppendKernelArgs("audit_backlog_limit=8192", []string{"audit=1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("audit_backlog_limit=8192 audit=1"))
	})
	It("handles an empty command line", func() {
		out, _, err := hardening.AppendKernelArgs("", []string{"audit=1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("audit=1"))
	})
})

var _ = Describe("CheckForbiddenKernelArgs", func() {
	forbidden := []string{"nosmap", "selinux=0", "systemd.confirm_spawn"}

	It("accepts a command line without forbidden arguments", func() {
		Expect(hardening.CheckForbiddenKernelArgs("quiet selinux=1 nosmapfoo systemd.confirm_spawn=0", forbidden)).To(Succeed())
		Expect(hardening.CheckForbiddenKernelArgs("", forbidden)).To(Succeed())
	})
	It("rejects bare and valued forms of a bare forbidden argument", func() {
		Expect(hardening.CheckForbiddenKernelArgs("quiet nosmap", forbidden)).To(MatchError(ContainSubstring("nosmap")))
		Expect(hardening.CheckForbiddenKernelArgs("nosmap=1", forbidden)).To(MatchError(ContainSubstring("nosmap=1")))
	})
	It("rejects exact key=value matches and enabled forms", func() {
		err := hardening.CheckForbiddenKernelArgs("selinux=0 systemd.confirm_spawn=yes", forbidden)
		Expect(err).To(MatchError(ContainSubstring("selinux=0 systemd.confirm_spawn=yes")))
		Expect(hardening.CheckForbiddenKernelArgs("systemd.confirm_spawn", forbidden)).NotTo(Succeed())
		Expect(hardening.CheckForbiddenKernelArgs("systemd.confirm_spawn=off", forbidden)).To(Succeed())
	})
})

var _ = Describe("XCCDF parsing", func() {
	It("reads selected rules and values from scan results", func() {
		f, err := os.Open("testdata/scan-before.xml")
		Expect(err).NotTo(HaveOccurred())
		defer f.Close()

		res, err := hardening.ParseResults(f)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Values).To(HaveKeyWithValue(value+"var_slub_debug_options", "FZP"))
		Expect(res.Rules).To(HaveKeyWithValue(rule+"grub2_pti_argument", hardening.ResultNotSelected))
		Expect(res.Selected()).To(HaveLen(8))
		Expect(res.Selected()).NotTo(ContainElement(rule + "grub2_pti_argument"))
		Expect(res.Count()).To(HaveKeyWithValue(hardening.ResultFail, 7))
	})
	It("fails when there is no TestResult", func() {
		_, err := hardening.ParseResults(strings.NewReader(`<Benchmark/>`))
		Expect(err).To(MatchError(ContainSubstring("no TestResult")))
	})
	It("lists tailored profiles", func() {
		f, err := os.Open("testdata/tailoring.xml")
		Expect(err).NotTo(HaveOccurred())
		defer f.Close()

		ids, err := hardening.TailoringProfiles(f)
		Expect(err).NotTo(HaveOccurred())
		Expect(ids).To(Equal([]string{hardening.SSGProfilePrefix + "stig_elemental"}))
	})
	It("rejects files that are not tailoring files", func() {
		_, err := hardening.TailoringProfiles(strings.NewReader(`<Benchmark><Profile id="x"/></Benchmark>`))
		Expect(err).To(MatchError(ContainSubstring("not an XCCDF tailoring file")))

		_, err = hardening.TailoringProfiles(strings.NewReader(`<Tailoring></Tailoring>`))
		Expect(err).To(MatchError(ContainSubstring("defines no profiles")))
	})
})
