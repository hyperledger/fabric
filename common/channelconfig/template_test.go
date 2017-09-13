/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	"github.com/hyperledger/fabric/common/configtx"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

func TestNewChainTemplate(t *testing.T) {
	consortiumName := "Test"
	orgs := []string{"org1", "org2", "org3"}
	nct := NewChainCreationTemplate(consortiumName, orgs)

	newChainID := "foo"
	configEnv, err := nct.Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error creation a chain creation config")
	}

	configUpdate, err := configtx.UnmarshalConfigUpdate(configEnv.ConfigUpdate)
	if err != nil {
		t.Fatalf("Should not have errored: %s", err)
	}

	consortiumProto := &cb.Consortium{}
	err = proto.Unmarshal(configUpdate.WriteSet.Values[ConsortiumKey].Value, consortiumProto)
	assert.NoError(t, err)
	assert.Equal(t, consortiumName, consortiumProto.Name, "Should have set correct consortium name")

	assert.Equal(t, configUpdate.WriteSet.Groups[ApplicationGroupKey].Version, uint64(1))

	assert.Len(t, configUpdate.WriteSet.Groups[ApplicationGroupKey].Groups, len(orgs))

	for _, org := range orgs {
		group, ok := configUpdate.WriteSet.Groups[ApplicationGroupKey].Groups[org]
		assert.True(t, ok, "Expected to find %s but did not", org)
		for _, policy := range group.Policies {
			assert.Equal(t, AdminsPolicyKey, policy.ModPolicy)
		}
	}
}
