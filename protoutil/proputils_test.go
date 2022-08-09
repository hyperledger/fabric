/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func createCIS() *pb.ChaincodeInvocationSpec {
	return &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_GOLANG,
			ChaincodeId: &pb.ChaincodeID{Name: "chaincode_name"},
			Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte("arg1"), []byte("arg2")}},
		},
	}
}

func TestGetChaincodeDeploymentSpec(t *testing.T) {
	_, err := protoutil.UnmarshalChaincodeDeploymentSpec([]byte("bad spec"))
	require.Error(t, err, "Expected error with malformed spec")

	cds, _ := proto.Marshal(&pb.ChaincodeDeploymentSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type: pb.ChaincodeSpec_GOLANG,
		},
	})
	_, err = protoutil.UnmarshalChaincodeDeploymentSpec(cds)
	require.NoError(t, err, "Unexpected error getting deployment spec")
}

func TestCDSProposals(t *testing.T) {
	var prop *pb.Proposal
	var err error
	var txid string
	creator := []byte("creator")
	cds := &pb.ChaincodeDeploymentSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type: pb.ChaincodeSpec_GOLANG,
		},
	}
	policy := []byte("policy")
	escc := []byte("escc")
	vscc := []byte("vscc")
	chainID := "testchannelid"

	// install
	prop, txid, err = protoutil.CreateInstallProposalFromCDS(cds, creator)
	require.NotNil(t, prop, "Install proposal should not be nil")
	require.NoError(t, err, "Unexpected error creating install proposal")
	require.NotEqual(t, "", txid, "txid should not be empty")

	// deploy
	prop, txid, err = protoutil.CreateDeployProposalFromCDS(chainID, cds, creator, policy, escc, vscc, nil)
	require.NotNil(t, prop, "Deploy proposal should not be nil")
	require.NoError(t, err, "Unexpected error creating deploy proposal")
	require.NotEqual(t, "", txid, "txid should not be empty")

	// upgrade
	prop, txid, err = protoutil.CreateUpgradeProposalFromCDS(chainID, cds, creator, policy, escc, vscc, nil)
	require.NotNil(t, prop, "Upgrade proposal should not be nil")
	require.NoError(t, err, "Unexpected error creating upgrade proposal")
	require.NotEqual(t, "", txid, "txid should not be empty")
}

func TestProposal(t *testing.T) {
	// create a proposal from a ChaincodeInvocationSpec
	prop, _, err := protoutil.CreateChaincodeProposalWithTransient(
		common.HeaderType_ENDORSER_TRANSACTION,
		testChannelID, createCIS(),
		[]byte("creator"),
		map[string][]byte{"certx": []byte("transient")})
	if err != nil {
		t.Fatalf("Could not create chaincode proposal, err %s\n", err)
		return
	}

	// serialize the proposal
	pBytes, err := proto.Marshal(prop)
	if err != nil {
		t.Fatalf("Could not serialize the chaincode proposal, err %s\n", err)
		return
	}

	// deserialize it and expect it to be the same
	propBack, err := protoutil.UnmarshalProposal(pBytes)
	if err != nil {
		t.Fatalf("Could not deserialize the chaincode proposal, err %s\n", err)
		return
	}
	if !proto.Equal(prop, propBack) {
		t.Fatalf("Proposal and deserialized proposals don't match\n")
		return
	}

	// get back the header
	hdr, err := protoutil.UnmarshalHeader(prop.Header)
	if err != nil {
		t.Fatalf("Could not extract the header from the proposal, err %s\n", err)
	}

	hdrBytes, err := protoutil.GetBytesHeader(hdr)
	if err != nil {
		t.Fatalf("Could not marshal the header, err %s\n", err)
	}

	hdr, err = protoutil.UnmarshalHeader(hdrBytes)
	if err != nil {
		t.Fatalf("Could not unmarshal the header, err %s\n", err)
	}

	chdr, err := protoutil.UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		t.Fatalf("Could not unmarshal channel header, err %s", err)
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		t.Fatalf("Could not unmarshal signature header, err %s", err)
	}

	_, err = protoutil.GetBytesSignatureHeader(shdr)
	if err != nil {
		t.Fatalf("Could not marshal signature header, err %s", err)
	}

	// sanity check on header
	if chdr.Type != int32(common.HeaderType_ENDORSER_TRANSACTION) ||
		shdr.Nonce == nil ||
		string(shdr.Creator) != "creator" {
		t.Fatalf("Invalid header after unmarshalling\n")
		return
	}

	// get back the header extension
	hdrExt, err := protoutil.UnmarshalChaincodeHeaderExtension(chdr.Extension)
	if err != nil {
		t.Fatalf("Could not extract the header extensions from the proposal, err %s\n", err)
		return
	}

	// sanity check on header extension
	if string(hdrExt.ChaincodeId.Name) != "chaincode_name" {
		t.Fatalf("Invalid header extension after unmarshalling\n")
		return
	}

	cpp, err := protoutil.UnmarshalChaincodeProposalPayload(prop.Payload)
	if err != nil {
		t.Fatalf("could not unmarshal proposal payload")
	}

	cis, err := protoutil.UnmarshalChaincodeInvocationSpec(cpp.Input)
	if err != nil {
		t.Fatalf("could not unmarshal proposal chaincode invocation spec")
	}

	// sanity check on cis
	if cis.ChaincodeSpec.Type != pb.ChaincodeSpec_GOLANG ||
		cis.ChaincodeSpec.ChaincodeId.Name != "chaincode_name" ||
		len(cis.ChaincodeSpec.Input.Args) != 2 ||
		string(cis.ChaincodeSpec.Input.Args[0]) != "arg1" ||
		string(cis.ChaincodeSpec.Input.Args[1]) != "arg2" {
		t.Fatalf("Invalid chaincode invocation spec after unmarshalling\n")
		return
	}

	if string(shdr.Creator) != "creator" {
		t.Fatalf("Failed checking Creator field. Invalid value, expectext 'creator', got [%s]", string(shdr.Creator))
		return
	}
	value, ok := cpp.TransientMap["certx"]
	if !ok || string(value) != "transient" {
		t.Fatalf("Failed checking Transient field. Invalid value, expectext 'transient', got [%s]", string(value))
		return
	}
}

