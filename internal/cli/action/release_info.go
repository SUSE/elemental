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
	"context"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	cmdpkg "github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/internal/config"
	"github.com/suse/elemental/v3/pkg/extractor"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/api/product"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/manifest/source"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/urfave/cli/v3"
)

// to keep a track if user requested for markdown output
var markdown bool

type basicInfo struct {
	Name         string
	Version      string
	CreationDate string
	Source       string
}

func ReleaseInfo(_ context.Context, cmd *cli.Command) error {
	if cmd.Root().Metadata == nil || cmd.Root().Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	system := cmd.Root().Metadata["system"].(*sys.System)
	args := &cmdpkg.ReleaseInfoArgs

	system.Logger().Debug("release-info called with args: %+v", args)

	if cmd.Args() == nil || cmd.Args().Len() == 0 {
		system.Logger().Error("no file or OCI image provided")
		return fmt.Errorf("refer usage: %s", cmd.UsageText)
	}
	arg := cmd.Args().Get(0)
	srcType, err := argSourceType(system, arg)
	if err != nil {
		return err
	}
	system.Logger().Debug("found source type: %s", srcType)

	uri := arg
	if !strings.Contains(arg, "://") {
		uri = fmt.Sprintf("%s://%s", srcType, arg)
	}

	if srcType == source.OCI {
		// check if it's a valid OCI image before proceeding
		if _, err := name.ParseReference(uri); err != nil {
			return fmt.Errorf("invalid OCI image reference: %w", err)
		}
	}
	resolved, err := resolveManifest(system, uri, args.Local)
	if err != nil {
		return err
	}

	markdown = args.Markdown

	out := cmd.Writer
	if out == nil {
		out = cmd.Root().Writer
	}
	return printManifest(resolved, uri, out)
}

func resolveManifest(system *sys.System, uri string, local bool) (*resolver.ResolvedManifest, error) {
	output, err := config.NewOutput(system.FS(), "", "")
	if err != nil {
		return nil, err
	}
	defer func() {
		system.Logger().Debug("Cleaning up working directory")
		if rmErr := output.Cleanup(system.FS()); rmErr != nil {
			system.Logger().Error("Cleaning up working directory failed: %v", rmErr)
		}
	}()

	res, err := manifestResolver(system.FS(), output, local)
	if err != nil {
		return nil, err
	}

	return res.Resolve(uri)
}

// argSourceType takes a string argument and returns if the release manifest source type is a file or an OCI image
func argSourceType(s *sys.System, arg string) (source.ReleaseManifestSourceType, error) {
	if arg == "" {
		return 0, fmt.Errorf("no file or OCI image provided to release-info")
	}
	u, err := url.Parse(arg)
	if err != nil {
		return 0, err
	}
	if u.Scheme != "" {
		switch u.Scheme {
		case "file":
			return source.File, nil
		case "oci":
			return source.OCI, nil
		default:
			return 0, fmt.Errorf("encountered invalid schema %q; supported schemas: %q, %q", u.Scheme, "file", "oci")
		}
	}
	if ok, _ := vfs.Exists(s.FS(), arg); ok {
		return source.File, nil
	}
	return source.OCI, nil
}

func manifestResolver(fs vfs.FS, out config.Output, local bool) (*resolver.Resolver, error) {
	const (
		globPattern = "release_manifest*.yaml"
	)

	searchPaths := []string{
		globPattern,
		filepath.Join("etc", "release-manifest", globPattern),
	}

	manifestsDir := out.ReleaseManifestsStoreDir()
	if err := vfs.MkdirAll(fs, manifestsDir, 0700); err != nil {
		return nil, fmt.Errorf("creating release manifest store '%s': %w", manifestsDir, err)
	}

	extr, err := extractor.New(searchPaths, extractor.WithStore(manifestsDir), extractor.WithLocal(local), extractor.WithFS(fs))
	if err != nil {
		return nil, fmt.Errorf("initializing OCI release manifest extractor: %w", err)
	}

	return resolver.New(source.NewReader(extr)), nil
}

