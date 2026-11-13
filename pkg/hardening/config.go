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

// Package hardening applies SCAP security profiles (DISA STIG, ANSSI, CIS, ...)
// or XCCDF tailoring files to an OS snapshot before it is sealed read-only.
package hardening

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// SSGProfilePrefix is the XCCDF ID prefix used by ComplianceAsCode profiles.
	SSGProfilePrefix = "xccdf_org.ssgproject.content_profile_"
	// SSGRulePrefix is the XCCDF ID prefix used by ComplianceAsCode rules.
	SSGRulePrefix = "xccdf_org.ssgproject.content_rule_"
	// SSGValuePrefix is the XCCDF ID prefix used by ComplianceAsCode values.
	SSGValuePrefix = "xccdf_org.ssgproject.content_value_"

	// InImageTailoring is where a user supplied tailoring file is stored inside
	// the installed system. /etc is snapshotted and merged on upgrades, so the
	// file is carried over to every new snapshot.
	InImageTailoring = "/etc/elemental/hardening/tailoring.xml"

	// ResultsDir is where scan results and reports are written inside the
	// installed system. /var is shared across snapshots.
	ResultsDir = "/var/log/elemental/hardening"

	// DefaultDatastreamDir is where scap-security-guide installs its datastreams.
	DefaultDatastreamDir = "/usr/share/xml/scap/ssg/content"

	oscapBin = "/usr/bin/oscap"
)

var idRegexp = regexp.MustCompile(`^[A-Za-z0-9_.\-]+$`)

// Config describes the hardening profile applied to a deployment. It is shared
// by the image definition (install.yaml) and the deployment description.
type Config struct {
	// Profile is the XCCDF profile ID. Short ComplianceAsCode IDs such as
	// "stig" or "anssi_bp28_high" are expanded to their full form. When a
	// tailoring file is given, this is the ID of a profile defined in it and
	// may be omitted if the tailoring file defines exactly one profile.
	Profile string `yaml:"profile,omitempty"`
	// Datastream is the absolute path of the SCAP datastream inside the OS
	// image. If empty, the single datastream found in DefaultDatastreamDir is used.
	Datastream string `yaml:"datastream,omitempty"`
	// Tailoring is the absolute path to an XCCDF tailoring file. At build time
	// this is a path on the build host; once installed it is InImageTailoring.
	Tailoring string `yaml:"tailoring,omitempty"`
}

// Enabled reports whether a hardening profile has been requested.
func (c *Config) Enabled() bool {
	return c != nil && (c.Profile != "" || c.Tailoring != "")
}

// Validate checks the configuration is well-formed. It does not touch the
// filesystem; file checks happen when the configuration is parsed and applied.
func (c *Config) Validate() error {
	if !c.Enabled() {
		if c != nil && c.Datastream != "" {
			return fmt.Errorf("hardening datastream given without a profile or tailoring file")
		}
		return nil
	}
	if c.Profile != "" && !idRegexp.MatchString(c.Profile) {
		return fmt.Errorf("invalid hardening profile ID %q", c.Profile)
	}
	if c.Datastream != "" && !filepath.IsAbs(c.Datastream) {
		return fmt.Errorf("hardening datastream must be an absolute path inside the OS image, got %q", c.Datastream)
	}
	if c.Tailoring != "" && !filepath.IsAbs(c.Tailoring) {
		return fmt.Errorf("hardening tailoring file must be an absolute path, got %q", c.Tailoring)
	}
	return nil
}

// NormalizeProfileID expands short ComplianceAsCode profile IDs ("stig") to
// their full XCCDF form. Fully qualified IDs are returned unchanged.
func NormalizeProfileID(id string) string {
	if id == "" || strings.HasPrefix(id, "xccdf_") {
		return id
	}
	return SSGProfilePrefix + id
}

// ResolveProfile returns the profile to evaluate, given the configured profile
// and the profiles defined in the tailoring file (if any).
func ResolveProfile(configured string, tailored []string) (string, error) {
	profile := NormalizeProfileID(configured)
	if len(tailored) == 0 {
		if profile == "" {
			return "", fmt.Errorf("no hardening profile configured")
		}
		return profile, nil
	}
	if profile == "" {
		if len(tailored) == 1 {
			return tailored[0], nil
		}
		return "", fmt.Errorf("tailoring file defines several profiles (%s), set 'profile' to choose one",
			strings.Join(tailored, ", "))
	}
	for _, p := range tailored {
		if p == profile {
			return profile, nil
		}
	}
	return "", fmt.Errorf("profile %q is not defined in the tailoring file (found: %s)",
		profile, strings.Join(tailored, ", "))
}