func TestProposalWithTxID(t *testing.T) {
	// create a proposal from a ChaincodeInvocationSpec
	prop, txid, err := protoutil.CreateChaincodeProposalWithTxIDAndTransient(
		common.HeaderType_ENDORSER_TRANSACTION,
		testChannelID,
		createCIS(),
		[]byte("creator"),
		"testtx",
		map[string][]byte{"certx": []byte("transient")},
	)
	require.Nil(t, err)
	require.NotNil(t, prop)
	require.Equal(t, txid, "testtx")

	prop, txid, err = protoutil.CreateChaincodeProposalWithTxIDAndTransient(
		common.HeaderType_ENDORSER_TRANSACTION,
		testChannelID,
		createCIS(),
		[]byte("creator"),
		"",
		map[string][]byte{"certx": []byte("transient")},
	)
	require.Nil(t, err)
	require.NotNil(t, prop)
	require.NotEmpty(t, txid)
}

func TestProposalResponse(t *testing.T) {
	events := &pb.ChaincodeEvent{
		ChaincodeId: "ccid",
		EventName:   "EventName",
		Payload:     []byte("EventPayload"),
		TxId:        "TxID",
	}
	ccid := &pb.ChaincodeID{
		Name:    "ccid",
		Version: "v1",
	}

	pHashBytes := []byte("proposal_hash")
	pResponse := &pb.Response{Status: 200}
	results := []byte("results")
	eventBytes, err := protoutil.GetBytesChaincodeEvent(events)
	if err != nil {
		t.Fatalf("Failure while marshalling the ProposalResponsePayload")
		return
	}

	// get the bytes of the response
	pResponseBytes, err := protoutil.GetBytesResponse(pResponse)
	if err != nil {
		t.Fatalf("Failure while marshalling the Response")
		return
	}

	// get the response from bytes
	_, err = protoutil.UnmarshalResponse(pResponseBytes)
	if err != nil {
		t.Fatalf("Failure while unmarshalling the Response")
		return
	}

	// get the bytes of the ProposalResponsePayload
	prpBytes, err := protoutil.GetBytesProposalResponsePayload(pHashBytes, pResponse, results, eventBytes, ccid)
	if err != nil {
		t.Fatalf("Failure while marshalling the ProposalResponsePayload")
		return
	}

	// get the ProposalResponsePayload message
	prp, err := protoutil.UnmarshalProposalResponsePayload(prpBytes)
	if err != nil {
		t.Fatalf("Failure while unmarshalling the ProposalResponsePayload")
		return
	}

	// get the ChaincodeAction message
	act, err := protoutil.UnmarshalChaincodeAction(prp.Extension)
	if err != nil {
		t.Fatalf("Failure while unmarshalling the ChaincodeAction")
		return
	}

	// sanity check on the action
	if string(act.Results) != "results" {
		t.Fatalf("Invalid actions after unmarshalling")
		return
	}

	event, err := protoutil.UnmarshalChaincodeEvents(act.Events)
	if err != nil {
		t.Fatalf("Failure while unmarshalling the ChainCodeEvents")
		return
	}

	// sanity check on the event
	if string(event.ChaincodeId) != "ccid" {
		t.Fatalf("Invalid actions after unmarshalling")
		return
	}

	pr := &pb.ProposalResponse{
		Payload:     prpBytes,
		Endorsement: &pb.Endorsement{Endorser: []byte("endorser"), Signature: []byte("signature")},
		Version:     1, // TODO: pick right version number
		Response:    &pb.Response{Status: 200, Message: "OK"},
	}

	// create a proposal response
	prBytes, err := protoutil.GetBytesProposalResponse(pr)
	if err != nil {
		t.Fatalf("Failure while marshalling the ProposalResponse")
		return
	}

	// get the proposal response message back
	prBack, err := protoutil.UnmarshalProposalResponse(prBytes)
	if err != nil {
		t.Fatalf("Failure while unmarshalling the ProposalResponse")
		return
	}

	// sanity check on pr
	if prBack.Response.Status != 200 ||
		string(prBack.Endorsement.Signature) != "signature" ||
		string(prBack.Endorsement.Endorser) != "endorser" ||
		!bytes.Equal(prBack.Payload, prpBytes) {
		t.Fatalf("Invalid ProposalResponse after unmarshalling")
		return
	}
}