func printManifest(manifest *resolver.ResolvedManifest, arg string, out io.Writer) error {
	// initialize some essentials
	var cm *core.ReleaseManifest
	var pm *product.ReleaseManifest

	cm = manifest.CorePlatform
	if manifest.ProductExtension != nil {
		pm = manifest.ProductExtension
	}

	if err := printBasicData(cm, pm, arg, out); err != nil {
		return err
	}

	// infrastructure component data
	if err := printInfraData(cm, out); err != nil {
		return err
	}

	// systemd extensions info
	if err := printSystemdData(cm, pm, out); err != nil {
		return err
	}

	// helm charts
	if err := printHelmChartsData(cm, pm, out); err != nil {
		return err
	}

	return nil
}

func newTable(markdown bool, out io.Writer) *tablewriter.Table {
	tableConfig := tablewriter.Config{
		Header: tw.CellConfig{
			Alignment: tw.CellAlignment{
				Global: tw.AlignCenter,
			},
		},
		Row: tw.CellConfig{
			Alignment: tw.CellAlignment{Global: tw.AlignLeft},
			Merging:   tw.CellMerging{Mode: tw.MergeHierarchical},
		},
	}

	if markdown {
		return tablewriter.NewTable(out, tablewriter.WithConfig(tableConfig), tablewriter.WithRenderer(renderer.NewMarkdown()))
	}
	return tablewriter.NewTable(out, tablewriter.WithConfig(tableConfig))
}

// most basic information that shall be printed for all the optional flags
func printBasicData(cm *core.ReleaseManifest, pm *product.ReleaseManifest, arg string, out io.Writer) error {
	var data [][]string
	var cmBasic, pmBasic *basicInfo
	table := newTable(markdown, out)

	cmBasic = &basicInfo{
		Name:         cm.Metadata.Name,
		Version:      cm.Metadata.Version,
		CreationDate: cm.Metadata.CreationDate,
		Source:       arg,
	}
	if pm != nil {
		// we are dealing with a product manifest
		pmBasic = &basicInfo{
			Name:         pm.Metadata.Name,
			Version:      pm.Metadata.Version,
			CreationDate: pm.Metadata.CreationDate,
			Source:       arg,
		}
		cmBasic.Source = pm.CorePlatform.Image

		table.Header([]string{"Attribute", "Core Platform (Base)", "Product Manifest (Extension)"})
		data = append(data, []string{"Name", cmBasic.Name, pmBasic.Name})
		data = append(data, []string{"Version", cmBasic.Version, pm.Metadata.Version})
		data = append(data, []string{"Release Date", cmBasic.CreationDate, pm.Metadata.CreationDate})
		data = append(data, []string{"Source", cmBasic.Source, pmBasic.Source})
	} else {
		// we are dealing with a core manifest
		table.Header([]string{"Attribute", "Core Platform (Base)"})
		data = append(data, []string{"Name", cmBasic.Name})
		data = append(data, []string{"Version", cmBasic.Version})
		data = append(data, []string{"Release Data", cmBasic.CreationDate})
		data = append(data, []string{"Source", cmBasic.Source})

	}
	return printAndClearData(table, data, out)
}

func printInfraData(cm *core.ReleaseManifest, out io.Writer) error {
	var data [][]string
	table := newTable(markdown, out)
	table.Header([]string{"Infrastructure Components", "Version", "Source"})

	osVersion := "Unknown"
	if cm.Components.OperatingSystem.Image.Base != "" {
		parts := strings.Split(cm.Components.OperatingSystem.Image.Base, ":")
		if len(parts) > 1 {
			osVersion = "SLES " + strings.Split(parts[1], "-")[0]
		}
		data = append(data, []string{"Operating System", osVersion, cm.Components.OperatingSystem.Image.Base})
	}

	if cm.Components.Kubernetes != nil {
		data = append(data, []string{"Kubernetes", cm.Components.Kubernetes.Version, cm.Components.Kubernetes.Image})
	}

	return printAndClearData(table, data, out)
}

