/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil_test

import (
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"

	"github.com/hyperledger/fabric/protoutil"
	"github.com/hyperledger/fabric/protoutil/fakes"
	"github.com/stretchr/testify/require"
)

func TestGetPayloads(t *testing.T) {
	var txAction *pb.TransactionAction
	var err error

	// good
	ccActionBytes, _ := proto.Marshal(&pb.ChaincodeAction{
		Results: []byte("results"),
	})
	proposalResponsePayload := &pb.ProposalResponsePayload{
		Extension: ccActionBytes,
	}
	proposalResponseBytes, err := proto.Marshal(proposalResponsePayload)
	require.NoError(t, err)
	ccActionPayload := &pb.ChaincodeActionPayload{
		Action: &pb.ChaincodeEndorsedAction{
			ProposalResponsePayload: proposalResponseBytes,
		},
	}
	ccActionPayloadBytes, _ := proto.Marshal(ccActionPayload)
	txAction = &pb.TransactionAction{
		Payload: ccActionPayloadBytes,
	}
	_, _, err = protoutil.GetPayloads(txAction)
	require.NoError(t, err, "Unexpected error getting payload bytes")
	t.Logf("error1 [%s]", err)

	// nil proposal response extension
	proposalResponseBytes, err = proto.Marshal(&pb.ProposalResponsePayload{
		Extension: nil,
	})
	require.NoError(t, err)
	ccActionPayloadBytes, _ = proto.Marshal(&pb.ChaincodeActionPayload{
		Action: &pb.ChaincodeEndorsedAction{
			ProposalResponsePayload: proposalResponseBytes,
		},
	})
	txAction = &pb.TransactionAction{
		Payload: ccActionPayloadBytes,
	}
	_, _, err = protoutil.GetPayloads(txAction)
	require.Error(t, err, "Expected error with nil proposal response extension")
	t.Logf("error2 [%s]", err)

	// malformed proposal response payload
	ccActionPayloadBytes, _ = proto.Marshal(&pb.ChaincodeActionPayload{
		Action: &pb.ChaincodeEndorsedAction{
			ProposalResponsePayload: []byte("bad payload"),
		},
	})
	txAction = &pb.TransactionAction{
		Payload: ccActionPayloadBytes,
	}
	_, _, err = protoutil.GetPayloads(txAction)
	require.Error(t, err, "Expected error with malformed proposal response payload")
	t.Logf("error3 [%s]", err)

	// malformed proposal response payload extension
	proposalResponseBytes, _ = proto.Marshal(&pb.ProposalResponsePayload{
		Extension: []byte("bad extension"),
	})
	ccActionPayloadBytes, _ = proto.Marshal(&pb.ChaincodeActionPayload{
		Action: &pb.ChaincodeEndorsedAction{
			ProposalResponsePayload: proposalResponseBytes,
		},
	})
	txAction = &pb.TransactionAction{
		Payload: ccActionPayloadBytes,
	}
	_, _, err = protoutil.GetPayloads(txAction)
	require.Error(t, err, "Expected error with malformed proposal response extension")
	t.Logf("error4 [%s]", err)

	// nil proposal response payload extension
	proposalResponseBytes, _ = proto.Marshal(&pb.ProposalResponsePayload{
		ProposalHash: []byte("hash"),
	})
	ccActionPayloadBytes, _ = proto.Marshal(&pb.ChaincodeActionPayload{
		Action: &pb.ChaincodeEndorsedAction{
			ProposalResponsePayload: proposalResponseBytes,
		},
	})
	txAction = &pb.TransactionAction{
		Payload: ccActionPayloadBytes,
	}
	_, _, err = protoutil.GetPayloads(txAction)
	require.Error(t, err, "Expected error with nil proposal response extension")
	t.Logf("error5 [%s]", err)

	// malformed transaction action payload
	txAction = &pb.TransactionAction{
		Payload: []byte("bad payload"),
	}
	_, _, err = protoutil.GetPayloads(txAction)
	require.Error(t, err, "Expected error with malformed transaction action payload")
	t.Logf("error6 [%s]", err)
}

