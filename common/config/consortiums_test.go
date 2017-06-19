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
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConsortiums(t *testing.T) {

	csg := NewConsortiumsGroup(nil)
	cg, err := csg.NewGroup("testCG")
	assert.NoError(t, err, "NewGroup shuld not have returned error")
	_, ok := cg.(*ConsortiumGroup)
	assert.Equal(t, true, ok, "NewGroup should have returned a ConsortiumGroup")

	csc := csg.Allocate()
	_, ok = csc.(*ConsortiumsConfig)
	assert.Equal(t, true, ok, "Allocate should have returned a ConsortiumsConfig")

	csc.Commit()
	assert.Equal(t, csc, csg.ConsortiumsConfig, "Failed to commit ConsortiumsConfig")

	err = csc.Validate(t, map[string]ValueProposer{
		"badGroup": &OrganizationGroup{},
	})
	assert.Error(t, err, "Validate should have returned error for wrong type of ValueProposer")
	err = csc.Validate(t, map[string]ValueProposer{
		"testGroup": cg,
	})
	assert.NoError(t, err, "Validate should not have returned error")
	assert.Equal(t, cg, csc.(*ConsortiumsConfig).Consortiums()["testGroup"],
		"Expected consortium groups to be the same")

}
