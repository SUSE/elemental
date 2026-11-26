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
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/suse/elemental/v3/pkg/chroot"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

// oscap exit codes: 0 all rules passed, 2 at least one rule failed, 1 error.
const oscapExitFailedRules = 2

// maxListed caps how many rule IDs are listed in a single log message.
const maxListed = 20

// Result summarises a hardening run.
type Result struct {
	// Profile is the fully qualified profile ID that was applied.
	Profile string
	// Tailoring is the tailoring file path inside the image, if any.
	Tailoring string
	// Plan is the classification of the selected rules.
	Plan Plan
	// Counts is the number of rules per result after remediation.
	Counts map[string]int
	// Failed lists rules still failing after remediation.
	Failed []string
	// ResultsDir is where results and the HTML report were written, inside the image.
	ResultsDir string
}

// Apply hardens the OS tree at root according to cfg. It must run while root
// is still writable, i.e. before the snapshot is locked.
//
// It runs oscap twice inside a chroot of root:
//  1. a scan to resolve which rules and values the profile selects;
//  2. a remediation run that skips rules Elemental handles itself: kernel
//     arguments (returned in Result.Plan.KernelArgs for the caller to add to the
//     bootloader configuration), FIPS mode (handled by the crypto policy) and
//     package installation/removal (which belongs in the OS image).
//
// Rules that still fail after remediation are reported, not treated as errors:
// many profiles contain rules that cannot be met at build time (partitioning,
// site specific settings) and the report is the record of what is outstanding.
func Apply(ctx context.Context, s *sys.System, root string, cfg *Config) (*Result, error) {
	if !cfg.Enabled() {
		return nil, fmt.Errorf("hardening is not configured")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	fs := s.FS()

	if ok, _ := vfs.Exists(fs, filepath.Join(root, oscapBin)); !ok {
		return nil, fmt.Errorf("%s not found in the OS image: the image must include openscap and scap-security-guide to apply a hardening profile", oscapBin)
	}

	datastream, err := findDatastream(fs, root, cfg.Datastream)
	if err != nil {
		return nil, err
	}

	res := &Result{}
	var tailored []string
	res.Tailoring, tailored, err = installTailoring(fs, root, cfg.Tailoring)
	if err != nil {
		return nil, err
	}

	res.Profile, err = ResolveProfile(cfg.Profile, tailored)
	if err != nil {
		return nil, err
	}

	res.ResultsDir = filepath.Join(ResultsDir, time.Now().UTC().Format("20060102T150405Z"))
	if err := vfs.MkdirAll(fs, filepath.Join(root, res.ResultsDir), vfs.DirPerm); err != nil {
		return nil, fmt.Errorf("creating hardening results directory: %w", err)
	}

	common := []string{"xccdf", "eval", "--profile", res.Profile}
	if res.Tailoring != "" {
		common = append(common, "--tailoring-file", res.Tailoring)
	}

	if res.Plan, err = plan(ctx, s, root, res, common, datastream); err != nil {
		return nil, err
	}
	if err = remediate(ctx, s, root, res, common, datastream); err != nil {
		return nil, err
	}
	return res, nil
}

// installTailoring copies a tailoring file from the build host into the image,
// unless it is already there, and returns its in-image path and profiles.
func installTailoring(fs vfs.FS, root, tailoring string) (string, []string, error) {
	if tailoring == "" {
		return "", nil, nil
	}
	target := filepath.Join(root, InImageTailoring)
	if tailoring != InImageTailoring {
		if err := vfs.MkdirAll(fs, filepath.Dir(target), vfs.DirPerm); err != nil {
			return "", nil, fmt.Errorf("creating tailoring directory: %w", err)
		}
		if err := vfs.CopyFile(fs, tailoring, target); err != nil {
			return "", nil, fmt.Errorf("copying tailoring file %q into the image: %w", tailoring, err)
		}
	}
	profiles, err := TailoringProfilesFile(fs, target)
	if err != nil {
		return "", nil, err
	}
	return InImageTailoring, profiles, nil
}

// plan scans the image to find which rules and values the profile selects and
// classifies them.
func plan(ctx context.Context, s *sys.System, root string, res *Result, common []string, datastream string) (Plan, error) {
	log := s.Logger()
	log.Info("Hardening: scanning image against profile %s (datastream %s)", res.Profile, datastream)

	scanFile := filepath.Join(res.ResultsDir, "scan-before.xml")
	args := append(slices.Clone(common), "--results", scanFile, datastream)
	if err := runOscap(ctx, s, root, args); err != nil {
		return Plan{}, fmt.Errorf("scanning image: %w", err)
	}
	scan, err := ParseResultsFile(s.FS(), filepath.Join(root, scanFile))
	if err != nil {
		return Plan{}, err
	}

	p := Classify(scan.Rules, scan.Values)
	if len(p.Unhandled) > 0 {
		log.Warn("Hardening: bootloader rules not handled by Elemental, these will need a documented exception or manual configuration: %s",
			listIDs(p.Unhandled))
	}
	if len(p.Packages) > 0 {
		log.Warn("Hardening: the OS image does not satisfy these package rules; packages are not installed or removed inside the snapshot, so satisfy them in the OS image or deselect them in a tailoring file: %s",
			listIDs(p.Packages))
	}
	return p, nil
}

// remediate applies the profile, skipping the rules in res.Plan.Skip, and
// records what is still failing.
func remediate(ctx context.Context, s *sys.System, root string, res *Result, common []string, datastream string) error {
	log := s.Logger()
	log.Info("Hardening: remediating image against profile %s", res.Profile)

	resultsFile := filepath.Join(res.ResultsDir, "results.xml")
	report := filepath.Join(res.ResultsDir, "report.html")
	args := append(slices.Clone(common), "--remediate", "--results", resultsFile, "--report", report)
	for _, id := range res.Plan.Skip {
		args = append(args, "--skip-rule", id)
	}
	args = append(args, datastream)
	if err := runOscap(ctx, s, root, args); err != nil {
		return fmt.Errorf("remediating image: %w", err)
	}

	final, err := ParseResultsFile(s.FS(), filepath.Join(root, resultsFile))
	if err != nil {
		return err
	}
	res.Counts = final.Count()
	for id, r := range final.Rules {
		if r == ResultFail || r == ResultError {
			res.Failed = append(res.Failed, id)
		}
	}
	slices.Sort(res.Failed)

	log.Info("Hardening: %d passed, %d fixed, %d failed, %d errors, %d not applicable; report at %s",
		res.Counts[ResultPass], res.Counts[ResultFixed], res.Counts[ResultFail], res.Counts[ResultError],
		res.Counts[ResultNotApplicable], report)
	if len(res.Failed) > 0 {
		log.Warn("Hardening: rules still failing after remediation: %s", listIDs(res.Failed))
	}
	return nil
}

// runOscap runs oscap inside a chroot of root. Exit code 2 (some rules failed)
// is expected and not an error.
func runOscap(ctx context.Context, s *sys.System, root string, args []string) error {
	callback := func() error {
		out, err := s.Runner().RunContext(ctx, oscapBin, args...)
		s.Logger().Debug("oscap output:\n%s", string(out))
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == oscapExitFailedRules {
			return nil
		}
		return err
	}
	return chroot.ChrootedCallback(s, root, nil, callback)
}

// findDatastream returns the datastream path inside the image.
func findDatastream(fs vfs.FS, root, configured string) (string, error) {
	if configured != "" {
		if ok, _ := vfs.Exists(fs, filepath.Join(root, configured)); !ok {
			return "", fmt.Errorf("hardening datastream %q not found in the OS image", configured)
		}
		return configured, nil
	}

	entries, err := fs.ReadDir(filepath.Join(root, DefaultDatastreamDir))
	if err != nil {
		return "", fmt.Errorf("no SCAP datastreams found in the OS image under %s; install scap-security-guide or set 'datastream': %w",
			DefaultDatastreamDir, err)
	}
	var found []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "ssg-") && strings.HasSuffix(e.Name(), "-ds.xml") {
			found = append(found, filepath.Join(DefaultDatastreamDir, e.Name()))
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no SCAP datastreams found in the OS image under %s; install scap-security-guide or set 'datastream'",
			DefaultDatastreamDir)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("several SCAP datastreams found in the OS image, set 'datastream' to one of: %s",
			strings.Join(found, ", "))
	}
}

func listIDs(ids []string) string {
	short := make([]string, 0, min(len(ids), maxListed))
	for _, id := range ids[:min(len(ids), maxListed)] {
		short = append(short, strings.TrimPrefix(id, SSGRulePrefix))
	}
	s := strings.Join(short, ", ")
	if len(ids) > maxListed {
		s += fmt.Sprintf(" (and %d more, see the report)", len(ids)-maxListed)
	}
	return s
}