func TestDeduplicateEndorsements(t *testing.T) {
	signID := &fakes.SignerSerializer{}
	signID.SerializeReturns([]byte("signer"), nil)
	signerBytes, err := signID.Serialize()
	require.NoError(t, err, "Unexpected error serializing signing identity")

	proposal := &pb.Proposal{
		Header: protoutil.MarshalOrPanic(&cb.Header{
			ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
				Extension: protoutil.MarshalOrPanic(&pb.ChaincodeHeaderExtension{}),
			}),
			SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{
				Creator: signerBytes,
			}),
		}),
	}
	responses := []*pb.ProposalResponse{
		{Payload: []byte("payload"), Endorsement: &pb.Endorsement{Endorser: []byte{5, 4, 3}}, Response: &pb.Response{Status: int32(200)}},
		{Payload: []byte("payload"), Endorsement: &pb.Endorsement{Endorser: []byte{5, 4, 3}}, Response: &pb.Response{Status: int32(200)}},
	}

	transaction, err := protoutil.CreateSignedTx(proposal, signID, responses...)
	require.NoError(t, err)
	require.True(t, proto.Equal(transaction, transaction), "got: %#v, want: %#v", transaction, transaction)

	pl := protoutil.UnmarshalPayloadOrPanic(transaction.Payload)
	tx, err := protoutil.UnmarshalTransaction(pl.Data)
	require.NoError(t, err)
	ccap, err := protoutil.UnmarshalChaincodeActionPayload(tx.Actions[0].Payload)
	require.NoError(t, err)
	require.Len(t, ccap.Action.Endorsements, 1)
	require.Equal(t, []byte{5, 4, 3}, ccap.Action.Endorsements[0].Endorser)
}

