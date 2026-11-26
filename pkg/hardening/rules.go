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

package hardening

import (
	"fmt"
	"slices"
	"strings"
)

// kernelArgRule maps a ComplianceAsCode kernel argument rule to what Elemental
// does with the kernel command line instead of letting oscap edit the
// bootloader configuration (which Elemental manages itself). A rule either
// requires an argument (key and value or variable) or forbids some (forbidden).
type kernelArgRule struct {
	key string
	// value is used when variable is empty or the profile does not set it.
	value string
	// variable is the short ID of an XCCDF value holding the argument value.
	variable string
	// forbidden lists arguments that must not be on the command line. An entry
	// with "=" matches exactly. An entry without "=" matches the argument when
	// it is enabled: given bare, or with any value other than 0/no/false/off.
	forbidden []string
}

// kernelArgRules is keyed by short ComplianceAsCode rule ID.
//
// The SLES 16 entries match the rule IDs in ssg-sle16-ds.xml (SSG 0.1.81).
// Values and variable names must be checked against the rule remediations,
// e.g. with 'oscap xccdf generate fix --fix-type bash'.
var kernelArgRules = map[string]kernelArgRule{
	"grub2_audit_argument":                     {key: "audit", value: "1"},
	"grub2_audit_backlog_limit_argument":       {key: "audit_backlog_limit", value: "8192"},
	"grub2_enable_iommu_force":                 {key: "iommu", value: "force"},
	"grub2_l1tf_argument":                      {key: "l1tf", variable: "var_l1tf_options"},
	"grub2_mce_argument":                       {key: "mce", value: "0"},
	"grub2_mds_argument":                       {key: "mds", variable: "var_mds_options"},
	"grub2_page_alloc_shuffle_argument":        {key: "page_alloc.shuffle", value: "1"},
	"grub2_page_poison_argument":               {key: "page_poison", value: "1"},
	"grub2_pti_argument":                       {key: "pti", value: "on"},
	"grub2_rng_core_default_quality_argument":  {key: "rng_core.default_quality", value: "500"},
	"grub2_slab_nomerge_argument":              {key: "slab_nomerge", value: "yes"},
	"grub2_slub_debug_argument":                {key: "slub_debug", value: "P", variable: "var_slub_debug_options"},
	"grub2_spec_store_bypass_disable_argument": {key: "spec_store_bypass_disable", variable: "var_spec_store_bypass_disable_options"},
	"grub2_spectre_v2_argument":                {key: "spectre_v2", value: "on"},

	"grub2_disable_interactive_boot": {forbidden: []string{"systemd.confirm_spawn"}},
	"grub2_enable_selinux":           {forbidden: []string{"selinux=0", "enforcing=0"}},
	"grub2_nosmap_argument_absent":   {forbidden: []string{"nosmap"}},
	"grub2_nosmep_argument_absent":   {forbidden: []string{"nosmep"}},

	// Not in the SLES 16 datastream, kept for other SUSE products.
	"grub2_vsyscall_argument":     {key: "vsyscall", value: "none"},
	"grub2_init_on_free":          {key: "init_on_free", value: "1"},
	"grub2_ipv6_disable_argument": {key: "ipv6.disable", value: "1"},
}

const fipsRule = "grub2_enable_fips_mode"

// Plan is the outcome of classifying the rules selected by a profile.
type Plan struct {
	// KernelArgs are the "key=value" arguments the profile requires.
	KernelArgs []string
	// ForbiddenArgs are arguments the profile requires to be absent.
	ForbiddenArgs []string
	// RequiresFIPS is set when the profile selects the FIPS mode rule.
	RequiresFIPS bool
	// Skip lists full rule IDs oscap must not remediate inside the snapshot.
	Skip []string
	// Unhandled lists bootloader rules Elemental neither remediates nor maps.
	Unhandled []string
	// Packages lists package install/removal rules the image does not satisfy.
	// They must be satisfied by the OS image itself rather than by installing
	// or removing packages in a snapshot, so they are skipped and reported.
	Packages []string
}