func TestEnvelope(t *testing.T) {
	// create a proposal from a ChaincodeInvocationSpec
	prop, _, err := protoutil.CreateChaincodeProposal(common.HeaderType_ENDORSER_TRANSACTION, testChannelID, createCIS(), signerSerialized)
	if err != nil {
		t.Fatalf("Could not create chaincode proposal, err %s\n", err)
		return
	}

	response := &pb.Response{Status: 200, Payload: []byte("payload")}
	result := []byte("res")
	ccid := &pb.ChaincodeID{Name: "foo", Version: "v1"}

	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response, result, nil, ccid, signer)
	if err != nil {
		t.Fatalf("Could not create proposal response, err %s\n", err)
		return
	}

	tx, err := protoutil.CreateSignedTx(prop, signer, presp)
	if err != nil {
		t.Fatalf("Could not create signed tx, err %s\n", err)
		return
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("Could not marshal envelope, err %s\n", err)
		return
	}

	tx, err = protoutil.GetEnvelopeFromBlock(envBytes)
	if err != nil {
		t.Fatalf("Could not unmarshal envelope, err %s\n", err)
		return
	}

	act2, err := protoutil.GetActionFromEnvelope(envBytes)
	if err != nil {
		t.Fatalf("Could not extract actions from envelop, err %s\n", err)
		return
	}

	if act2.Response.Status != response.Status {
		t.Fatalf("response staus don't match")
		return
	}
	if !bytes.Equal(act2.Response.Payload, response.Payload) {
		t.Fatalf("response payload don't match")
		return
	}

	if !bytes.Equal(act2.Results, result) {
		t.Fatalf("results don't match")
		return
	}

	txpayl, err := protoutil.UnmarshalPayload(tx.Payload)
	if err != nil {
		t.Fatalf("Could not unmarshal payload, err %s\n", err)
		return
	}

	tx2, err := protoutil.UnmarshalTransaction(txpayl.Data)
	if err != nil {
		t.Fatalf("Could not unmarshal Transaction, err %s\n", err)
		return
	}

	sh, err := protoutil.UnmarshalSignatureHeader(tx2.Actions[0].Header)
	if err != nil {
		t.Fatalf("Could not unmarshal SignatureHeader, err %s\n", err)
		return
	}

	if !bytes.Equal(sh.Creator, signerSerialized) {
		t.Fatalf("creator does not match")
		return
	}

	cap, err := protoutil.UnmarshalChaincodeActionPayload(tx2.Actions[0].Payload)
	if err != nil {
		t.Fatalf("Could not unmarshal ChaincodeActionPayload, err %s\n", err)
		return
	}
	require.NotNil(t, cap)

	prp, err := protoutil.UnmarshalProposalResponsePayload(cap.Action.ProposalResponsePayload)
	if err != nil {
		t.Fatalf("Could not unmarshal ProposalResponsePayload, err %s\n", err)
		return
	}

	ca, err := protoutil.UnmarshalChaincodeAction(prp.Extension)
	if err != nil {
		t.Fatalf("Could not unmarshal ChaincodeAction, err %s\n", err)
		return
	}

	if ca.Response.Status != response.Status {
		t.Fatalf("response staus don't match")
		return
	}
	if !bytes.Equal(ca.Response.Payload, response.Payload) {
		t.Fatalf("response payload don't match")
		return
	}

	if !bytes.Equal(ca.Results, result) {
		t.Fatalf("results don't match")
		return
	}
}

