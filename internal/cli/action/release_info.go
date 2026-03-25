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
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"go.yaml.in/yaml/v3"

	cmdpkg "github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/internal/config"
	"github.com/suse/elemental/v3/pkg/extractor"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/api/product"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/manifest/source"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/urfave/cli/v3"
)

// ManifestInfo holds the structured information for display
type ManifestInfo struct {
	Core    *CoreInfo    `json:"core,omitempty" yaml:"core,omitempty"`
	Product *ProductInfo `json:"product,omitempty" yaml:"product,omitempty"`
}

// CoreInfo represents core manifest details
type CoreInfo struct {
	Name              string   `json:"name" yaml:"name"`
	Version           string   `json:"version" yaml:"version"`
	OperatingSystem   string   `json:"operatingSystem" yaml:"operatingSystem"`
	HelmCharts        []string `json:"helmCharts,omitempty" yaml:"helmCharts,omitempty"`
	HelmRepos         []string `json:"helmRepos,omitempty" yaml:"helmRepos,omitempty"`
	SystemdExtensions []string `json:"systemdExtensions,omitempty" yaml:"systemdExtensions,omitempty"`
}

// ProductInfo represents product manifest details
type ProductInfo struct {
	Name              string   `json:"name" yaml:"name"`
	Version           string   `json:"version" yaml:"version"`
	SystemdExtensions []string `json:"systemdExtensions,omitempty" yaml:"systemdExtensions,omitempty"`
	HelmCharts        []string `json:"helmCharts,omitempty" yaml:"helmCharts,omitempty"`
	HelmRepos         []string `json:"helmRepos,omitempty" yaml:"helmRepos,omitempty"`
}

func ReleaseInfo(_ context.Context, cmd *cli.Command) error {
	if cmd.Root().Metadata == nil || cmd.Root().Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	system := cmd.Root().Metadata["system"].(*sys.System)
	args := &cmdpkg.ReleaseInfoArgs

	if cmd.Args() == nil || cmd.Args().Len() == 0 {
		system.Logger().Error("no file or OCI image provided")
		return fmt.Errorf("refer usage: %s", cmd.UsageText)
	}
	system.Logger().Debug("release-info called with args: %+v", args)

	arg := cmd.Args().First()
	resolved, err := resolveManifest(system, arg, args.Local)
	if err != nil {
		return err
	}

	info := buildManifestInfo(resolved, args.Core, args.Product)

	return printManifestInfo(info, args.Output, system.Logger().GetOutput())
}