func printSystemdData(cm *core.ReleaseManifest, pm *product.ReleaseManifest, out io.Writer) error {
	var data [][]string
	var table *tablewriter.Table

	if pm != nil {
		table = newTable(markdown, out)
		table.Header([]string{"Systemd Extensions", "Image Reference"})

		for _, s := range cm.Components.Systemd.Extensions {
			data = append(data, []string{s.Name, s.Image})
		}
		for _, s := range pm.Components.Systemd.Extensions {
			data = append(data, []string{s.Name + "(*)", s.Image})
		}
	} else if len(cm.Components.Systemd.Extensions) > 0 {
		table = newTable(markdown, out)
		table.Header([]string{"Systemd Extensions", "Image Reference"})

		for _, s := range cm.Components.Systemd.Extensions {
			data = append(data, []string{s.Name, s.Image})
		}
	}
	return printAndClearData(table, data, out)
}

func printHelmChartsData(cm *core.ReleaseManifest, pm *product.ReleaseManifest, out io.Writer) error {
	var data [][]string
	var table *tablewriter.Table

	if pm != nil {
		if cm.Components.Helm != nil && len(cm.Components.Helm.Charts) > 0 {
			table = newTable(markdown, out)
			table.Header([]string{"Chart Name", "Version", "Repository", "Target Namespace", "Depends On"})

			data = coreManifestHelmChartsData(cm.Components.Helm)
		}
		if pm.Components.Helm != nil && len(pm.Components.Helm.Charts) > 0 {
			pmRepos := repoUrls(pm.Components.Helm.Repositories)
			for _, c := range pm.Components.Helm.Charts {
				if len(c.DependsOn) > 0 {
					for _, v := range c.DependsOn {
						dependsOn := fmt.Sprintf("%s (%s)", v.Name, v.Type)
						data = append(data, []string{c.GetName() + "(*)", c.Version, pmRepos[c.Repository], c.Namespace, dependsOn})
					}
				} else {
					data = append(data, []string{c.GetName() + "(*)", c.Version, pmRepos[c.Repository], c.Namespace, "-"})
				}
			}

		}
	} else if cm.Components.Helm != nil && len(cm.Components.Helm.Charts) > 0 {
		table = newTable(markdown, out)
		table.Header([]string{"Chart Name", "Version", "Repository", "Target Namespace", "Depends On"})

		data = coreManifestHelmChartsData(cm.Components.Helm)

	}
	return printAndClearData(table, data, out)
}

func coreManifestHelmChartsData(h *api.Helm) [][]string {
	var data [][]string

	cmRepos := repoUrls(h.Repositories)
	for _, c := range h.Charts {
		if len(c.DependsOn) > 0 {
			for _, v := range c.DependsOn {
				dependsOn := fmt.Sprintf("%s (%s)", v.Name, v.Type)
				data = append(data, []string{c.GetName(), c.Version, cmRepos[c.Repository], c.Namespace, dependsOn})
			}
		} else {
			data = append(data, []string{c.GetName(), c.Version, cmRepos[c.Repository], c.Namespace, "-"})
		}
	}

	return data
}

// printAndClearData prints and clears data after every stage
func printAndClearData(table *tablewriter.Table, data [][]string, out io.Writer) error {
	if len(data) == 0 {
		return nil
	}

	var err error
	if err = table.Bulk(data); err != nil {
		return err
	}

	if err = table.Render(); err != nil {
		return err
	}
	clear(data)
	fmt.Fprintln(out)

	return nil
}

func repoUrls(repos []*api.HelmRepository) map[string]string {
	mapping := map[string]string{}

	for _, r := range repos {
		mapping[r.Name] = r.URL
	}

	return mapping
}
