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

	"github.com/hyperledger/fabric/common/config/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func TestConsortiumGroup(t *testing.T) {

	cg := NewConsortiumGroup(msp.NewMSPConfigHandler())
	og, err := cg.NewGroup("testGroup")
	assert.NoError(t, err, "NewGroup should not have returned error")
	assert.Equal(t, "testGroup", og.(*OrganizationGroup).Name(),
		"Unexpected group name returned")

	cc := cg.Allocate()
	_, ok := cc.(*ConsortiumConfig)
	assert.Equal(t, true, ok, "Allocate should have returned a ConsortiumConfig")

	_, _, err = cg.BeginValueProposals(t, []string{"testGroup"})
	assert.NoError(t, err, "BeginValueProposals should not have returned error")

	err = cg.PreCommit(t)
	assert.NoError(t, err, "PreCommit should not have returned error")
	cg.CommitProposals(t)
	cg.RollbackProposals(t)

}

func TestConsortiumConfig(t *testing.T) {
	cg := NewConsortiumGroup(msp.NewMSPConfigHandler())
	cc := NewConsortiumConfig(cg)
	orgs := cc.Organizations()
	assert.Equal(t, 0, len(orgs))

	policy := cc.ChannelCreationPolicy()
	assert.EqualValues(t, cb.Policy_UNKNOWN, policy.Type, "Expected policy type to be UNKNOWN")

	cc.Commit()
	assert.Equal(t, cg.ConsortiumConfig, cc, "Error committing ConsortiumConfig")

	og, _ := cg.NewGroup("testGroup")
	err := cc.Validate(t, map[string]ValueProposer{
		"testGroup": og,
	})
	assert.NoError(t, err, "Validate returned unexpected error")
	csg := NewConsortiumsGroup(nil)
	err = cc.Validate(t, map[string]ValueProposer{
		"testGroup": csg,
	})
	assert.Error(t, err, "Validate should have failed")

}
