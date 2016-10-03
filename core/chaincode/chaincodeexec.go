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

	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
)

//create a Transactions - this has to change to Proposal when we move chaincode to use Proposals
func createTx(typ pb.Transaction_Type, ccname string, args [][]byte) (*pb.Transaction, error) {
	var tx *pb.Transaction
	var err error
	uuid := util.GenerateUUID()
	spec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: ccname}, CtorMsg: &pb.ChaincodeInput{Args: args}}}
	tx, err = pb.NewChaincodeExecute(spec, uuid, typ)
	if nil != err {
		return nil, err
	}
	return tx, nil
}

func getCDSFromLCCC(ctxt context.Context, chainID string, chaincodeID string) ([]byte, error) {
	return ExecuteChaincode(ctxt, pb.Transaction_CHAINCODE_INVOKE, string(DefaultChain), "lccc", [][]byte{[]byte("getdepspec"), []byte(chainID), []byte(chaincodeID)})
}

// ExecuteChaincode executes a given chaincode given chaincode name and arguments
func ExecuteChaincode(ctxt context.Context, typ pb.Transaction_Type, chainname string, ccname string, args [][]byte) ([]byte, error) {
	var tx *pb.Transaction
	var err error
	var b []byte

	tx, err = createTx(typ, ccname, args)
	b, _, err = Execute(ctxt, GetChain(ChainName(chainname)), tx)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s", err)
	}
	return b, err
}
