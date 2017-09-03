/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	"github.com/hyperledger/fabric/common/configtx"
	mmsp "github.com/hyperledger/fabric/common/mocks/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

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

func TestMakeChainCreationTransactionWithSigner(t *testing.T) {
	channelID := "foo"

	signer, err := mmsp.NewNoopMsp().GetDefaultSigningIdentity()
	assert.NoError(t, err, "Creating noop MSP")

	cct, err := MakeChainCreationTransaction(channelID, "test", signer)
	assert.NoError(t, err, "Making chain creation tx")

	assert.NotEmpty(t, cct.Signature, "Should have signature")

	payload, err := utils.UnmarshalPayload(cct.Payload)
	assert.NoError(t, err, "Unmarshaling payload")

	configUpdateEnv, err := configtx.UnmarshalConfigUpdateEnvelope(payload.Data)
	assert.NoError(t, err, "Unmarshaling ConfigUpdateEnvelope")

	assert.NotEmpty(t, configUpdateEnv.Signatures, "Should have config env sigs")

	sigHeader, err := utils.GetSignatureHeader(payload.Header.SignatureHeader)
	assert.NoError(t, err, "Unmarshaling SignatureHeader")
	assert.NotEmpty(t, sigHeader.Creator, "Creator specified")
}

func TestMakeChainCreationTransactionNoSigner(t *testing.T) {
	channelID := "foo"
	cct, err := MakeChainCreationTransaction(channelID, "test", nil)
	assert.NoError(t, err, "Making chain creation tx")

	assert.Empty(t, cct.Signature, "Should have empty signature")

	payload, err := utils.UnmarshalPayload(cct.Payload)
	assert.NoError(t, err, "Unmarshaling payload")

	configUpdateEnv, err := configtx.UnmarshalConfigUpdateEnvelope(payload.Data)
	assert.NoError(t, err, "Unmarshaling ConfigUpdateEnvelope")

	assert.Empty(t, configUpdateEnv.Signatures, "Should have no config env sigs")

	sigHeader, err := utils.GetSignatureHeader(payload.Header.SignatureHeader)
	assert.NoError(t, err, "Unmarshaling SignatureHeader")
	assert.Empty(t, sigHeader.Creator, "No creator specified")
}
