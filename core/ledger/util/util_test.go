/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetSortedKeys(t *testing.T) {
	mapKeyValue := make(map[string]int)
	mapKeyValue["blue"] = 10
	mapKeyValue["apple"] = 15
	mapKeyValue["red"] = 12
	mapKeyValue["123"] = 22
	mapKeyValue["a"] = 33
	mapKeyValue[""] = 30
	require.Equal(t, []string{"", "123", "a", "apple", "blue", "red"}, GetSortedKeys(mapKeyValue))
}

func TestGetValuesBySortedKeys(t *testing.T) {
	type name struct {
		fName string
		lName string
	}
	mapKeyValue := make(map[string]*name)
	mapKeyValue["2"] = &name{"Two", "two"}
	mapKeyValue["3"] = &name{"Three", "three"}
	mapKeyValue["5"] = &name{"Five", "five"}
	mapKeyValue[""] = &name{"None", "none"}

	sortedRes := []*name{}
	GetValuesBySortedKeys(&mapKeyValue, &sortedRes)
	require.Equal(
		t,
		[]*name{{"None", "none"}, {"Two", "two"}, {"Three", "three"}, {"Five", "five"}},
		sortedRes,
	)
}