// Classify sorts the rules selected by a profile into what Elemental handles
// itself, what oscap must skip and what is left to oscap remediation. rules
// maps full rule IDs to their scan result; rules the profile does not select
// have the result "notselected" and are ignored. values holds the profile's
// resolved XCCDF values (full IDs), as reported in scan results.
func Classify(rules map[string]string, values map[string]string) Plan {
	var p Plan
	ids := make([]string, 0, len(rules))
	for id, res := range rules {
		if res != ResultNotSelected {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)

	for _, id := range ids {
		short, ok := strings.CutPrefix(id, SSGRulePrefix)
		if !ok {
			continue
		}
		switch {
		case short == fipsRule:
			p.RequiresFIPS = true
			p.Skip = append(p.Skip, id)
		case isBootloaderRule(short):
			p.Skip = append(p.Skip, id)
			rule, known := kernelArgRules[short]
			if !known {
				p.Unhandled = append(p.Unhandled, id)
				continue
			}
			if len(rule.forbidden) > 0 {
				p.ForbiddenArgs = append(p.ForbiddenArgs, rule.forbidden...)
				continue
			}
			value := rule.value
			if rule.variable != "" {
				if v, set := values[SSGValuePrefix+rule.variable]; set && v != "" {
					value = v
				}
			}
			if value == "" {
				p.Unhandled = append(p.Unhandled, id)
				continue
			}
			p.KernelArgs = append(p.KernelArgs, rule.key+"="+value)
		case isPackageRule(short):
			// A package rule the image already satisfies needs no fix, so
			// leave it to oscap, which then reports it as passing.
			if res := rules[id]; res == ResultPass || res == ResultNotApplicable {
				continue
			}
			p.Skip = append(p.Skip, id)
			p.Packages = append(p.Packages, id)
		}
	}
	return p
}

// isBootloaderRule matches rules whose oscap remediation edits the bootloader
// configuration (kernel arguments, bootloader passwords, s390x zipl).
func isBootloaderRule(short string) bool {
	return strings.HasPrefix(short, "grub2_") || strings.HasPrefix(short, "zipl_")
}

// isPackageRule matches rules whose remediation installs or removes packages.
func isPackageRule(short string) bool {
	return strings.HasPrefix(short, "package_") &&
		(strings.HasSuffix(short, "_installed") || strings.HasSuffix(short, "_removed"))
}

// CheckForbiddenKernelArgs returns an error listing the arguments of cmdline
// that the profile requires to be absent.
func CheckForbiddenKernelArgs(cmdline string, forbidden []string) error {
	var found []string
	for _, f := range strings.Fields(cmdline) {
		for _, bad := range forbidden {
			if f == bad || (!strings.Contains(bad, "=") && isEnabled(f, bad)) {
				found = append(found, f)
				break
			}
		}
	}
	if len(found) > 0 {
		return fmt.Errorf("kernel command line sets %s, which the hardening profile forbids", strings.Join(found, " "))
	}
	return nil
}

// isEnabled reports whether arg sets key to a value that turns it on.
func isEnabled(arg, key string) bool {
	value, ok := strings.CutPrefix(arg, key+"=")
	if !ok {
		return false
	}
	switch strings.ToLower(value) {
	case "0", "no", "false", "off":
		return false
	}
	return true
}

// AppendKernelArgs appends the required arguments to cmdline. Arguments already
// present with the same value are not duplicated, so this is safe to call on
// every upgrade. An argument present with a different value is an error: the
// user's command line would leave the system non-compliant.
func AppendKernelArgs(cmdline string, args []string) (string, []string, error) {
	fields := strings.Fields(cmdline)
	var appended []string

	for _, arg := range args {
		key, _, _ := strings.Cut(arg, "=")
		present := false
		for _, f := range fields {
			fKey, _, _ := strings.Cut(f, "=")
			if fKey != key {
				continue
			}
			if f != arg {
				return "", nil, fmt.Errorf(
					"kernel command line sets %q but the hardening profile requires %q", f, arg)
			}
			present = true
		}
		if !present {
			fields = append(fields, arg)
			appended = append(appended, arg)
		}
	}
	return strings.Join(fields, " "), appended, nil
}