func TestProposalTxID(t *testing.T) {
	nonce := []byte{1}
	creator := []byte{2}

	txid := protoutil.ComputeTxID(nonce, creator)
	require.NotEmpty(t, txid, "TxID cannot be empty.")
	require.Nil(t, protoutil.CheckTxID(txid, nonce, creator))
	require.Error(t, protoutil.CheckTxID("", nonce, creator))

	txid = protoutil.ComputeTxID(nil, nil)
	require.NotEmpty(t, txid, "TxID cannot be empty.")
}

func TestComputeProposalTxID(t *testing.T) {
	txid := protoutil.ComputeTxID([]byte{1}, []byte{1})

	// Compute the function computed by ComputeTxID,
	// namely, base64(sha256(nonce||creator))
	hf := sha256.New()
	hf.Write([]byte{1})
	hf.Write([]byte{1})
	hashOut := hf.Sum(nil)
	txid2 := hex.EncodeToString(hashOut)

	t.Logf("% x\n", hashOut)
	t.Logf("% s\n", txid)
	t.Logf("% s\n", txid2)

	require.Equal(t, txid, txid2)
}

var (
	signer           msp.SigningIdentity
	signerSerialized []byte
)

func TestMain(m *testing.M) {
	// setup the MSP manager so that we can sign/verify
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		fmt.Printf("Could not initialize msp")
		os.Exit(-1)
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		fmt.Printf("Could not initialize cryptoProvider")
		os.Exit(-1)
	}
	signer, err = mspmgmt.GetLocalMSP(cryptoProvider).GetDefaultSigningIdentity()
	if err != nil {
		fmt.Printf("Could not get signer")
		os.Exit(-1)
	}

	signerSerialized, err = signer.Serialize()
	if err != nil {
		fmt.Printf("Could not serialize identity")
		os.Exit(-1)
	}

	os.Exit(m.Run())
}

func TestInvokedChaincodeName(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		name, err := protoutil.InvokedChaincodeName(protoutil.MarshalOrPanic(&pb.Proposal{
			Payload: protoutil.MarshalOrPanic(&pb.ChaincodeProposalPayload{
				Input: protoutil.MarshalOrPanic(&pb.ChaincodeInvocationSpec{
					ChaincodeSpec: &pb.ChaincodeSpec{
						ChaincodeId: &pb.ChaincodeID{
							Name: "cscc",
						},
					},
				}),
			}),
		}))
		require.NoError(t, err)
		require.Equal(t, "cscc", name)
	})

	t.Run("BadProposalBytes", func(t *testing.T) {
		_, err := protoutil.InvokedChaincodeName([]byte("garbage"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not unmarshal proposal")
	})

	t.Run("BadChaincodeProposalBytes", func(t *testing.T) {
		_, err := protoutil.InvokedChaincodeName(protoutil.MarshalOrPanic(&pb.Proposal{
			Payload: []byte("garbage"),
		}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not unmarshal chaincode proposal payload")
	})

	t.Run("BadChaincodeInvocationSpec", func(t *testing.T) {
		_, err := protoutil.InvokedChaincodeName(protoutil.MarshalOrPanic(&pb.Proposal{
			Payload: protoutil.MarshalOrPanic(&pb.ChaincodeProposalPayload{
				Input: []byte("garbage"),
			}),
		}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not unmarshal chaincode invocation spec")
	})

	t.Run("NilChaincodeSpec", func(t *testing.T) {
		_, err := protoutil.InvokedChaincodeName(protoutil.MarshalOrPanic(&pb.Proposal{
			Payload: protoutil.MarshalOrPanic(&pb.ChaincodeProposalPayload{
				Input: protoutil.MarshalOrPanic(&pb.ChaincodeInvocationSpec{}),
			}),
		}))
		require.EqualError(t, err, "chaincode spec is nil")
	})

	t.Run("NilChaincodeID", func(t *testing.T) {
		_, err := protoutil.InvokedChaincodeName(protoutil.MarshalOrPanic(&pb.Proposal{
			Payload: protoutil.MarshalOrPanic(&pb.ChaincodeProposalPayload{
				Input: protoutil.MarshalOrPanic(&pb.ChaincodeInvocationSpec{
					ChaincodeSpec: &pb.ChaincodeSpec{},
				}),
			}),
		}))
		require.EqualError(t, err, "chaincode id is nil")
	})
}
