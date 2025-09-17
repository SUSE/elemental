/*
Copyright © 2025 SUSE LLC
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

package build

import (
	"fmt"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/internal/butane"
	uc "github.com/suse/elemental/v3/internal/butane/unifiedcore/v0m1"
	"github.com/suse/elemental/v3/internal/image"
)

// configureIngition writes the ignition configuration file based on the provided butane configuartion
func (b *Builder) configureIgnition(def *image.Definition, buildDir image.BuildDir) error {
	if def.OperatingSystem.ButaneConfig.IsZero() {
		b.System.Logger().Info("No butane configuration to translate into Ignition syntax")
		return nil
	}

	rootMapping, err := sanitizeButaneConfig(&def.OperatingSystem.ButaneConfig)
	if err != nil {
		return err
	}

	b.System.Logger().Info("Translating butane configuration to Ignition syntax")
	ignitionFile := filepath.Join(buildDir.OverlaysDir(), image.IgnitionFilePath())

	butaneBytes, err := yaml.Marshal(rootMapping)
	if err != nil {
		return fmt.Errorf("failed marshalling butane configuration: %w", err)
	}

	return butane.WriteIngitionFile(b.System, butaneBytes, ignitionFile)
}

// sanitizeButaneConfig checks the butance config does not include any variant neither version
// and returns the ButanceConfig mapping root with the desired variant and version.
func sanitizeButaneConfig(butaneConfig *yaml.Node) (*yaml.Node, error) {
	const variantK = "variant"
	var rootMapping *yaml.Node

	switch butaneConfig.Kind {
	case yaml.DocumentNode:
		rootMapping = butaneConfig.Content[0]
	case yaml.MappingNode:
		rootMapping = butaneConfig
	default:
		return nil, fmt.Errorf("unexpected kind if yaml node: %v", butaneConfig.Kind)
	}

	// Content is a flat slice with: key1, value1, key2, value2, ...
	for i := 0; i < len(rootMapping.Content); i += 2 {
		keyNode := rootMapping.Content[i]
		if keyNode.Value == variantK {
			return nil, fmt.Errorf("butaneConfig does nos support 'variant' key")
		}
	}
	err := addRootItem(rootMapping, variantK, uc.Variant)
	if err != nil {
		return nil, fmt.Errorf("failed setting butane variant: %w", err)
	}

	return rootMapping, nil
}

// addRootItem adds a new key-value pair to the root mapping of a yaml.Node.
func addRootItem(rootMapping *yaml.Node, key, value string) error {
	if rootMapping.Kind != yaml.MappingNode {
		return fmt.Errorf("failure, YAML root is not a mapping, cannot add item")
	}

	// Create the new key node.
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: key,
	}

	// Create the new value node.
	valueNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: value,
	}
	rootMapping.Content = append(rootMapping.Content, keyNode, valueNode)

	return nil
}
