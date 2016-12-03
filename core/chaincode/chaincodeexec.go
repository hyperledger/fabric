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

package chaincode

import (
	"golang.org/x/net/context"

	"fmt"

	pb "github.com/hyperledger/fabric/protos/peer"
)

//create a chaincode invocation spec
func createCIS(ccname string, args [][]byte) (*pb.ChaincodeInvocationSpec, error) {
	var err error
	spec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: ccname}, CtorMsg: &pb.ChaincodeInput{Args: args}}}
	if nil != err {
		return nil, err
	}
	return spec, nil
}

// GetCDSFromLCCC gets chaincode deployment spec from LCCC
func GetCDSFromLCCC(ctxt context.Context, txid string, prop *pb.Proposal, chainID string, chaincodeID string) ([]byte, error) {
	payload, _, err := ExecuteChaincode(ctxt, chainID, txid, prop, "lccc", [][]byte{[]byte("getdepspec"), []byte(chainID), []byte(chaincodeID)})
	return payload, err
}

// ExecuteChaincode executes a given chaincode given chaincode name and arguments
func ExecuteChaincode(ctxt context.Context, chainID string, txid string, prop *pb.Proposal, ccname string, args [][]byte) ([]byte, *pb.ChaincodeEvent, error) {
	var spec *pb.ChaincodeInvocationSpec
	var err error
	var b []byte
	var ccevent *pb.ChaincodeEvent

	spec, err = createCIS(ccname, args)
	b, ccevent, err = Execute(ctxt, chainID, txid, prop, spec)
	if err != nil {
		return nil, nil, fmt.Errorf("Error deploying chaincode: %s", err)
	}
	return b, ccevent, err
}