func TestCreateSignedTx(t *testing.T) {
	var err error
	prop := &pb.Proposal{}

	signID := &fakes.SignerSerializer{}
	signID.SerializeReturns([]byte("signer"), nil)
	signerBytes, err := signID.Serialize()
	require.NoError(t, err, "Unexpected error serializing signing identity")

	ccHeaderExtensionBytes := protoutil.MarshalOrPanic(&pb.ChaincodeHeaderExtension{})
	chdrBytes := protoutil.MarshalOrPanic(&cb.ChannelHeader{
		Extension: ccHeaderExtensionBytes,
	})
	shdrBytes := protoutil.MarshalOrPanic(&cb.SignatureHeader{
		Creator: signerBytes,
	})
	responses := []*pb.ProposalResponse{{}}

	// malformed signature header
	headerBytes := protoutil.MarshalOrPanic(&cb.Header{
		SignatureHeader: []byte("bad signature header"),
	})
	prop.Header = headerBytes
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.Error(t, err, "Expected error with malformed signature header")

	// set up the header bytes for the remaining tests
	headerBytes, _ = proto.Marshal(&cb.Header{
		ChannelHeader:   chdrBytes,
		SignatureHeader: shdrBytes,
	})
	prop.Header = headerBytes

	nonMatchingTests := []struct {
		responses     []*pb.ProposalResponse
		expectedError string
	}{
		// good response followed by bad response
		{
			[]*pb.ProposalResponse{
				{Payload: []byte("payload"), Response: &pb.Response{Status: int32(200)}},
				{Payload: []byte{}, Response: &pb.Response{Status: int32(500), Message: "failed to endorse"}},
			},
			"proposal response was not successful, error code 500, msg failed to endorse",
		},
		// bad response followed by good response
		{
			[]*pb.ProposalResponse{
				{Payload: []byte{}, Response: &pb.Response{Status: int32(500), Message: "failed to endorse"}},
				{Payload: []byte("payload"), Response: &pb.Response{Status: int32(200)}},
			},
			"proposal response was not successful, error code 500, msg failed to endorse",
		},
	}
	for i, nonMatchingTest := range nonMatchingTests {
		_, err = protoutil.CreateSignedTx(prop, signID, nonMatchingTest.responses...)
		require.EqualErrorf(t, err, nonMatchingTest.expectedError, "Expected non-matching response error '%v' for test %d", nonMatchingTest.expectedError, i)
	}

	// good responses, but different payloads
	responses = []*pb.ProposalResponse{
		{Payload: []byte("payload"), Response: &pb.Response{Status: int32(200)}},
		{Payload: []byte("payload2"), Response: &pb.Response{Status: int32(200)}},
	}
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	if err == nil || strings.HasPrefix(err.Error(), "ProposalResponsePayloads do not match (base64):") == false {
		require.FailNow(t, "Error is expected when response payloads do not match")
	}

	// no endorsement
	responses = []*pb.ProposalResponse{{
		Payload: []byte("payload"),
		Response: &pb.Response{
			Status: int32(200),
		},
	}}
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.Error(t, err, "Expected error with no endorsements")

	// success
	responses = []*pb.ProposalResponse{{
		Payload:     []byte("payload"),
		Endorsement: &pb.Endorsement{},
		Response: &pb.Response{
			Status: int32(200),
		},
	}}
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.NoError(t, err, "Unexpected error creating signed transaction")
	t.Logf("error: [%s]", err)

	//
	//
	// additional failure cases
	prop = &pb.Proposal{}
	responses = []*pb.ProposalResponse{}
	// no proposal responses
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.Error(t, err, "Expected error with no proposal responses")

	// missing proposal header
	responses = append(responses, &pb.ProposalResponse{})
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.Error(t, err, "Expected error with no proposal header")

	// bad proposal payload
	prop.Payload = []byte("bad payload")
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.Error(t, err, "Expected error with malformed proposal payload")

	// bad payload header
	prop.Header = []byte("bad header")
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.Error(t, err, "Expected error with malformed proposal header")
}

func TestCreateSignedTxNoSigner(t *testing.T) {
	_, err := protoutil.CreateSignedTx(nil, nil, &pb.ProposalResponse{})
	require.ErrorContains(t, err, "signer is required when creating a signed transaction")
}

func TestCreateSignedTxStatus(t *testing.T) {
	serializedExtension, err := proto.Marshal(&pb.ChaincodeHeaderExtension{})
	require.NoError(t, err)
	serializedChannelHeader, err := proto.Marshal(&cb.ChannelHeader{
		Extension: serializedExtension,
	})
	require.NoError(t, err)

	signingID := &fakes.SignerSerializer{}
	signingID.SerializeReturns([]byte("signer"), nil)
	serializedSigningID, err := signingID.Serialize()
	require.NoError(t, err)
	serializedSignatureHeader, err := proto.Marshal(&cb.SignatureHeader{
		Creator: serializedSigningID,
	})
	require.NoError(t, err)

	header := &cb.Header{
		ChannelHeader:   serializedChannelHeader,
		SignatureHeader: serializedSignatureHeader,
	}

	serializedHeader, err := proto.Marshal(header)
	require.NoError(t, err)

	proposal := &pb.Proposal{
		Header: serializedHeader,
	}

	tests := []struct {
		status      int32
		expectedErr string
	}{
		{status: 0, expectedErr: "proposal response was not successful, error code 0, msg response-message"},
		{status: 199, expectedErr: "proposal response was not successful, error code 199, msg response-message"},
		{status: 200, expectedErr: ""},
		{status: 201, expectedErr: ""},
		{status: 399, expectedErr: ""},
		{status: 400, expectedErr: "proposal response was not successful, error code 400, msg response-message"},
	}
	for _, tc := range tests {
		t.Run(strconv.Itoa(int(tc.status)), func(t *testing.T) {
			response := &pb.ProposalResponse{
				Payload:     []byte("payload"),
				Endorsement: &pb.Endorsement{},
				Response: &pb.Response{
					Status:  tc.status,
					Message: "response-message",
				},
			}

			_, err := protoutil.CreateSignedTx(proposal, signingID, response)
			if tc.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}

func TestCreateSignedEnvelope(t *testing.T) {
	var env *cb.Envelope
	channelID := "mychannelID"
	msg := &cb.ConfigEnvelope{}

	id := &fakes.SignerSerializer{}
	id.SignReturnsOnCall(0, []byte("goodsig"), nil)
	id.SignReturnsOnCall(1, nil, errors.New("bad signature"))
	env, err := protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG, channelID,
		id, msg, int32(1), uint64(1))
	require.NoError(t, err, "Unexpected error creating signed envelope")
	require.NotNil(t, env, "Envelope should not be nil")
	// mock sign returns the bytes to be signed
	require.Equal(t, []byte("goodsig"), env.Signature, "Unexpected signature returned")
	payload := &cb.Payload{}
	err = proto.Unmarshal(env.Payload, payload)
	require.NoError(t, err, "Failed to unmarshal payload")
	data := &cb.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, data)
	require.NoError(t, err, "Expected payload data to be a config envelope")
	require.Equal(t, msg, data, "Payload data does not match expected value")

	_, err = protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG, channelID,
		id, &cb.ConfigEnvelope{}, int32(1), uint64(1))
	require.Error(t, err, "Expected sign error")
}

