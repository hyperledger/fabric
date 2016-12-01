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

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("vscc")

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
// function name and args[1] is the Envelope
func (vscc *ValidatorOneValidSignature) Invoke(stub shim.ChaincodeStubInterface) ([]byte, error) {
	// args[0] - function name (not used now)
	// args[1] - serialized Envelope
	args := stub.GetArgs()
	if len(args) < 2 {
		return nil, errors.New("Incorrect number of arguments")
	}

	if args[1] == nil {
		return nil, errors.New("No block to validate")
	}

	logger.Infof("VSCC invoked")

	// get the envelope...
	env, err := utils.GetEnvelope(args[1])
	if err != nil {
		logger.Errorf("VSCC error: GetEnvelope failed, err %s", err)
		return nil, err
	}

	// ...and the payload...
	payl, err := utils.GetPayload(env)
	if err != nil {
		logger.Errorf("VSCC error: GetPayload failed, err %s", err)
		return nil, err
	}

	// validate the payload type
	if common.HeaderType(payl.Header.ChainHeader.Type) != common.HeaderType_ENDORSER_TRANSACTION {
		logger.Errorf("Only Endorser Transactions are supported, provided type %d", payl.Header.ChainHeader.Type)
		return nil, fmt.Errorf("Only Endorser Transactions are supported, provided type %d", payl.Header.ChainHeader.Type)
	}

	// ...and the transaction...
	tx, err := utils.GetTransaction(payl.Data)
	if err != nil {
		logger.Errorf("VSCC error: GetTransaction failed, err %s", err)
		return nil, err
	}

	// loop through each of the actions within
	for _, act := range tx.Actions {
		cap, err := utils.GetChaincodeActionPayload(act.Payload)
		if err != nil {
			logger.Errorf("VSCC error: GetChaincodeActionPayload failed, err %s", err)
			return nil, err
		}

		// this is what is being signed
		prespBytes := cap.Action.ProposalResponsePayload

		// loop through each of the endorsements
		for _, endorsement := range cap.Action.Endorsements {
			// extract the identity of the signer
			end, err := msp.GetManager().DeserializeIdentity(endorsement.Endorser)
			if err != nil {
				logger.Errorf("VSCC error: DeserializeIdentity failed, err %s", err)
				return nil, err
			}

			// validate it
			valid, err := end.Validate()
			if err != nil || !valid {
				logger.Errorf("Invalid endorser, err %s, valid %t", err, valid)
				return nil, fmt.Errorf("Invalid endorser, err %s, valid %t", err, valid)
			}

			// verify the signature
			valid, err = end.Verify(append(prespBytes, endorsement.Endorser...), endorsement.Signature)
			if err != nil || !valid {
				logger.Errorf("Invalid signature, err %s, valid %t", err, valid)
				return nil, fmt.Errorf("Invalid signature, err %s, valid %t", err, valid)
			}
		}
	}

	logger.Infof("VSCC exists successfully")

	return nil, nil
}

// Query is here to satisfy the Chaincode interface. We don't need it for this system chaincode
func (vscc *ValidatorOneValidSignature) Query(stub shim.ChaincodeStubInterface) ([]byte, error) {
	return nil, nil
}
