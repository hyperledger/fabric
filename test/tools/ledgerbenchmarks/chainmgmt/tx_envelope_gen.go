/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package chainmgmt

import (
	mockmsp "github.com/hyperledger/fabric/common/mocks/msp"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
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
	mspLcl = mockmsp.NewNoopMsp()
	signer, _ = mspLcl.GetDefaultSigningIdentity()
	serializedSigner, _ = signer.Serialize()

	dummyProposal, _, _ = putils.CreateChaincodeProposal(
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
		prop, _, err = putils.CreateChaincodeProposal(
			common.HeaderType_ENDORSER_TRANSACTION,
			dummyChainID,
			&pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: dummyCCID}},
			serializedSigner)
		if err != nil {
			return nil, err
		}
	}
	presp, err := putils.CreateProposalResponse(prop.Header, prop.Payload, nil, simulationResults, nil, dummyCCID, nil, signer)
	if err != nil {
		return nil, err
	}

	env, err := putils.CreateSignedTx(prop, signer, presp)
	if err != nil {
		return nil, err
	}
	return env, nil
}
