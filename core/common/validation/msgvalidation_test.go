/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func createTestTransactionEnvelope(channel string, response *peer.Response, simRes []byte) (*common.Envelope, error) {
	prop, err := createTestProposalAndSignedProposal(channel)
	if err != nil {
		return nil, fmt.Errorf("failed to create test proposal and signed proposal, err %s", err)
	}

	// endorse it to get a proposal response
	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response, simRes, nil, getChaincodeID(), signer)
	if err != nil {
		return nil, fmt.Errorf("CreateProposalResponse failed, err %s", err)
	}

	// assemble a transaction from that proposal and endorsement
	tx, err := protoutil.CreateSignedTx(prop, signer, presp)
	if err != nil {
		return nil, fmt.Errorf("CreateSignedTx failed, err %s", err)
	}

	return tx, nil
}

func createTestProposalAndSignedProposal(channel string) (*peer.Proposal, error) {
	// get a toy proposal
	prop, err := getProposal(channel)
	if err != nil {
		return nil, fmt.Errorf("getProposal failed, err %s", err)
	}

	return prop, nil
}

func TestCheckSignatureFromCreator(t *testing.T) {
	response := &peer.Response{Status: 200}
	simRes := []byte("simulation_result")

	env, err := createTestTransactionEnvelope("testchannelid", response, simRes)
	require.Nil(t, err, "failed to create test transaction: %s", err)
	require.NotNil(t, env)

	// get the payload from the envelope
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	require.NoError(t, err, "GetPayload returns err %s", err)

	// validate the header
	chdr, shdr, err := validateCommonHeader(payload.Header)
	require.NoError(t, err, "validateCommonHeader returns err %s", err)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	// validate the signature in the envelope
	err = checkSignatureFromCreator(shdr.Creator, env.Signature, env.Payload, chdr.ChannelId, cryptoProvider)
	require.NoError(t, err, "checkSignatureFromCreator returns err %s", err)

	// corrupt the creator
	err = checkSignatureFromCreator([]byte("junk"), env.Signature, env.Payload, chdr.ChannelId, cryptoProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "MSP error: could not deserialize")

	// check nonexistent channel
	err = checkSignatureFromCreator(shdr.Creator, env.Signature, env.Payload, "junkchannel", cryptoProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "MSP error: channel doesn't exist")
}
