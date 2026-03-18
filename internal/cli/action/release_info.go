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

// coreManifest struct here is only for the sake of `elemental3 release-info` command;
// original struct is in pkg/manifest/api/core/manifest.go
type coreManifest struct {
	OperatingSystem   string   `yaml:"operatingSystem" json:"operatingSystem,omitempty"`
	HelmCharts        []string `yaml:"helmCharts,omitempty" json:"helmCharts,omitempty"`
	HelmRepos         []string `yaml:"helmRepos,omitempty" json:"helmRepos,omitempty"`
	SystemdExtensions []string `yaml:"systemdExtensions,omitempty" json:"systemdExtensions,omitempty"`
}

// productManifest struct here is only for the sake of `elemental3 release-info` command;
// original struct is in pkg/manifest/api/product/manifest.go
type productManifest struct {
	SystemdExtensions []string `yaml:"systemdExtensions"`
	HelmCharts        []string `yaml:"helmCharts"`
	HelmRepos         []string `yaml:"helmRepos"`
}

func ReleaseInfo(ctx context.Context, cmd *cli.Command) error {
	if cmd.Root().Metadata == nil || cmd.Root().Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	system := cmd.Root().Metadata["system"].(*sys.System)
	args := &cmdpkg.ReleaseInfoArgs

	system.Logger().Debug("release-info called with args: %+v", args)

	// check if the provided arg is a local file or an OCI image
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
		_, err = name.ParseReference(uri)
		if err != nil {
			return fmt.Errorf("invalid OCI image reference: %w", err)
		}
	}

	output, err := config.NewOutput(system.FS(), "", "")
	if err != nil {
		return err
	}
	defer output.Cleanup(system.FS())

	resolver, err := manifestResolver(system.FS(), output, args.Local)
	if err != nil {
		return err
	}
	resolvedManifest, err := resolver.Resolve(uri)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.Debug)

	coreManifestOut := generateCoreManifest(resolvedManifest.CorePlatform)
	//var pmOut *productManifest
	//if resolvedManifest.ProductExtension != nil {
	//	pmOut = printProductManifest(w, resolvedManifest.ProductExtension)
	//
	//	fmt.Fprintf(w, "Product Manifest\n%s\n", strings.Repeat("-", len("Product Manifest")))
	//	fmt.Fprintf(w, "Helm Charts\t %s\n", strings.Join(pmOut.HelmCharts, ", "))
	//	fmt.Fprintf(w, "Helm Repositories\t %s \n", strings.Join(pmOut.HelmRepos, ", "))
	//	fmt.Fprintln(w)
	//}
	//
	//fmt.Fprintf(w, "Core Manifest\n%s\n", strings.Repeat("-", len("Core Manifest")))
	//fmt.Fprintf(w, "Operating System\t %s\n", coreManifestOut.OperatingSystem)
	//fmt.Fprintf(w, "Helm Charts\t %s\n", strings.Join(coreManifestOut.HelmCharts, ", "))
	//fmt.Fprintf(w, "Helm Repositories\t %s \n", strings.Join(coreManifestOut.HelmRepos, ", "))
	//
	w.Flush()
	//
	//if pmOut != nil {
	//	data, err := yaml.Marshal(&pmOut)
	//	if err != nil {
	//		return err
	//	}
	//	fmt.Println()
	//	fmt.Println(string(data))
	//}
	data, err := json.Marshal(&coreManifestOut)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println(string(data))

	return nil
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

func generateCoreManifest(cm *core.ReleaseManifest) *coreManifest {
	var os string
	os = strings.Split(cm.Components.OperatingSystem.Image.Base, ":")[1]
	os = "SLES " + strings.Split(os, "-")[0]

	var helmCharts, helmRepos, systemdExtensions []string

	for _, h := range cm.Components.Helm.Charts {
		helmCharts = append(helmCharts, fmt.Sprintf("%s (%s)", h.GetName(), h.GetRepositoryName()))
	}
	for _, r := range cm.Components.Helm.Repositories {
		helmRepos = append(helmRepos, r.Name)
	}
	for _, e := range cm.Components.Systemd.Extensions {
		systemdExtensions = append(systemdExtensions, e.Name)
	}

	return &coreManifest{
		OperatingSystem:   os,
		HelmCharts:        helmCharts,
		HelmRepos:         helmRepos,
		SystemdExtensions: systemdExtensions,
	}
}

func printProductManifest(w *tabwriter.Writer, pm *product.ReleaseManifest) *productManifest {
	var systemdExtensions, helmCharts, helmRepos []string

	for _, e := range pm.Components.Systemd.Extensions {
		systemdExtensions = append(systemdExtensions, e.Name)
	}
	for _, h := range pm.Components.Helm.Charts {
		helmCharts = append(helmCharts, fmt.Sprintf("%s (%s)", h.GetName(), h.GetRepositoryName()))
	}
	for _, r := range pm.Components.Helm.Repositories {
		helmRepos = append(helmRepos, r.Name)
	}

	return &productManifest{
		SystemdExtensions: systemdExtensions,
		HelmCharts:        helmCharts,
		HelmRepos:         helmRepos,
	}
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
