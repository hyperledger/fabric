/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package util

import (
	"reflect"
	"sort"

	"github.com/hyperledger/fabric/common/util"
)

// GetSortedKeys returns the keys of the map in a sorted order. This function assumes that the keys are string
func GetSortedKeys(m interface{}) []string {
	mapVal := reflect.ValueOf(m)
	keyVals := mapVal.MapKeys()
	keys := []string{}
	for _, keyVal := range keyVals {
		keys = append(keys, keyVal.String())
	}
	sort.Strings(keys)
	return keys
}

// GetValuesBySortedKeys returns the values of the map (mapPtr) in the list (listPtr) in the sorted order of key of the map
// This function assumes that the mapPtr is a pointer to a map and listPtr is a pointer to a list. Further type of keys of the
// map are assumed to be string and the types of the values of the maps and the list are same
func GetValuesBySortedKeys(mapPtr interface{}, listPtr interface{}) {
	mapVal := reflect.ValueOf(mapPtr).Elem()
	keyVals := mapVal.MapKeys()
	if len(keyVals) == 0 {
		return
	}
	keys := make(keys, len(keyVals))
	for i, k := range keyVals {
		keys[i] = newKey(k)
	}
	sort.Sort(keys)
	out := reflect.ValueOf(listPtr).Elem()
	for _, k := range keys {
		val := mapVal.MapIndex(k.Value)
		out.Set(reflect.Append(out, val))
	}
}

type key struct {
	reflect.Value
	str string
}

type keys []*key

func newKey(v reflect.Value) *key {
	return &key{v, v.String()}
}

func (keys keys) Len() int {
	return len(keys)
}

func (keys keys) Swap(i, j int) {
	keys[i], keys[j] = keys[j], keys[i]
}

func (keys keys) Less(i, j int) bool {
	return keys[i].str < keys[j].str
}

// ComputeStringHash computes the hash of the given string
func ComputeStringHash(input string) []byte {
	return ComputeHash([]byte(input))
}

// ComputeHash computes the hash of the given bytes
func ComputeHash(input []byte) []byte {
	return util.ComputeSHA256(input)
}