func TestCreateSignedEnvelopeNilSigner(t *testing.T) {
	var env *cb.Envelope
	channelID := "mychannelID"
	msg := &cb.ConfigEnvelope{}

	env, err := protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG, channelID,
		nil, msg, int32(1), uint64(1))
	require.NoError(t, err, "Unexpected error creating signed envelope")
	require.NotNil(t, env, "Envelope should not be nil")
	require.Empty(t, env.Signature, "Signature should have been empty")
	payload := &cb.Payload{}
	err = proto.Unmarshal(env.Payload, payload)
	require.NoError(t, err, "Failed to unmarshal payload")
	data := &cb.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, data)
	require.NoError(t, err, "Expected payload data to be a config envelope")
	require.Equal(t, msg, data, "Payload data does not match expected value")
}

func TestGetSignedProposal(t *testing.T) {
	var signedProp *pb.SignedProposal
	var err error

	sig := []byte("signature")

	signID := &fakes.SignerSerializer{}
	signID.SignReturns(sig, nil)

	prop := &pb.Proposal{}
	propBytes, _ := proto.Marshal(prop)
	signedProp, err = protoutil.GetSignedProposal(prop, signID)
	require.NoError(t, err, "Unexpected error getting signed proposal")
	require.Equal(t, propBytes, signedProp.ProposalBytes,
		"Proposal bytes did not match expected value")
	require.Equal(t, sig, signedProp.Signature,
		"Signature did not match expected value")

	_, err = protoutil.GetSignedProposal(nil, signID)
	require.Error(t, err, "Expected error with nil proposal")
	_, err = protoutil.GetSignedProposal(prop, nil)
	require.Error(t, err, "Expected error with nil signing identity")
}

