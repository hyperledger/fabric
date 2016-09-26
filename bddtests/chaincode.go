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

package bddtests

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
)

func createChaincodeSpec(ccType string, path string, args [][]byte) *pb.ChaincodeSpec {
	// make chaincode spec for chaincode to be deployed
	ccSpec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value[ccType]),
		ChaincodeID: &pb.ChaincodeID{Path: path},
		CtorMsg:     &pb.ChaincodeInput{Args: args}}
	return ccSpec

}

func createPropsalID() string {
	return util.GenerateUUID()
}

// createChaincodeDeploymentSpec  Returns a deployment proposal of chaincode type
func createProposalForChaincode(ccChaincodeDeploymentSpec *pb.ChaincodeDeploymentSpec) (proposal *pb.Proposal, err error) {
	var ccDeploymentSpecBytes []byte
	if ccDeploymentSpecBytes, err = proto.Marshal(ccChaincodeDeploymentSpec); err != nil {
		return nil, fmt.Errorf("Error creating proposal from ChaincodeDeploymentSpec:  %s", err)
	}
	lcChaincodeSpec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG,
		ChaincodeID: &pb.ChaincodeID{Name: "lccc"},
		CtorMsg:     &pb.ChaincodeInput{Args: [][]byte{[]byte("deploy"), []byte("default"), ccDeploymentSpecBytes}}}
	lcChaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: lcChaincodeSpec}
	var ccLifecycleChaincodeInvocationSpecBytes []byte
	if ccLifecycleChaincodeInvocationSpecBytes, err = proto.Marshal(lcChaincodeInvocationSpec); err != nil {
		return nil, fmt.Errorf("Error creating proposal from ChaincodeDeploymentSpec:  %s", err)
	}
	// make proposal
	proposal = &pb.Proposal{Type: pb.Proposal_CHAINCODE, Id: createPropsalID(), Payload: ccLifecycleChaincodeInvocationSpecBytes}
	return proposal, nil
}
