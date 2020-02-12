/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chainmgmt

import (
	"fmt"

	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger/kvledger/benchmark/mocks"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
)

const (
	dummyChainID     = "myChain"
	dummyCCName      = "myChaincode"
	useDummyProposal = true
)

var (
	dummyCCID        = &pb.ChaincodeID{Name: dummyCCName, Version: "v1"}
	dummyProposal    *pb.Proposal
	mspLcl           msp.MSP
	signer           msp.SigningIdentity
	serializedSigner []byte
)

func init() {
	mspLcl = mocks.NewNoopMsp()
	signer, _ = mspLcl.GetDefaultSigningIdentity()
	serializedSigner, _ = signer.Serialize()

	dummyProposal, _, _ = protoutil.CreateChaincodeProposal(
		common.HeaderType_ENDORSER_TRANSACTION, dummyChainID,
		&pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: dummyCCID}},
		serializedSigner)
}

func createTxEnv(simulationResults []byte) (*common.Envelope, error) {
	var prop *pb.Proposal
	var err error
	if useDummyProposal {
		prop = dummyProposal
	} else {
		prop, _, err = protoutil.CreateChaincodeProposal(
			common.HeaderType_ENDORSER_TRANSACTION,
			dummyChainID,
			&pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: dummyCCID}},
			serializedSigner)
		if err != nil {
			return nil, err
		}
	}
	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, nil, simulationResults, nil, dummyCCID, signer)
	if err != nil {
		return nil, err
	}

	env, err := protoutil.CreateSignedTx(prop, signer, presp)
	if err != nil {
		return nil, err
	}
	return env, nil
}

func panicOnError(err error) {
	if err != nil {
		panic(fmt.Errorf("Error:%s", err))
	}
}
