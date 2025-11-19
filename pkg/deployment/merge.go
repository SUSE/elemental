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

// Merge applies non zero values of src to dst.
//
// Supported merge slice types include: Disks and Partitions.
// Disks are merged by order (e.g. first instance of src is merged with first instance of dst).
// Partitions are merged by label (e.g. label "foo" from src will only be merged with label "foo" from dst).
// If src contains duplicate partitions with duplicate labels, the last partition of the duplicates will be merged.
// Non-merged src values are appended to dst.
//
// Non-supported slice types are replaced, not merged.
func Merge(dst, src *Deployment) error {
	t := &transformer{}
	return mergo.Merge(dst, src, mergo.WithOverride, mergo.WithTransformers(t))
}

func (t *transformer) mergeDisks() func(dest, src reflect.Value) error {
	return mergePtrSlice[[]*Disk](t, nil)
}

func (t *transformer) mergePartitions() func(dest, src reflect.Value) error {
	return mergePtrSlice[Partitions](t, func(p *Partition) string {
		if p == nil {
			return ""
		}

		return p.Label
	})
}

// mergePtrSlice merges src into dest where common elements are matched based on an
// identifier key function. If the key function is nil, merging falls back to index based
// merging where elements from src are merged into dest by order.
func mergePtrSlice[T ~[]*E, E any](
	t *transformer,
	key func(*E) string,
) func(dest, src reflect.Value) error {
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

		var final T
		var err error
		if key == nil {
			final, err = mergePtrSliceIndex(destSlice, srcSlice, t)
			if err != nil {
				return fmt.Errorf("merging pointer slice by index: %w", err)
			}
		} else {
			final, err = mergePtrSliceByKey(destSlice, srcSlice, t, key)
			if err != nil {
				return fmt.Errorf("merging pointer slice by key: %w", err)
			}
		}
		dest.Set(reflect.ValueOf(slices.Clip(final)))
		return nil
	}
}

// mergePtrSliceByKey produces a slice that combines the elements from destSlice and srcSlice.
// Any elements seen in both slices are merged. Merging is done based on a key function that
// returns an identifier of the element. Elements seen with the same identified in both destSlice
// and srcSlice are merged.
//
// Elements that do not contain an identifier are just appended to the final slice.
// If there are duplicate elements in srcSlice with the same identifier, only the last element from the duplicates
// will be added in the final slice.
//
// Ordering of the destSlice is preserved within the produced slice by abiding to the following merge order:
//
//  1. srcSlice elements without a key are added first
//  2. srcSlice elements with a key but without a matching element in destSlice are added next
//  3. Lastly elements that are both present in srcSlice and destSlice are merged, where merging is done in the
//     order that these elements are seen in destSlice.
func mergePtrSliceByKey[T ~[]*E, E any](destSlice, srcSlice T, t *transformer, key func(*E) string) (T, error) {
	final := T{}
	keyedSources := map[string]*E{}
	for _, entry := range srcSlice {
		if entry == nil {
			continue
		}
		entryKey := key(entry)
		if entryKey != "" {
			keyedSources[entryKey] = entry
			continue
		}

		final = append(final, entry)
	}

	processedDestEntries := []*E{}
	for _, entry := range destSlice {
		if entry == nil {
			continue
		}

		entryKey := key(entry)
		if entryKey == "" {
			processedDestEntries = append(processedDestEntries, entry)
			continue
		}

		if srcEntry, ok := keyedSources[entryKey]; ok {
			if err := mergo.Merge(entry, srcEntry, mergo.WithOverride, mergo.WithTransformers(t)); err != nil {
				return nil, err
			}
			delete(keyedSources, entryKey)
		}

		processedDestEntries = append(processedDestEntries, entry)
	}

	for _, v := range keyedSources {
		final = append(final, v)
	}

	return append(final, processedDestEntries...), nil
}

func mergePtrSliceIndex[T ~[]*E, E any](destSlice, srcSlice T, t *transformer) (T, error) {
	finalSlice := T{}
	if len(srcSlice) == 0 {
		finalSlice = append(finalSlice, destSlice...)
	} else {
		destSize := len(destSlice)
		for i := range destSize {
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
				return nil, err
			}
			finalSlice = append(finalSlice, destSlice[i])
		}

		// append additional items from src
		if len(srcSlice) > len(destSlice) {
			for _, item := range srcSlice[destSize:] {
				if item == nil {
					continue
				}
				finalSlice = append(finalSlice, item)
			}
		}
	}
	return finalSlice, nil
}