func TestMockSignedEndorserProposalOrPanic(t *testing.T) {
	var prop *pb.Proposal
	var signedProp *pb.SignedProposal

	ccProposal := &pb.ChaincodeProposalPayload{}
	cis := &pb.ChaincodeInvocationSpec{}
	chainID := "testchannelid"
	sig := []byte("signature")
	creator := []byte("creator")
	cs := &pb.ChaincodeSpec{
		ChaincodeId: &pb.ChaincodeID{
			Name: "mychaincode",
		},
	}

	signedProp, prop = protoutil.MockSignedEndorserProposalOrPanic(chainID, cs,
		creator, sig)
	require.Equal(t, sig, signedProp.Signature,
		"Signature did not match expected result")
	propBytes, _ := proto.Marshal(prop)
	require.Equal(t, propBytes, signedProp.ProposalBytes,
		"Proposal bytes do not match expected value")
	err := proto.Unmarshal(prop.Payload, ccProposal)
	require.NoError(t, err, "Expected ChaincodeProposalPayload")
	err = proto.Unmarshal(ccProposal.Input, cis)
	require.NoError(t, err, "Expected ChaincodeInvocationSpec")
	require.Equal(t, cs.ChaincodeId.Name, cis.ChaincodeSpec.ChaincodeId.Name,
		"Chaincode name did not match expected value")
}

func TestMockSignedEndorserProposal2OrPanic(t *testing.T) {
	var prop *pb.Proposal
	var signedProp *pb.SignedProposal

	ccProposal := &pb.ChaincodeProposalPayload{}
	cis := &pb.ChaincodeInvocationSpec{}
	chainID := "testchannelid"
	sig := []byte("signature")
	signID := &fakes.SignerSerializer{}
	signID.SignReturns(sig, nil)

	signedProp, prop = protoutil.MockSignedEndorserProposal2OrPanic(chainID,
		&pb.ChaincodeSpec{}, signID)
	require.Equal(t, sig, signedProp.Signature,
		"Signature did not match expected result")
	propBytes, _ := proto.Marshal(prop)
	require.Equal(t, propBytes, signedProp.ProposalBytes,
		"Proposal bytes do not match expected value")
	err := proto.Unmarshal(prop.Payload, ccProposal)
	require.NoError(t, err, "Expected ChaincodeProposalPayload")
	err = proto.Unmarshal(ccProposal.Input, cis)
	require.NoError(t, err, "Expected ChaincodeInvocationSpec")
}

func TestGetBytesProposalPayloadForTx(t *testing.T) {
	input := &pb.ChaincodeProposalPayload{
		Input:        []byte("input"),
		TransientMap: make(map[string][]byte),
	}
	expected, _ := proto.Marshal(&pb.ChaincodeProposalPayload{
		Input: []byte("input"),
	})

	result, err := protoutil.GetBytesProposalPayloadForTx(input)
	require.NoError(t, err, "Unexpected error getting proposal payload")
	require.Equal(t, expected, result, "Payload does not match expected value")

	_, err = protoutil.GetBytesProposalPayloadForTx(nil)
	require.Error(t, err, "Expected error with nil proposal payload")
}

func TestGetProposalHash2(t *testing.T) {
	expectedHashHex := "7b622ef4e1ab9b7093ec3bbfbca17d5d6f14a437914a6839319978a7034f7960"
	expectedHash, _ := hex.DecodeString(expectedHashHex)
	hdr := &cb.Header{
		ChannelHeader:   []byte("chdr"),
		SignatureHeader: []byte("shdr"),
	}
	propHash, err := protoutil.GetProposalHash2(hdr, []byte("ccproppayload"))
	require.NoError(t, err, "Unexpected error getting hash2 for proposal")
	require.Equal(t, expectedHash, propHash, "Proposal hash did not match expected hash")

	_, err = protoutil.GetProposalHash2(&cb.Header{}, []byte("ccproppayload"))
	require.Error(t, err, "Expected error with nil arguments")
}

func TestGetProposalHash1(t *testing.T) {
	expectedHashHex := "d4c1e3cac2105da5fddc2cfe776d6ec28e4598cf1e6fa51122c7f70d8076437b"
	expectedHash, _ := hex.DecodeString(expectedHashHex)
	hdr := &cb.Header{
		ChannelHeader:   []byte("chdr"),
		SignatureHeader: []byte("shdr"),
	}

	ccProposal, _ := proto.Marshal(&pb.ChaincodeProposalPayload{})

	propHash, err := protoutil.GetProposalHash1(hdr, ccProposal)
	require.NoError(t, err, "Unexpected error getting hash for proposal")
	require.Equal(t, expectedHash, propHash, "Proposal hash did not match expected hash")

	_, err = protoutil.GetProposalHash1(hdr, []byte("ccproppayload"))
	require.Error(t, err, "Expected error with malformed chaincode proposal payload")

	_, err = protoutil.GetProposalHash1(&cb.Header{}, []byte("ccproppayload"))
	require.Error(t, err, "Expected error with nil arguments")
}

