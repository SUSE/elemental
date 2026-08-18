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

package config

import (
	"fmt"
	"slices"

	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/helm"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"go.yaml.in/yaml/v3"
)

const (
	elementalLifecycleManager = "elemental-lifecycle-manager"
	rancher                   = "rancher"
	systemUpgradeController   = "system-upgrade-controller"
	certManager               = "cert-manager"
)

// lcmWebhookValues is used to marshal data from values file for LCM chart
type lcmWebhookValues struct {
	Webhook struct {
		Cert struct {
			CreateDefault  bool   `yaml:"createDefault"`
			ExistingSecret string `yaml:"existingSecret"`
			CABundle       string `yaml:"caBundle"`
		} `yaml:"cert"`
	} `yaml:"webhook"`
}

// evaluateLCMDeps removes dependencies of LCM if they are satisfied separately, i.e.,
// - if Rancher chart is enabled in release.yaml, it removes dependency on system-upgrade-controller
// - if the values files contains custom certificate configuration, it removes dependency on cert-manager
func evaluateLCMDeps(enabled []release.HelmChart, corePlatform *core.ReleaseManifest, valueFiles map[string]string, valuesResolver helmValuesResolver) error {
	var lcmChart *api.HelmChart

	var (
		lcmEnabled     = false
		rancherEnabled = false
	)
	for _, chart := range enabled {
		if chart.Name == rancher {
			rancherEnabled = true
			continue
		}
		if chart.Name == elementalLifecycleManager {
			lcmEnabled = true
		}
	}
	if !lcmEnabled {
		// nothing to do!
		return nil
	}

	coreCharts := corePlatform.Components.Helm
	for _, chart := range coreCharts.Charts {
		if chart.GetName() == elementalLifecycleManager {
			lcmChart = chart
			break
		}
	}

	if lcmChart == nil {
		// this could be the case if using core manifest that doesn't contain LCM charts which is the case currently
		// TODO (dharmit): remove this check once the core manifest includes LCM chart by default
		return nil // nothing to do!
	}

	if rancherEnabled {
		// remove system-upgrade-controller from list of dependencies
		for i, dep := range lcmChart.DependsOn {
			if dep.Name == systemUpgradeController {
				lcmChart.DependsOn = slices.Delete(lcmChart.DependsOn, i, i+1)
				break
			}
		}

	}

	_, ok := valueFiles[elementalLifecycleManager]
	if !ok {
		// values.yaml equivalent for LCM isn't provided; cert-manager dependency to be kept as-is
		return nil
	}

	source := &helm.ValueSource{Inline: lcmChart.GetInlineValues(), File: valueFiles[lcmChart.GetName()]}
	values, err := valuesResolver.Resolve(source)
	if err != nil {
		return fmt.Errorf("resolving values for chart %s: %w", lcmChart.GetName(), err)
	}

	var lcmValues lcmWebhookValues
	err = yaml.Unmarshal(values, &lcmValues)
	if err != nil {
		return err
	}

	if !lcmValues.Webhook.Cert.CreateDefault && lcmValues.Webhook.Cert.ExistingSecret != "" {
		// checking only createDefault and existingSecret because caBundle could be an empty string
		for i, dep := range lcmChart.DependsOn {
			if dep.Name == certManager {
				lcmChart.DependsOn = slices.Delete(lcmChart.DependsOn, i, i+1)
				break
			}
		}
	}

	return nil
}
