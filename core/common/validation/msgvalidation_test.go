/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

func createTestTransactionEnvelope(channel string, response *peer.Response, simRes []byte) (*common.Envelope, error) {
	prop, sProp, err := createTestProposalAndSignedProposal(channel)
	if err != nil {
		return nil, fmt.Errorf("failed to create test proposal and signed proposal, err %s", err)
	}

	// validate it
	_, _, _, err = ValidateProposalMessage(sProp)
	if err != nil {
		return nil, fmt.Errorf("ValidateProposalMessage failed, err %s", err)
	}

	// endorse it to get a proposal response
	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response, simRes, nil, getChaincodeID(), nil, signer)
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

func createTestProposalAndSignedProposal(channel string) (*peer.Proposal, *peer.SignedProposal, error) {
	// get a toy proposal
	prop, err := getProposal(channel)
	if err != nil {
		return nil, nil, fmt.Errorf("getProposal failed, err %s", err)
	}

	// sign it
	sProp, err := protoutil.GetSignedProposal(prop, signer)
	if err != nil {
		return nil, nil, fmt.Errorf("GetSignedProposal failed, err %s", err)
	}
	return prop, sProp, nil
}

func protoMarshal(t *testing.T, m proto.Message) []byte {
	bytes, err := proto.Marshal(m)
	assert.NoError(t, err)
	return bytes
}

// createTestHeader creates a header for a given transaction type, channel id, and creator
// Based on useGoodTxid, the returned header will have either a good or bad txid for testing purpose
func createTestHeader(t *testing.T, txType common.HeaderType, channelId string, creator []byte, useGoodTxid bool) (*common.Header, error) {
	nonce := []byte("nonce-abc-12345")

	// useGoodTxid is used to for testing purpose. When it is true, we use a bad value for txid
	txid := "bad"
	if useGoodTxid {
		txid = protoutil.ComputeTxID(nonce, creator)
	}

	chdr := &common.ChannelHeader{
		Type:      int32(txType),
		ChannelId: channelId,
		TxId:      txid,
		Epoch:     uint64(0),
	}

	shdr := &common.SignatureHeader{
		Creator: creator,
		Nonce:   nonce,
	}

	return &common.Header{
		ChannelHeader:   protoMarshal(t, chdr),
		SignatureHeader: protoMarshal(t, shdr),
	}, nil
}

func createTestEnvelope(t *testing.T, data []byte, header *common.Header, signer msp.SigningIdentity) (*common.Envelope, error) {
	payload := &common.Payload{
		Header: header,
		Data:   data,
	}
	payloadBytes := protoMarshal(t, payload)

	signature, err := signer.Sign(payloadBytes)
	assert.NoError(t, err)

	return &common.Envelope{
		Payload:   payloadBytes,
		Signature: signature,
	}, nil
}

func TestCheckSignatureFromCreator(t *testing.T) {
	response := &peer.Response{Status: 200}
	simRes := []byte("simulation_result")

	env, err := createTestTransactionEnvelope(util.GetTestChainID(), response, simRes)
	assert.Nil(t, err, "failed to create test transaction: %s", err)
	assert.NotNil(t, env)

	// get the payload from the envelope
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	assert.NoError(t, err, "GetPayload returns err %s", err)

	// validate the header
	chdr, shdr, err := validateCommonHeader(payload.Header)
	assert.NoError(t, err, "validateCommonHeader returns err %s", err)

	// validate the signature in the envelope
	err = checkSignatureFromCreator(shdr.Creator, env.Signature, env.Payload, chdr.ChannelId)
	assert.NoError(t, err, "checkSignatureFromCreator returns err %s", err)

	// corrupt the creator
	err = checkSignatureFromCreator([]byte("junk"), env.Signature, env.Payload, chdr.ChannelId)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MSP error: could not deserialize")

	// check nonexistent channel
	err = checkSignatureFromCreator(shdr.Creator, env.Signature, env.Payload, "junkchannel")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MSP error: channel doesn't exist")
}

func TestValidateProposalMessage(t *testing.T) {
	// nonexistent channel
	fakeChannel := "fakechannel"
	_, sProp, err := createTestProposalAndSignedProposal(fakeChannel)
	assert.NoError(t, err)
	// validate it - it should fail
	_, _, _, err = ValidateProposalMessage(sProp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("access denied: channel [%s] creator org [%s]", fakeChannel, signerMSPId))

	// invalid signature
	_, sProp, err = createTestProposalAndSignedProposal(util.GetTestChainID())
	assert.NoError(t, err)
	sigCopy := make([]byte, len(sProp.Signature))
	copy(sigCopy, sProp.Signature)
	for i := 0; i < len(sProp.Signature); i++ {
		sigCopy[i] = byte(int(sigCopy[i]+1) % 255)
	}
	// validate it - it should fail
	_, _, _, err = ValidateProposalMessage(&peer.SignedProposal{ProposalBytes: sProp.ProposalBytes, Signature: sigCopy})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("access denied: channel [%s] creator org [%s]", util.GetTestChainID(), signerMSPId))
}

func ToHex(q uint64) string {
	return "0x" + strconv.FormatUint(q, 16)
}
