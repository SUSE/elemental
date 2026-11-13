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
	"encoding/xml"
	"errors"
	"fmt"
	"io"

	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

// Possible XCCDF rule results, see the XCCDF 1.2 specification.
const (
	ResultPass          = "pass"
	ResultFail          = "fail"
	ResultError         = "error"
	ResultUnknown       = "unknown"
	ResultNotApplicable = "notapplicable"
	ResultNotChecked    = "notchecked"
	ResultNotSelected   = "notselected"
	ResultInformational = "informational"
	ResultFixed         = "fixed"
)

// ScanResult is the subset of an XCCDF TestResult Elemental cares about.
type ScanResult struct {
	// Rules maps full rule IDs to their result.
	Rules map[string]string
	// Values maps full value IDs to the value the profile resolved them to.
	Values map[string]string
}

// Selected returns the IDs of all rules selected by the evaluated profile.
func (r ScanResult) Selected() []string {
	var ids []string
	for id, res := range r.Rules {
		if res != ResultNotSelected {
			ids = append(ids, id)
		}
	}
	return ids
}

// Count returns the number of rules with each result.
func (r ScanResult) Count() map[string]int {
	c := map[string]int{}
	for _, res := range r.Rules {
		c[res]++
	}
	return c
}

type xmlTestResult struct {
	SetValues []struct {
		IDRef string `xml:"idref,attr"`
		Value string `xml:",chardata"`
	} `xml:"set-value"`
	RuleResults []struct {
		IDRef  string `xml:"idref,attr"`
		Result string `xml:"result"`
	} `xml:"rule-result"`
}

// ParseResults reads an XCCDF results file as written by 'oscap xccdf eval --results'.
// Results files embed the whole benchmark and can be large, so the document is
// streamed and only the (last) TestResult element is decoded.
func ParseResults(r io.Reader) (*ScanResult, error) {
	dec := xml.NewDecoder(r)
	var found *xmlTestResult
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing XCCDF results: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "TestResult" {
			continue
		}
		tr := &xmlTestResult{}
		if err := dec.DecodeElement(tr, &se); err != nil {
			return nil, fmt.Errorf("decoding XCCDF TestResult: %w", err)
		}
		found = tr
	}
	if found == nil {
		return nil, fmt.Errorf("no TestResult found in XCCDF results")
	}

	res := &ScanResult{Rules: map[string]string{}, Values: map[string]string{}}
	for _, v := range found.SetValues {
		res.Values[v.IDRef] = v.Value
	}
	for _, rr := range found.RuleResults {
		res.Rules[rr.IDRef] = rr.Result
	}
	return res, nil
}

// ParseResultsFile is ParseResults for a file on the given filesystem.
func ParseResultsFile(fs vfs.FS, path string) (*ScanResult, error) {
	f, err := fs.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening XCCDF results %q: %w", path, err)
	}
	defer f.Close()
	return ParseResults(f)
}

// TailoringProfiles returns the IDs of the profiles defined in an XCCDF tailoring file.
func TailoringProfiles(r io.Reader) ([]string, error) {
	dec := xml.NewDecoder(r)
	var ids []string
	sawTailoring := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing XCCDF tailoring file: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "Tailoring":
			sawTailoring = true
		case "Profile":
			for _, a := range se.Attr {
				if a.Name.Local == "id" && a.Value != "" {
					ids = append(ids, a.Value)
				}
			}
		}
	}
	if !sawTailoring {
		return nil, fmt.Errorf("not an XCCDF tailoring file: no Tailoring element found")
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("XCCDF tailoring file defines no profiles")
	}
	return ids, nil
}

// TailoringProfilesFile is TailoringProfiles for a file on the given filesystem.
func TailoringProfilesFile(fs vfs.FS, path string) ([]string, error) {
	f, err := fs.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening tailoring file %q: %w", path, err)
	}
	defer f.Close()
	return TailoringProfiles(f)
}
