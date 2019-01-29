/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testutils

import (
	"fmt"
	"os"

	"github.com/hyperledger/fabric/common/crypto"
	mmsp "github.com/hyperledger/fabric/common/mocks/msp"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
)

var (
	signer msp.SigningIdentity
)

func init() {
	var err error
	// setup the MSP manager so that we can sign/verify
	err = msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		fmt.Printf("Could not load msp config, err %s", err)
		os.Exit(-1)
		return
	}
	signer, err = mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		os.Exit(-1)
		fmt.Printf("Could not initialize msp/signer")
		return
	}
}

// ConstructBytesProposalResponsePayload constructs a ProposalResponsePayload byte for tests with a default signer.
func ConstructBytesProposalResponsePayload(chainID string, ccid *pb.ChaincodeID, pResponse *pb.Response, simulationResults []byte) ([]byte, error) {
	ss, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	prop, _, err := protoutil.CreateChaincodeProposal(common.HeaderType_ENDORSER_TRANSACTION, chainID, &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: ccid}}, ss)
	if err != nil {
		return nil, err
	}

	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, pResponse, simulationResults, nil, ccid, nil, signer)
	if err != nil {
		return nil, err
	}

	return presp.Payload, nil
}

// ConstructSignedTxEnvWithDefaultSigner constructs a transaction envelop for tests with a default signer.
// This method helps other modules to construct a transaction with supplied parameters
func ConstructSignedTxEnvWithDefaultSigner(chainID string, ccid *pb.ChaincodeID, response *pb.Response, simulationResults []byte, txid string, events []byte, visibility []byte) (*common.Envelope, string, error) {
	return ConstructSignedTxEnv(chainID, ccid, response, simulationResults, txid, events, visibility, signer)
}

// ConstructSignedTxEnv constructs a transaction envelop for tests
func ConstructSignedTxEnv(chainID string, ccid *pb.ChaincodeID, pResponse *pb.Response, simulationResults []byte, txid string, events []byte, visibility []byte, signer msp.SigningIdentity) (*common.Envelope, string, error) {
	ss, err := signer.Serialize()
	if err != nil {
		return nil, "", err
	}

	var prop *pb.Proposal
	if txid == "" {
		// if txid is not set, then we need to generate one while creating the proposal message
		prop, txid, err = protoutil.CreateChaincodeProposal(common.HeaderType_ENDORSER_TRANSACTION, chainID, &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: ccid}}, ss)

	} else {
		// if txid is set, we should not generate a txid instead reuse the given txid
		nonce, err := crypto.GetRandomNonce()
		if err != nil {
			return nil, "", err
		}
		prop, txid, err = protoutil.CreateChaincodeProposalWithTxIDNonceAndTransient(txid, common.HeaderType_ENDORSER_TRANSACTION, chainID, &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: ccid}}, nonce, ss, nil)
	}
	if err != nil {
		return nil, "", err
	}

	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, pResponse, simulationResults, nil, ccid, nil, signer)
	if err != nil {
		return nil, "", err
	}

	env, err := protoutil.CreateSignedTx(prop, signer, presp)
	if err != nil {
		return nil, "", err
	}
	return env, txid, nil
}

var mspLcl msp.MSP
var sigId msp.SigningIdentity

// ConstructUnsignedTxEnv creates a Transaction envelope from given inputs
func ConstructUnsignedTxEnv(chainID string, ccid *pb.ChaincodeID, response *pb.Response, simulationResults []byte, txid string, events []byte, visibility []byte) (*common.Envelope, string, error) {
	if mspLcl == nil {
		mspLcl = mmsp.NewNoopMsp()
		sigId, _ = mspLcl.GetDefaultSigningIdentity()
	}

	return ConstructSignedTxEnv(chainID, ccid, response, simulationResults, txid, events, visibility, sigId)
}
