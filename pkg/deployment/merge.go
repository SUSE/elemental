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

package deployment

import (
	"fmt"
	"reflect"
	"slices"

	"dario.cat/mergo"
)

type transformer struct{}

// Transformer tells mergo which types this transformer is for.
func (t *transformer) Transformer(typ reflect.Type) func(dest, src reflect.Value) error {
	// Check if the type is the one we want to customize
	switch {
	case typ == reflect.TypeOf([]*Disk{}):
		return t.mergeDisks()
	case typ == reflect.TypeOf(Partitions{}):
		return t.mergePartitions()
	}
	return nil
}

// Merge applies non zero values of the source deployment to the destination deployment.
// The list of disks and partitions are merged by order. For instance, first source partition
// is merged with first destination partition. If source disks or partitions list is longer
// than the destination one, any overflow item is appended.
//
// Other slice types in Deployment are simply replaced, not merged.
func Merge(dst, src *Deployment) error {
	t := &transformer{}
	return mergo.Merge(dst, src, mergo.WithOverride, mergo.WithTransformers(t))
}

func (t *transformer) mergeDisks() func(dest, src reflect.Value) error {
	return mergePtrSlice[[]*Disk](t)
}

func (t *transformer) mergePartitions() func(dest, src reflect.Value) error {
	return mergePtrSlice[Partitions](t)
}

func mergePtrSlice[T ~[]*E, E any](t *transformer) func(dest, src reflect.Value) error {
	return func(dest, src reflect.Value) error {
		if !dest.CanSet() {
			return fmt.Errorf("dest cannot be set")
		}

		destSlice, ok := dest.Interface().(T)
		if !ok {
			return fmt.Errorf("transformer expected dest to be a pointers slice, got %T", dest.Interface())
		}

		srcSlice, ok := src.Interface().(T)
		if !ok {
			return fmt.Errorf("transformer expected src to be a pointers slice, got %T", src.Interface())
		}

		finalSlice := T{}
		loop := min(len(srcSlice), len(destSlice))

		for i := range loop {
			if srcSlice[i] == nil {
				// nil in source slices can be used to remove from dest
				continue
			}
			if destSlice[i] == nil {
				finalSlice = append(finalSlice, srcSlice[i])
				continue
			}
			err := mergo.Merge(destSlice[i], srcSlice[i], mergo.WithOverride, mergo.WithTransformers(t))
			if err != nil {
				return err
			}
			finalSlice = append(finalSlice, destSlice[i])
		}

		// append additional items from src
		if len(srcSlice) > len(destSlice) {
			for _, item := range srcSlice[loop:] {
				if item == nil {
					continue
				}
				finalSlice = append(finalSlice, item)
			}
		}

		// keep unmerged already existing items
		if len(srcSlice) < len(destSlice) {
			for _, item := range destSlice[loop:] {
				if item == nil {
					continue
				}
				finalSlice = append(finalSlice, item)
			}
		}
		dest.Set(reflect.ValueOf(slices.Clip(finalSlice)))
		return nil
	}
}
