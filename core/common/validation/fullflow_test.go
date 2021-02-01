/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func getProposal(channelID string) (*peer.Proposal, error) {
	cis := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: getChaincodeID(),
			Type:        peer.ChaincodeSpec_GOLANG,
		},
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, channelID, cis, signerSerialized)
	return proposal, err
}

func getChaincodeID() *peer.ChaincodeID {
	return &peer.ChaincodeID{Name: "foo", Version: "v1"}
}

func createSignedTxTwoActions(proposal *peer.Proposal, signer msp.SigningIdentity, resps ...*peer.ProposalResponse) (*common.Envelope, error) {
	if len(resps) == 0 {
		return nil, fmt.Errorf("At least one proposal response is necessary")
	}

	// the original header
	hdr, err := protoutil.UnmarshalHeader(proposal.Header)
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal the proposal header")
	}

	// the original payload
	pPayl, err := protoutil.UnmarshalChaincodeProposalPayload(proposal.Payload)
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal the proposal payload")
	}

	// fill endorsements
	endorsements := make([]*peer.Endorsement, len(resps))
	for n, r := range resps {
		endorsements[n] = r.Endorsement
	}

	// create ChaincodeEndorsedAction
	cea := &peer.ChaincodeEndorsedAction{ProposalResponsePayload: resps[0].Payload, Endorsements: endorsements}

	// obtain the bytes of the proposal payload that will go to the transaction
	propPayloadBytes, err := protoutil.GetBytesProposalPayloadForTx(pPayl)
	if err != nil {
		return nil, err
	}

	// serialize the chaincode action payload
	cap := &peer.ChaincodeActionPayload{ChaincodeProposalPayload: propPayloadBytes, Action: cea}
	capBytes, err := protoutil.GetBytesChaincodeActionPayload(cap)
	if err != nil {
		return nil, err
	}

	// create a transaction
	taa := &peer.TransactionAction{Header: hdr.SignatureHeader, Payload: capBytes}
	taas := make([]*peer.TransactionAction, 2)
	taas[0] = taa
	taas[1] = taa
	tx := &peer.Transaction{Actions: taas}

	// serialize the tx
	txBytes, err := protoutil.GetBytesTransaction(tx)
	if err != nil {
		return nil, err
	}

	// create the payload
	payl := &common.Payload{Header: hdr, Data: txBytes}
	paylBytes, err := protoutil.GetBytesPayload(payl)
	if err != nil {
		return nil, err
	}

	// sign the payload
	sig, err := signer.Sign(paylBytes)
	if err != nil {
		return nil, err
	}

	// here's the envelope
	return &common.Envelope{Payload: paylBytes, Signature: sig}, nil
}

func TestGoodPath(t *testing.T) {
	// get a toy proposal
	prop, err := getProposal("testchannelid")
	if err != nil {
		t.Fatalf("getProposal failed, err %s", err)
		return
	}

	response := &peer.Response{Status: 200}
	simRes := []byte("simulation_result")

	// endorse it to get a proposal response
	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response, simRes, nil, getChaincodeID(), signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	// assemble a transaction from that proposal and endorsement
	tx, err := protoutil.CreateSignedTx(prop, signer, presp)
	if err != nil {
		t.Fatalf("CreateSignedTx failed, err %s", err)
		return
	}

	// validate the transaction
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	payl, txResult := ValidateTransaction(tx, cryptoProvider)
	if txResult != peer.TxValidationCode_VALID {
		t.Fatalf("ValidateTransaction failed, err %s", err)
		return
	}

	txx, err := protoutil.UnmarshalTransaction(payl.Data)
	if err != nil {
		t.Fatalf("GetTransaction failed, err %s", err)
		return
	}

	act := txx.Actions

	// expect one single action
	if len(act) != 1 {
		t.Fatalf("Ivalid number of TransactionAction, expected 1, got %d", len(act))
		return
	}

	// get the payload of the action
	_, simResBack, err := protoutil.GetPayloads(act[0])
	if err != nil {
		t.Fatalf("GetPayloads failed, err %s", err)
		return
	}

	// compare it to the original action and expect it to be equal
	if string(simRes) != string(simResBack.Results) {
		t.Fatal("Simulation results are different")
		return
	}
}

func TestTXWithTwoActionsRejected(t *testing.T) {
	// get a toy proposal
	prop, err := getProposal("testchannelid")
	if err != nil {
		t.Fatalf("getProposal failed, err %s", err)
		return
	}

	response := &peer.Response{Status: 200}
	simRes := []byte("simulation_result")

	// endorse it to get a proposal response
	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response, simRes, nil, &peer.ChaincodeID{Name: "somename", Version: "someversion"}, signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	// assemble a transaction from that proposal and endorsement
	tx, err := createSignedTxTwoActions(prop, signer, presp)
	if err != nil {
		t.Fatalf("CreateSignedTx failed, err %s", err)
		return
	}

	// validate the transaction
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	_, txResult := ValidateTransaction(tx, cryptoProvider)
	if txResult == peer.TxValidationCode_VALID {
		t.Fatalf("ValidateTransaction should have failed")
		return
	}
}

func corrupt(bytes []byte) {
	rand.Seed(time.Now().UnixNano())
	bytes[rand.Intn(len(bytes))]--
}