func TestCreateProposalResponseFailure(t *testing.T) {
	// create a proposal from a ChaincodeInvocationSpec
	prop, _, err := protoutil.CreateChaincodeProposal(cb.HeaderType_ENDORSER_TRANSACTION, testChannelID, createCIS(), signerSerialized)
	if err != nil {
		t.Fatalf("Could not create chaincode proposal, err %s\n", err)
		return
	}

	response := &pb.Response{Status: 502, Payload: []byte("Invalid function name")}
	result := []byte("res")

	prespFailure, err := protoutil.CreateProposalResponseFailure(prop.Header, prop.Payload, response, result, nil, "foo")
	if err != nil {
		t.Fatalf("Could not create proposal response failure, err %s\n", err)
		return
	}

	require.Equal(t, int32(502), prespFailure.Response.Status)
	// drilldown into the response to find the chaincode response
	pRespPayload, err := protoutil.UnmarshalProposalResponsePayload(prespFailure.Payload)
	require.NoError(t, err, "Error while unmarshalling proposal response payload: %s", err)
	ca, err := protoutil.UnmarshalChaincodeAction(pRespPayload.Extension)
	require.NoError(t, err, "Error while unmarshalling chaincode action: %s", err)

	require.Equal(t, int32(502), ca.Response.Status)
	require.Equal(t, "Invalid function name", string(ca.Response.Payload))
}

func TestGetorComputeTxIDFromEnvelope(t *testing.T) {
	t.Run("txID is present in the envelope", func(t *testing.T) {
		txID := "709184f9d24f6ade8fcd4d6521a6eef295fef6c2e67216c58b68ac15e8946492"
		envelopeBytes := createSampleTxEnvelopeBytes(txID)
		actualTxID, err := protoutil.GetOrComputeTxIDFromEnvelope(envelopeBytes)
		require.Nil(t, err)
		require.Equal(t, "709184f9d24f6ade8fcd4d6521a6eef295fef6c2e67216c58b68ac15e8946492", actualTxID)
	})

	t.Run("txID is not present in the envelope", func(t *testing.T) {
		txID := ""
		envelopeBytes := createSampleTxEnvelopeBytes(txID)
		actualTxID, err := protoutil.GetOrComputeTxIDFromEnvelope(envelopeBytes)
		require.Nil(t, err)
		require.Equal(t, "709184f9d24f6ade8fcd4d6521a6eef295fef6c2e67216c58b68ac15e8946492", actualTxID)
	})
}

func createSampleTxEnvelopeBytes(txID string) []byte {
	chdr := &cb.ChannelHeader{
		TxId: "709184f9d24f6ade8fcd4d6521a6eef295fef6c2e67216c58b68ac15e8946492",
	}
	chdrBytes := protoutil.MarshalOrPanic(chdr)

	shdr := &cb.SignatureHeader{
		Nonce:   []byte("nonce"),
		Creator: []byte("creator"),
	}
	shdrBytes := protoutil.MarshalOrPanic(shdr)

	hdr := &cb.Header{
		ChannelHeader:   chdrBytes,
		SignatureHeader: shdrBytes,
	}

	payload := &cb.Payload{
		Header: hdr,
	}
	payloadBytes := protoutil.MarshalOrPanic(payload)

	envelope := &cb.Envelope{
		Payload: payloadBytes,
	}
	return protoutil.MarshalOrPanic(envelope)
}
