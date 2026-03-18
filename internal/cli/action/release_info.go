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
	"net/url"
	"os"
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
	OperatingSystem   string   `json:"operatingSystem" yaml:"operatingSystem"`
	HelmCharts        []string `json:"helmCharts,omitempty" yaml:"helmCharts,omitempty"`
	HelmRepos         []string `json:"helmRepos,omitempty" yaml:"helmRepos,omitempty"`
	SystemdExtensions []string `json:"systemdExtensions,omitempty" yaml:"systemdExtensions,omitempty"`
}

// ProductInfo represents product manifest details
type ProductInfo struct {
	SystemdExtensions []string `json:"systemdExtensions,omitempty" yaml:"systemdExtensions,omitempty"`
	HelmCharts        []string `json:"helmCharts,omitempty" yaml:"helmCharts,omitempty"`
	HelmRepos         []string `json:"helmRepos,omitempty" yaml:"helmRepos,omitempty"`
}

func ReleaseInfo(ctx context.Context, cmd *cli.Command) error {
	if cmd.Root().Metadata == nil || cmd.Root().Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	system := cmd.Root().Metadata["system"].(*sys.System)
	args := &cmdpkg.ReleaseInfoArgs

	system.Logger().Debug("release-info called with args: %+v", args)

	arg := cmd.Args().Get(0)
	resolved, err := resolveManifest(ctx, system, arg, args.Local)
	if err != nil {
		return err
	}

	info := buildManifestInfo(resolved, args.Core, args.Product)

	return printManifestInfo(info, args.Output)
}

func resolveManifest(ctx context.Context, system *sys.System, arg string, local bool) (*resolver.ResolvedManifest, error) {
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
		if _, err := name.ParseReference(uri); err != nil {
			return nil, fmt.Errorf("invalid OCI image reference: %w", err)
		}
	}

	output, err := config.NewOutput(system.FS(), "", "")
	if err != nil {
		return nil, err
	}
	defer output.Cleanup(system.FS())

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

func printManifestInfo(info *ManifestInfo, format string) error {
	switch strings.ToLower(format) {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(info)
	case "yaml":
		encoder := yaml.NewEncoder(os.Stdout)
		return encoder.Encode(info)
	case "":
		printTable(info)
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
}

func printTable(info *ManifestInfo) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if info.Product != nil {
		fmt.Fprintf(w, "Product Manifest\n%s\n", strings.Repeat("-", 16))
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
		OperatingSystem:   osName,
		HelmCharts:        mapHelmCharts(cm.Components.Helm.Charts),
		HelmRepos:         mapHelmRepos(cm.Components.Helm.Repositories),
		SystemdExtensions: mapSystemdExtensions(cm.Components.Systemd.Extensions),
	}
}

func mapProductInfo(pm *product.ReleaseManifest) *ProductInfo {
	return &ProductInfo{
		SystemdExtensions: mapSystemdExtensions(pm.Components.Systemd.Extensions),
		HelmCharts:        mapHelmCharts(pm.Components.Helm.Charts),
		HelmRepos:         mapHelmRepos(pm.Components.Helm.Repositories),
	}
}

func mapHelmCharts(charts []*api.HelmChart) []string {
	var result []string
	for _, h := range charts {
		result = append(result, fmt.Sprintf("%s (%s)", h.GetName(), h.GetRepositoryName()))
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