func TestBadTx(t *testing.T) {
	// get a toy proposal
	prop, err := getProposal("testchannelid")
	if err != nil {
		t.Fatalf("getProposal failed, err %s", err)
		return
	}

	response := &peer.Response{Status: 200}
	simRes := []byte("simulation_result")

	// endorse it to get a proposal response
	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response, simRes, nil, getChaincodeID(), signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	// assemble a transaction from that proposal and endorsement
	tx, err := protoutil.CreateSignedTx(prop, signer, presp)
	if err != nil {
		t.Fatalf("CreateSignedTx failed, err %s", err)
		return
	}

	// mess with the transaction payload
	paylOrig := tx.Payload
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	for i := 0; i < len(paylOrig); i++ {
		paylCopy := make([]byte, len(paylOrig))
		copy(paylCopy, paylOrig)
		paylCopy[i] = byte(int(paylCopy[i]+1) % 255)
		// validate the transaction it should fail
		_, txResult := ValidateTransaction(&common.Envelope{Signature: tx.Signature, Payload: paylCopy}, cryptoProvider)
		if txResult == peer.TxValidationCode_VALID {
			t.Fatal("ValidateTransaction should have failed")
			return
		}
	}

	// assemble a transaction from that proposal and endorsement
	tx, err = protoutil.CreateSignedTx(prop, signer, presp)
	if err != nil {
		t.Fatalf("CreateSignedTx failed, err %s", err)
		return
	}

	// mess with the transaction payload
	corrupt(tx.Signature)

	// validate the transaction it should fail
	_, txResult := ValidateTransaction(tx, cryptoProvider)
	if txResult == peer.TxValidationCode_VALID {
		t.Fatal("ValidateTransaction should have failed")
		return
	}
}

func Test2EndorsersAgree(t *testing.T) {
	// get a toy proposal
	prop, err := getProposal("testchannelid")
	if err != nil {
		t.Fatalf("getProposal failed, err %s", err)
		return
	}

	response1 := &peer.Response{Status: 200}
	simRes1 := []byte("simulation_result")

	// endorse it to get a proposal response
	presp1, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response1, simRes1, nil, getChaincodeID(), signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	response2 := &peer.Response{Status: 200}
	simRes2 := []byte("simulation_result")

	// endorse it to get a proposal response
	presp2, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response2, simRes2, nil, getChaincodeID(), signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	// assemble a transaction from that proposal and endorsement
	tx, err := protoutil.CreateSignedTx(prop, signer, presp1, presp2)
	if err != nil {
		t.Fatalf("CreateSignedTx failed, err %s", err)
		return
	}

	// validate the transaction
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	_, txResult := ValidateTransaction(tx, cryptoProvider)
	if txResult != peer.TxValidationCode_VALID {
		t.Fatalf("ValidateTransaction failed, err %s", err)
		return
	}
}

func Test2EndorsersDisagree(t *testing.T) {
	// get a toy proposal
	prop, err := getProposal("testchannelid")
	if err != nil {
		t.Fatalf("getProposal failed, err %s", err)
		return
	}

	response1 := &peer.Response{Status: 200}
	simRes1 := []byte("simulation_result1")

	// endorse it to get a proposal response
	presp1, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response1, simRes1, nil, getChaincodeID(), signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	response2 := &peer.Response{Status: 200}
	simRes2 := []byte("simulation_result2")

	// endorse it to get a proposal response
	presp2, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response2, simRes2, nil, getChaincodeID(), signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	// assemble a transaction from that proposal and endorsement
	_, err = protoutil.CreateSignedTx(prop, signer, presp1, presp2)
	if err == nil {
		t.Fatal("CreateSignedTx should have failed")
		return
	}
}

func TestInvocationsBadArgs(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	_, code := ValidateTransaction(nil, cryptoProvider)
	require.Equal(t, code, peer.TxValidationCode_NIL_ENVELOPE)
	err = validateEndorserTransaction(nil, nil)
	require.Error(t, err)
	err = validateConfigTransaction(nil, nil)
	require.Error(t, err)
	_, _, err = validateCommonHeader(nil)
	require.Error(t, err)
	err = validateChannelHeader(nil)
	require.Error(t, err)
	err = validateChannelHeader(&common.ChannelHeader{})
	require.Error(t, err)
	err = validateSignatureHeader(nil)
	require.Error(t, err)
	err = validateSignatureHeader(&common.SignatureHeader{})
	require.Error(t, err)
	err = validateSignatureHeader(&common.SignatureHeader{Nonce: []byte("a")})
	require.Error(t, err)
	err = checkSignatureFromCreator(nil, nil, nil, "", cryptoProvider)
	require.Error(t, err)
}

var (
	signer           msp.SigningIdentity
	signerSerialized []byte
	signerMSPId      string
)

func TestMain(m *testing.M) {
	// setup crypto algorithms
	// setup the MSP manager so that we can sign/verify
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		fmt.Printf("Could not initialize msp, err %s", err)
		os.Exit(-1)
		return
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		fmt.Printf("Initialize cryptoProvider bccsp failed: %s", err)
		os.Exit(-1)
		return
	}

	signer, err = mspmgmt.GetLocalMSP(cryptoProvider).GetDefaultSigningIdentity()
	if err != nil {
		fmt.Println("Could not get signer")
		os.Exit(-1)
		return
	}
	signerMSPId = signer.GetMSPIdentifier()

	signerSerialized, err = signer.Serialize()
	if err != nil {
		fmt.Println("Could not serialize identity")
		os.Exit(-1)
		return
	}

	os.Exit(m.Run())
}
