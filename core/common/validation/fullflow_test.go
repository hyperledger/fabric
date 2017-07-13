/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package validation

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	mmsp "github.com/hyperledger/fabric/common/mocks/msp"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func getProposal() (*peer.Proposal, error) {
	cis := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: getChaincodeID(),
			Type:        peer.ChaincodeSpec_GOLANG}}

	proposal, _, err := utils.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, util.GetTestChainID(), cis, signerSerialized)
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
	hdr, err := utils.GetHeader(proposal.Header)
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal the proposal header")
	}

	// the original payload
	pPayl, err := utils.GetChaincodeProposalPayload(proposal.Payload)
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
	propPayloadBytes, err := utils.GetBytesProposalPayloadForTx(pPayl, nil)
	if err != nil {
		return nil, err
	}

	// serialize the chaincode action payload
	cap := &peer.ChaincodeActionPayload{ChaincodeProposalPayload: propPayloadBytes, Action: cea}
	capBytes, err := utils.GetBytesChaincodeActionPayload(cap)
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
	txBytes, err := utils.GetBytesTransaction(tx)
	if err != nil {
		return nil, err
	}

	// create the payload
	payl := &common.Payload{Header: hdr, Data: txBytes}
	paylBytes, err := utils.GetBytesPayload(payl)
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
	prop, err := getProposal()
	if err != nil {
		t.Fatalf("getProposal failed, err %s", err)
		return
	}

	// sign it
	sProp, err := utils.GetSignedProposal(prop, signer)
	if err != nil {
		t.Fatalf("GetSignedProposal failed, err %s", err)
		return
	}

	// validate it
	_, _, _, err = ValidateProposalMessage(sProp)
	if err != nil {
		t.Fatalf("ValidateProposalMessage failed, err %s", err)
		return
	}

	response := &peer.Response{Status: 200}
	simRes := []byte("simulation_result")

	// endorse it to get a proposal response
	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, response, simRes, nil, getChaincodeID(), nil, signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	// assemble a transaction from that proposal and endorsement
	tx, err := utils.CreateSignedTx(prop, signer, presp)
	if err != nil {
		t.Fatalf("CreateSignedTx failed, err %s", err)
		return
	}

	// validate the transaction
	payl, txResult := ValidateTransaction(tx)
	if txResult != peer.TxValidationCode_VALID {
		t.Fatalf("ValidateTransaction failed, err %s", err)
		return
	}

	txx, err := utils.GetTransaction(payl.Data)
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
	_, simResBack, err := utils.GetPayloads(act[0])
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
	prop, err := getProposal()
	if err != nil {
		t.Fatalf("getProposal failed, err %s", err)
		return
	}

	response := &peer.Response{Status: 200}
	simRes := []byte("simulation_result")

	// endorse it to get a proposal response
	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, response, simRes, nil, &peer.ChaincodeID{Name: "somename", Version: "someversion"}, nil, signer)
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
	_, txResult := ValidateTransaction(tx)
	if txResult == peer.TxValidationCode_VALID {
		t.Fatalf("ValidateTransaction should have failed")
		return
	}
}

func TestBadProp(t *testing.T) {
	// get a toy proposal
	prop, err := getProposal()
	if err != nil {
		t.Fatalf("getProposal failed, err %s", err)
		return
	}

	// sign it
	sProp, err := utils.GetSignedProposal(prop, signer)
	if err != nil {
		t.Fatalf("GetSignedProposal failed, err %s", err)
		return
	}

	// mess with the signature
	sigOrig := sProp.Signature
	for i := 0; i < len(sigOrig); i++ {
		sigCopy := make([]byte, len(sigOrig))
		copy(sigCopy, sigOrig)
		sigCopy[i] = byte(int(sigCopy[i]+1) % 255)
		// validate it - it should fail
		_, _, _, err = ValidateProposalMessage(&peer.SignedProposal{ProposalBytes: sProp.ProposalBytes, Signature: sigCopy})
		if err == nil {
			t.Fatal("ValidateProposalMessage should have failed")
			return
		}
	}

	// sign it again
	sProp, err = utils.GetSignedProposal(prop, signer)
	if err != nil {
		t.Fatalf("GetSignedProposal failed, err %s", err)
		return
	}

	// mess with the message
	pbytesOrig := sProp.ProposalBytes
	for i := 0; i < len(pbytesOrig); i++ {
		pbytesCopy := make([]byte, len(pbytesOrig))
		copy(pbytesCopy, pbytesOrig)
		pbytesCopy[i] = byte(int(pbytesCopy[i]+1) % 255)
		// validate it - it should fail
		_, _, _, err = ValidateProposalMessage(&peer.SignedProposal{ProposalBytes: pbytesCopy, Signature: sProp.Signature})
		if err == nil {
			t.Fatal("ValidateProposalMessage should have failed")
			return
		}
	}

	// get a bad signing identity
	badSigner, err := mmsp.NewNoopMsp().GetDefaultSigningIdentity()
	if err != nil {
		t.Fatal("Couldn't get noop signer")
		return
	}

	// sign it again with the bad signer
	sProp, err = utils.GetSignedProposal(prop, badSigner)
	if err != nil {
		t.Fatalf("GetSignedProposal failed, err %s", err)
		return
	}

	// validate it - it should fail
	_, _, _, err = ValidateProposalMessage(sProp)
	if err == nil {
		t.Fatal("ValidateProposalMessage should have failed")
		return
	}
}

func corrupt(bytes []byte) {
	rand.Seed(time.Now().UnixNano())
	bytes[rand.Intn(len(bytes))]--
}