func resolveManifest(system *sys.System, arg string, local bool) (*resolver.ResolvedManifest, error) {
	srcType, err := argSourceType(system, arg)
	if err != nil {
		return nil, err
	}
	system.Logger().Debug("found source type: %s", srcType)

	uri := arg
	if !strings.Contains(arg, "://") {
		uri = fmt.Sprintf("%s://%s", srcType, arg)
	}

	if srcType == source.OCI {
		// check if it's a valid OCI image before proceeding
		if _, err := name.ParseReference(uri); err != nil {
			return nil, fmt.Errorf("invalid OCI image reference: %w", err)
		}
	}

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

func buildManifestInfo(resolved *resolver.ResolvedManifest, showCore, showProduct bool) *ManifestInfo {
	info := &ManifestInfo{}

	// If neither core nor product is specified, or both are specified, show both
	defaultShow := (!showCore && !showProduct) || (showCore && showProduct)

	if (showCore || defaultShow) && resolved.CorePlatform != nil {
		info.Core = mapCoreInfo(resolved.CorePlatform)
	}

	if (showProduct || defaultShow) && resolved.ProductExtension != nil {
		info.Product = mapProductInfo(resolved.ProductExtension)
	}

	return info
}

func printManifestInfo(info *ManifestInfo, format string, out io.Writer) error {
	switch strings.ToLower(format) {
	case "json":
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(info)
	case "yaml":
		encoder := yaml.NewEncoder(out)
		return encoder.Encode(info)
	case "":
		printTable(info, out)
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
}

func printTable(info *ManifestInfo, out io.Writer) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	if info.Product != nil {
		fmt.Fprintf(w, "Product Manifest\n%s\n", strings.Repeat("-", 16))
		fmt.Fprintf(w, "Name\t%s\n", info.Product.Name)
		fmt.Fprintf(w, "Version\t%s\n", info.Product.Version)
		if len(info.Product.SystemdExtensions) > 0 {
			fmt.Fprintf(w, "Systemd Extensions\t%s\n", strings.Join(info.Product.SystemdExtensions, ", "))
		}
		if len(info.Product.HelmCharts) > 0 {
			fmt.Fprintf(w, "Helm Charts\t%s\n", strings.Join(info.Product.HelmCharts, ", "))
		}
		if len(info.Product.HelmRepos) > 0 {
			fmt.Fprintf(w, "Helm Repositories\t%s\n", strings.Join(info.Product.HelmRepos, ", "))
		}
		fmt.Fprintln(w)
	}

	if info.Core != nil {
		fmt.Fprintf(w, "Core Manifest\n%s\n", strings.Repeat("-", 13))
		fmt.Fprintf(w, "Name\t%s\n", info.Core.Name)
		fmt.Fprintf(w, "Version\t%s\n", info.Core.Version)
		fmt.Fprintf(w, "Operating System\t%s\n", info.Core.OperatingSystem)
		if len(info.Core.SystemdExtensions) > 0 {
			fmt.Fprintf(w, "Systemd Extensions\t%s\n", strings.Join(info.Core.SystemdExtensions, ", "))
		}
		if len(info.Core.HelmCharts) > 0 {
			fmt.Fprintf(w, "Helm Charts\t%s\n", strings.Join(info.Core.HelmCharts, ", "))
		}
		if len(info.Core.HelmRepos) > 0 {
			fmt.Fprintf(w, "Helm Repositories\t%s\n", strings.Join(info.Core.HelmRepos, ", "))
		}
		fmt.Fprintln(w)
	}

	w.Flush()
}

func mapCoreInfo(cm *core.ReleaseManifest) *CoreInfo {
	osName := "Unknown"
	if cm.Components.OperatingSystem.Image.Base != "" {
		parts := strings.Split(cm.Components.OperatingSystem.Image.Base, ":")
		if len(parts) > 1 {
			osName = "SLES " + strings.Split(parts[1], "-")[0]
		}
	}

	return &CoreInfo{
		Name:              cm.Metadata.Name,
		Version:           cm.Metadata.Version,
		OperatingSystem:   osName,
		HelmCharts:        mapHelmCharts(cm.Components.Helm.Charts),
		HelmRepos:         mapHelmRepos(cm.Components.Helm.Repositories),
		SystemdExtensions: mapSystemdExtensions(cm.Components.Systemd.Extensions),
	}
}

func mapProductInfo(pm *product.ReleaseManifest) *ProductInfo {
	return &ProductInfo{
		Name:              pm.Metadata.Name,
		Version:           pm.Metadata.Version,
		SystemdExtensions: mapSystemdExtensions(pm.Components.Systemd.Extensions),
		HelmCharts:        mapHelmCharts(pm.Components.Helm.Charts),
		HelmRepos:         mapHelmRepos(pm.Components.Helm.Repositories),
	}
}

func mapHelmCharts(charts []*api.HelmChart) []string {
	var result []string
	for _, h := range charts {
		result = append(result, fmt.Sprintf("%s-%s (%s)", h.GetName(), h.Version, h.GetRepositoryName()))
	}
	return result
}

func mapHelmRepos(repos []*api.HelmRepository) []string {
	var result []string
	for _, r := range repos {
		result = append(result, r.Name)
	}
	return result
}

func mapSystemdExtensions(exts []api.SystemdExtension) []string {
	var result []string
	for _, e := range exts {
		result = append(result, e.Name)
	}
	return result
}

// argSourceType takes a string argument and returns if the release manifest source type is a file or an OCI image
func argSourceType(s *sys.System, arg string) (source.ReleaseManifestSourceType, error) {
	if arg == "" {
		return 0, fmt.Errorf("no file or OCI image provided to release-info")
	}
	u, err := url.Parse(arg)
	if err == nil && u.Scheme != "" {
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

func Diff(_ context.Context, cmd *cli.Command) error {
	if cmd.Root().Metadata == nil || cmd.Root().Metadata["system"] == nil {
		return fmt.Errorf("setting up initial configuration")
	}
	system := cmd.Root().Metadata["system"].(*sys.System)
	args := &cmdpkg.ReleaseInfoArgs

	if cmd.Args() == nil || cmd.Args().Len() != 2 {
		system.Logger().Error("insufficient files or OCI images provided for diff")
		return fmt.Errorf("refer usage: %s", cmd.UsageText)
	}
	system.Logger().Debug("diff called with args: %+v", cmd.Args())

	m1, m2 := cmd.Args().Get(0), cmd.Args().Get(1)
	r1, err := resolveManifest(system, m1, args.Local)
	if err != nil {
		system.Logger().Error("resolving manifest %q", m1)
		return err
	}
	r2, err := resolveManifest(system, m2, args.Local)
	if err != nil {
		system.Logger().Error("resolving manifest %q", m2)
		return err
	}

	manifestInfo1 := buildManifestInfo(r1, args.Core, args.Product)
	manifestInfo2 := buildManifestInfo(r2, args.Core, args.Product)

	printDiff(manifestInfo1, manifestInfo2, system.Logger().GetOutput())

	return nil
}

func printDiff(info1, info2 *ManifestInfo, out io.Writer) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	if info1.Core != nil || info2.Core != nil {
		title1 := getStringOrEmpty(info1.Core, func(p *CoreInfo) string { return p.Name }) + " " +
			getStringOrEmpty(info1.Core, func(p *CoreInfo) string { return p.Version })
		title2 := getStringOrEmpty(info2.Core, func(p *CoreInfo) string { return p.Name }) + " " +
			getStringOrEmpty(info2.Core, func(p *CoreInfo) string { return p.Version })
		fmt.Fprintf(w, "Core Manifest\n%s\n", strings.Repeat("-", 13))
		fmt.Fprintf(w, "Field\t%s\t%s\n", title1, title2)
		fmt.Fprintf(w, "%s\t%s\t%s\n", strings.Repeat("-", 5), strings.Repeat("-", len(title1)), strings.Repeat("-", len(title2)))

		// Operating System
		os1 := getStringOrEmpty(info1.Core, func(c *CoreInfo) string { return c.OperatingSystem })
		os2 := getStringOrEmpty(info2.Core, func(c *CoreInfo) string { return c.OperatingSystem })
		fmt.Fprintf(w, "Operating System\t%s\t%s\n", os1, os2)

		// Systemd Extensions
		printFieldComparison(w, "Systemd Extensions",
			getSliceOrEmpty(info1.Core, func(c *CoreInfo) []string { return c.SystemdExtensions }),
			getSliceOrEmpty(info2.Core, func(c *CoreInfo) []string { return c.SystemdExtensions }))

		// Helm Charts
		printFieldComparison(w, "Helm Charts",
			getSliceOrEmpty(info1.Core, func(c *CoreInfo) []string { return c.HelmCharts }),
			getSliceOrEmpty(info2.Core, func(c *CoreInfo) []string { return c.HelmCharts }))

		// Helm Repositories
		printFieldComparison(w, "Helm Repositories",
			getSliceOrEmpty(info1.Core, func(c *CoreInfo) []string { return c.HelmRepos }),
			getSliceOrEmpty(info2.Core, func(c *CoreInfo) []string { return c.HelmRepos }))

		fmt.Fprintln(w)
	}

	// Show Product comparison if either manifest has Product info
	if info1.Product != nil || info2.Product != nil {
		title1 := getStringOrEmpty(info1.Product, func(p *ProductInfo) string { return p.Name }) + " " +
			getStringOrEmpty(info1.Product, func(p *ProductInfo) string { return p.Version })
		title2 := getStringOrEmpty(info2.Product, func(p *ProductInfo) string { return p.Name }) + " " +
			getStringOrEmpty(info2.Product, func(p *ProductInfo) string { return p.Version })
		fmt.Fprintf(w, "Product Manifest\n%s\n", strings.Repeat("-", 16))
		fmt.Fprintf(w, "Field\t%s\t%s\n", title1, title2)
		fmt.Fprintf(w, "%s\t%s\t%s\n", strings.Repeat("-", 5), strings.Repeat("-", len(title1)), strings.Repeat("-", len(title2)))

		// Systemd Extensions
		printFieldComparison(w, "Systemd Extensions",
			getSliceOrEmpty(info1.Product, func(p *ProductInfo) []string { return p.SystemdExtensions }),
			getSliceOrEmpty(info2.Product, func(p *ProductInfo) []string { return p.SystemdExtensions }))

		// Helm Charts
		printFieldComparison(w, "Helm Charts",
			getSliceOrEmpty(info1.Product, func(p *ProductInfo) []string { return p.HelmCharts }),
			getSliceOrEmpty(info2.Product, func(p *ProductInfo) []string { return p.HelmCharts }))

		// Helm Repositories
		printFieldComparison(w, "Helm Repositories",
			getSliceOrEmpty(info1.Product, func(p *ProductInfo) []string { return p.HelmRepos }),
			getSliceOrEmpty(info2.Product, func(p *ProductInfo) []string { return p.HelmRepos }))

		fmt.Fprintln(w)
	}

	w.Flush()
}

func printFieldComparison(w io.Writer, fieldName string, items1, items2 []string) {
	maxLen := len(items1)
	if len(items2) > maxLen {
		maxLen = len(items2)
	}

	// If both lists are empty, print a single row with empty values
	if maxLen == 0 {
		fmt.Fprintf(w, "%s\t\t\n", fieldName)
		return
	}

	// Print first row with field name
	val1 := ""
	if len(items1) > 0 {
		val1 = items1[0]
	}
	val2 := ""
	if len(items2) > 0 {
		val2 = items2[0]
	}
	fmt.Fprintf(w, "%s\t%s\t%s\n", fieldName, val1, val2)

	// Print remaining rows with empty field name
	for i := 1; i < maxLen; i++ {
		val1 = ""
		if i < len(items1) {
			val1 = items1[i]
		}
		val2 = ""
		if i < len(items2) {
			val2 = items2[i]
		}
		fmt.Fprintf(w, "\t%s\t%s\n", val1, val2)
	}
}

func getSliceOrEmpty[T any](ptr *T, getter func(*T) []string) []string {
	if ptr == nil {
		return []string{}
	}
	return getter(ptr)
}

func getStringOrEmpty[T any](ptr *T, getter func(*T) string) string {
	if ptr == nil {
		return ""
	}
	return getter(ptr)
}
