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

package vscc

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	//"github.com/hyperledger/fabric/core/crypto"
	pb "github.com/hyperledger/fabric/protos"
)

// ValidatorOneValidSignature implements the default transaction validation policy,
// which is to check the correctness of the read-write set and the endorsement
// signatures
type ValidatorOneValidSignature struct {
}

// Init is called once when the chaincode started the first time
func (vscc *ValidatorOneValidSignature) Init(stub shim.ChaincodeStubInterface) ([]byte, error) {
	// best practice to do nothing (or very little) in Init
	return nil, nil
}

// Invoke is called to validate the specified block of transactions
// This validation system chaincode will check the read-write set validity and at least 1
// correct endorsement. Later we can create more validation system
// chaincodes to provide more sophisticated policy processing such as enabling
// policy specification to be coded as a transaction of the chaincode and the client
// selecting which policy to use for validation using parameter function
// @return serialized Block of valid and invalid transactions indentified
// Note that Peer calls this function with 2 arguments, where args[0] is the
// function name and args[1] is the block
func (vscc *ValidatorOneValidSignature) Invoke(stub shim.ChaincodeStubInterface) ([]byte, error) {
	// args[0] - function name (not used now)
	// args[1] - serialized Block object, which contains orderred transactions
	args := stub.GetArgs()
	if len(args) < 2 {
		return nil, errors.New("Incorrect number of arguments")
	}

	if args[1] == nil {
		return nil, errors.New("No block to validate")
	}

	block := &pb.Block2{}
	if err := proto.Unmarshal(args[1], block); err != nil {
		return nil, fmt.Errorf("Could not unmarshal block: %s", err)
	}

	// block.messages is an array, so we can deterministically iterate and
	// validate each transaction in order
	for _, v := range block.Transactions {
		tx := &pb.Transaction{}

		// Note: for v1, we do not have encrypted blocks

		if err := proto.Unmarshal(v, tx); err != nil {
			vscc.invalidate(tx)
		} else {
			vscc.validate(tx)
		}
	}

	// TODO: fill in after we get the end-to-end v1 skeleton working. Mocked returned value for now
	return args[1], nil
}

// Query is here to satisfy the Chaincode interface. We don't need it for this system chaincode
func (vscc *ValidatorOneValidSignature) Query(stub shim.ChaincodeStubInterface) ([]byte, error) {
	return nil, nil
}

func (vscc *ValidatorOneValidSignature) validate(tx *pb.Transaction) {
	// TODO: fill in after we get the end-to-end v1 skeleton working
}

func (vscc *ValidatorOneValidSignature) invalidate(tx *pb.Transaction) {
	// TODO: fill in after we get the end-to-end v1 skeleton working
}