func TestBadTx(t *testing.T) {
	// get a toy proposal
	prop, err := getProposal()
	if err != nil {
		t.Fatalf("getProposal failed, err %s", err)
		return
	}

	response := &peer.Response{Status: 200}
	simRes := []byte("simulation_result")

	// endorse it to get a proposal response
	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, response, simRes, nil, getChaincodeID(), nil, signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	// assemble a transaction from that proposal and endorsement
	tx, err := utils.CreateSignedTx(prop, signer, presp)
	if err != nil {
		t.Fatalf("CreateSignedTx failed, err %s", err)
		return
	}

	// mess with the transaction payload
	paylOrig := tx.Payload
	for i := 0; i < len(paylOrig); i++ {
		paylCopy := make([]byte, len(paylOrig))
		copy(paylCopy, paylOrig)
		paylCopy[i] = byte(int(paylCopy[i]+1) % 255)
		// validate the transaction it should fail
		_, txResult := ValidateTransaction(&common.Envelope{Signature: tx.Signature, Payload: paylCopy})
		if txResult == peer.TxValidationCode_VALID {
			t.Fatal("ValidateTransaction should have failed")
			return
		}
	}

	// assemble a transaction from that proposal and endorsement
	tx, err = utils.CreateSignedTx(prop, signer, presp)
	if err != nil {
		t.Fatalf("CreateSignedTx failed, err %s", err)
		return
	}

	// mess with the transaction payload
	corrupt(tx.Signature)

	// validate the transaction it should fail
	_, txResult := ValidateTransaction(tx)
	if txResult == peer.TxValidationCode_VALID {
		t.Fatal("ValidateTransaction should have failed")
		return
	}
}

func Test2EndorsersAgree(t *testing.T) {
	// get a toy proposal
	prop, err := getProposal()
	if err != nil {
		t.Fatalf("getProposal failed, err %s", err)
		return
	}

	response1 := &peer.Response{Status: 200}
	simRes1 := []byte("simulation_result")

	// endorse it to get a proposal response
	presp1, err := utils.CreateProposalResponse(prop.Header, prop.Payload, response1, simRes1, nil, getChaincodeID(), nil, signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	response2 := &peer.Response{Status: 200}
	simRes2 := []byte("simulation_result")

	// endorse it to get a proposal response
	presp2, err := utils.CreateProposalResponse(prop.Header, prop.Payload, response2, simRes2, nil, getChaincodeID(), nil, signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	// assemble a transaction from that proposal and endorsement
	tx, err := utils.CreateSignedTx(prop, signer, presp1, presp2)
	if err != nil {
		t.Fatalf("CreateSignedTx failed, err %s", err)
		return
	}

	// validate the transaction
	_, txResult := ValidateTransaction(tx)
	if txResult != peer.TxValidationCode_VALID {
		t.Fatalf("ValidateTransaction failed, err %s", err)
		return
	}
}

func Test2EndorsersDisagree(t *testing.T) {
	// get a toy proposal
	prop, err := getProposal()
	if err != nil {
		t.Fatalf("getProposal failed, err %s", err)
		return
	}

	response1 := &peer.Response{Status: 200}
	simRes1 := []byte("simulation_result1")

	// endorse it to get a proposal response
	presp1, err := utils.CreateProposalResponse(prop.Header, prop.Payload, response1, simRes1, nil, getChaincodeID(), nil, signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	response2 := &peer.Response{Status: 200}
	simRes2 := []byte("simulation_result2")

	// endorse it to get a proposal response
	presp2, err := utils.CreateProposalResponse(prop.Header, prop.Payload, response2, simRes2, nil, getChaincodeID(), nil, signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	// assemble a transaction from that proposal and endorsement
	_, err = utils.CreateSignedTx(prop, signer, presp1, presp2)
	if err == nil {
		t.Fatal("CreateSignedTx should have failed")
		return
	}
}

func TestInvocationsBadArgs(t *testing.T) {
	_, code := ValidateTransaction(nil)
	assert.Equal(t, code, peer.TxValidationCode_NIL_ENVELOPE)
	err := validateEndorserTransaction(nil, nil)
	assert.Error(t, err)
	err = validateConfigTransaction(nil, nil)
	assert.Error(t, err)
	_, _, err = validateCommonHeader(nil)
	assert.Error(t, err)
	err = validateChannelHeader(nil)
	assert.Error(t, err)
	err = validateChannelHeader(&common.ChannelHeader{})
	assert.Error(t, err)
	err = validateSignatureHeader(nil)
	assert.Error(t, err)
	err = validateSignatureHeader(&common.SignatureHeader{})
	assert.Error(t, err)
	err = validateSignatureHeader(&common.SignatureHeader{Nonce: []byte("a")})
	assert.Error(t, err)
	err = checkSignatureFromCreator(nil, nil, nil, "")
	assert.Error(t, err)
	_, _, _, err = ValidateProposalMessage(nil)
	assert.Error(t, err)
	_, err = validateChaincodeProposalMessage(nil, nil)
	assert.Error(t, err)
	_, err = validateChaincodeProposalMessage(&peer.Proposal{}, &common.Header{[]byte("a"), []byte("a")})
	assert.Error(t, err)
}

var signer msp.SigningIdentity
var signerSerialized []byte

func TestMain(m *testing.M) {
	// setup crypto algorithms
	// setup the MSP manager so that we can sign/verify
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		fmt.Printf("Could not initialize msp, err %s", err)
		os.Exit(-1)
		return
	}

	signer, err = mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		fmt.Println("Could not get signer")
		os.Exit(-1)
		return
	}

	signerSerialized, err = signer.Serialize()
	if err != nil {
		fmt.Println("Could not serialize identity")
		os.Exit(-1)
		return
	}

	os.Exit(m.Run())
}
