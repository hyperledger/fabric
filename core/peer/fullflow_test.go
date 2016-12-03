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

package peer

import (
	"math/rand"
	"testing"
	"time"

	"fmt"
	"os"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

func getProposal() (*peer.Proposal, error) {
	cis := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeID: &peer.ChaincodeID{Name: "foo"},
			Type:        peer.ChaincodeSpec_GOLANG}}

	uuid := util.GenerateUUID()

	return utils.CreateProposalFromCIS(uuid, util.GetTestChainID(), cis, signerSerialized)
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

	simRes := []byte("simulation_result")

	// endorse it to get a proposal response
	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, simRes, nil, nil, signer)
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
	_, act, err := ValidateTransaction(tx)
	if err != nil {
		t.Fatalf("ValidateTransaction failed, err %s", err)
		return
	}

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
		t.Fatalf("Simulation results are different")
		return
	}
}

var r *rand.Rand

func corrupt(bytes []byte) {
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().Unix()))
	}

	bytes[r.Int31n(int32(len(bytes)))]--
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
	corrupt(sProp.Signature)

	// validate it - it should fail
	_, _, _, err = ValidateProposalMessage(sProp)
	if err == nil {
		t.Fatalf("ValidateProposalMessage should have failed")
		return
	}

	// sign it again
	sProp, err = utils.GetSignedProposal(prop, signer)
	if err != nil {
		t.Fatalf("GetSignedProposal failed, err %s", err)
		return
	}

	// mess with the message
	corrupt(sProp.ProposalBytes)

	// validate it - it should fail
	_, _, _, err = ValidateProposalMessage(sProp)
	if err == nil {
		t.Fatalf("ValidateProposalMessage should have failed")
		return
	}

	// get a bad signing identity
	badSigner, err := msp.NewNoopMsp().GetSigningIdentity(nil)
	if err != nil {
		t.Fatalf("Couldn't get noop signer")
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
		t.Fatalf("ValidateProposalMessage should have failed")
		return
	}
}

func TestBadTx(t *testing.T) {
	// get a toy proposal
	prop, err := getProposal()
	if err != nil {
		t.Fatalf("getProposal failed, err %s", err)
		return
	}

	simRes := []byte("simulation_result")

	// endorse it to get a proposal response
	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, simRes, nil, nil, signer)
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
	corrupt(tx.Payload)

	// validate the transaction it should fail
	_, _, err = ValidateTransaction(tx)
	if err == nil {
		t.Fatalf("ValidateTransaction should have failed")
		return
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
	_, _, err = ValidateTransaction(tx)
	if err == nil {
		t.Fatalf("ValidateTransaction should have failed")
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

	simRes1 := []byte("simulation_result")

	// endorse it to get a proposal response
	presp1, err := utils.CreateProposalResponse(prop.Header, prop.Payload, simRes1, nil, nil, signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	simRes2 := []byte("simulation_result")

	// endorse it to get a proposal response
	presp2, err := utils.CreateProposalResponse(prop.Header, prop.Payload, simRes2, nil, nil, signer)
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
	_, _, err = ValidateTransaction(tx)
	if err != nil {
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

	simRes1 := []byte("simulation_result1")

	// endorse it to get a proposal response
	presp1, err := utils.CreateProposalResponse(prop.Header, prop.Payload, simRes1, nil, nil, signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	simRes2 := []byte("simulation_result2")

	// endorse it to get a proposal response
	presp2, err := utils.CreateProposalResponse(prop.Header, prop.Payload, simRes2, nil, nil, signer)
	if err != nil {
		t.Fatalf("CreateProposalResponse failed, err %s", err)
		return
	}

	// assemble a transaction from that proposal and endorsement
	_, err = utils.CreateSignedTx(prop, signer, presp1, presp2)
	if err == nil {
		t.Fatalf("CreateSignedTx should have failed")
		return
	}
}

var signer msp.SigningIdentity
var signerSerialized []byte

func TestMain(m *testing.M) {
	// setup crypto algorithms
	primitives.SetSecurityLevel("SHA2", 256)
	// setup the MSP manager so that we can sign/verify
	mspMgrConfigFile := "../../msp/peer-config.json"
	err := msp.GetManager().Setup(mspMgrConfigFile)
	if err != nil {
		os.Exit(-1)
		fmt.Printf("Could not initialize msp")
		return
	}
	mspId := "DEFAULT"
	id := "PEER"
	signingIdentity := &msp.IdentityIdentifier{Mspid: msp.ProviderIdentifier{Value: mspId}, Value: id}
	signer, err = msp.GetManager().GetSigningIdentity(signingIdentity)
	if err != nil {
		os.Exit(-1)
		fmt.Printf("Could not get signer")
		return
	}

	signerSerialized, err = signer.Serialize()
	if err != nil {
		os.Exit(-1)
		fmt.Printf("Could not serialize identity")
		return
	}

	os.Exit(m.Run())
}
