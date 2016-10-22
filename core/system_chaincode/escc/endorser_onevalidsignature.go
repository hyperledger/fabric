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

package escc

import (
	"errors"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	//"github.com/hyperledger/fabric/core/crypto"
)

// EndorserOneValidSignature implements the default endorsement policy, which is to
// sign the proposal hash and the read-write set
type EndorserOneValidSignature struct {
}

// Init is called once when the chaincode started the first time
func (e *EndorserOneValidSignature) Init(stub shim.ChaincodeStubInterface) ([]byte, error) {
	// best practice to do nothing (or very little) in Init
	return nil, nil
}

// Invoke is called to endorse the specified Proposal
// For now, we sign the input and return the endorsed result. Later we can expand
// the chaincode to provide more sophisticate policy processing such as enabling
// policy specification to be coded as a transaction of the chaincode and Client
// could select which policy to use for endorsement using parameter
// @return signature of Action object or error
// Note that Peer calls this function with 3 arguments, where args[0] is the
// function name and args[1] is the Action object and args[2] is Proposal object
func (e *EndorserOneValidSignature) Invoke(stub shim.ChaincodeStubInterface) ([]byte, error) {
	// args[0] - function name (not used now)
	// args[1] - serialized Action object
	// args[2] - serialized Proposal object (not used)
	args := stub.GetArgs()
	if len(args) < 3 {
		return nil, errors.New("Incorrect number of arguments")
	}

	if args[1] == nil {
		return nil, errors.New("Action object is null")
	}

	// TODO: since we are just trying to get the end-to-end happy path going, we return a fake signed proposal
	// Once the scc interface is updated to have a pointer to the peer SecHelper, we will compute the actual signature
	return []byte("true"), nil
	//return crypto.sign(args[1])
}

// Query is here to satisfy the Chaincode interface. We don't need it for this system chaincode
func (e *EndorserOneValidSignature) Query(stub shim.ChaincodeStubInterface) ([]byte, error) {
	return nil, nil
}
