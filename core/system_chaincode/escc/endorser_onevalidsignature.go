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

	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"

	"github.com/hyperledger/fabric/msp"
)

var logger = logging.MustGetLogger("escc")

// EndorserOneValidSignature implements the default endorsement policy, which is to
// sign the proposal hash and the read-write set
type EndorserOneValidSignature struct {
	signerId *msp.IdentityIdentifier
}

// Init is called once when the chaincode started the first time
// There are 2 mandatory arguments
// args[0] the msp identifier for the ESCC's signer
// args[1] the identifier for the ESCC's signer within the msp
func (e *EndorserOneValidSignature) Init(stub shim.ChaincodeStubInterface) ([]byte, error) {
	// Obtain the identifier of the identity that will be used to sign
	// Note that we cache this identity once and for all. If there is
	// the need to change the signing identity, there are several options:
	// 1) pass the desired signing identity as an optional argument to ESCC
	// 2) expose an ESCC Invoke function that changes the siging identity
	args := stub.GetArgs()
	if len(args) != 2 {
		return nil, fmt.Errorf("Incorrect number of arguments (expected 2, provided %d)", len(args))
	}
	e.signerId = &msp.IdentityIdentifier{Mspid: msp.ProviderIdentifier{Value: string(args[0])}, Value: string(args[1])}

	logger.Infof("Successfully initialized ESCC with identity: %s", e.signerId)

	return nil, nil
}

// Invoke is called to endorse the specified Proposal
// For now, we sign the input and return the endorsed result. Later we can expand
// the chaincode to provide more sophisticate policy processing such as enabling
// policy specification to be coded as a transaction of the chaincode and Client
// could select which policy to use for endorsement using parameter
// @return a marshalled proposal response
// Note that Peer calls this function with 4 mandatory arguments (and 2 optional ones):
// args[0] - function name (not used now)
// args[1] - serialized Header object
// args[2] - serialized ChaincodeProposalPayload object
// args[3] - binary blob of simulation results
// args[4] - serialized events (optional)
// args[5] - payloadVisibility (optional)
//
// NOTE: this chaincode is meant to sign another chaincode's simulation
// results. It should not manipulate state as any state change will be
// silently discarded: the only state changes that will be persisted if
// this endorsement is successful is what we are about to sign, which by
// definition can't be a state change of our own.
func (e *EndorserOneValidSignature) Invoke(stub shim.ChaincodeStubInterface) ([]byte, error) {
	args := stub.GetArgs()
	if len(args) < 4 {
		return nil, fmt.Errorf("Incorrect number of arguments (expected a minimum of 4, provided %d)", len(args))
	} else if len(args) > 6 {
		return nil, fmt.Errorf("Incorrect number of arguments (expected a maximum of 6, provided %d)", len(args))
	}

	logger.Infof("ESCC starts: %d args", len(args))

	// handle the header
	var hdr []byte
	if args[1] == nil {
		return nil, errors.New("serialized Header object is null")
	}

	hdr = args[1]

	// handle the proposal payload
	var payl []byte
	if args[2] == nil {
		return nil, errors.New("serialized ChaincodeProposalPayload object is null")
	}

	payl = args[2]

	// handle simulation results
	var results []byte
	if args[3] == nil {
		return nil, errors.New("simulation results are null")
	}

	results = args[3]

	// Handle serialized events if they have been provided
	// they might be nil in case there's no events but there
	// is a visibility field specified as the next arg
	events := []byte("")
	if len(args) > 4 && args[4] != nil {
		events = args[4]
	}

	// Handle payload visibility (it's an optional argument)
	visibility := []byte("") // TODO: when visibility is properly defined, replace with the default
	if len(args) > 5 {
		if args[5] == nil {
			return nil, errors.New("serialized events are null")
		}
		visibility = args[5]
	}

	// obtain the identity that will sign this proposal response
	// NOTE: we must obtain it every time: while e.signerId remains
	// constant, the corresponding cert might (and will) change
	// and so we cannot cache the result of this call; GetSigningIdentity
	// on the other hand will cache the identity as long as it
	// doesn't change
	signingEndorser, err := msp.GetManager().GetSigningIdentity(e.signerId)
	if err != nil {
		return nil, fmt.Errorf("Could not obtain the signing identity for %s, err %s", e.signerId, err)
	}

	// obtain a proposal response
	presp, err := utils.CreateProposalResponse(hdr, payl, results, events, visibility, signingEndorser)
	if err != nil {
		return nil, err
	}

	// marshall the proposal response so that we return its bytes
	prBytes, err := utils.GetBytesProposalResponse(presp)
	if err != nil {
		return nil, fmt.Errorf("Could not marshall ProposalResponse: err %s", err)
	}

	logger.Infof("ESCC exits successfully")
	return prBytes, nil
}
