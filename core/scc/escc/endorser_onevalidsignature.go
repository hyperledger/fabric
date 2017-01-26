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
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"

	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
)

var logger = logging.MustGetLogger("escc")

// EndorserOneValidSignature implements the default endorsement policy, which is to
// sign the proposal hash and the read-write set
type EndorserOneValidSignature struct {
}

// Init is called once when the chaincode started the first time
func (e *EndorserOneValidSignature) Init(stub shim.ChaincodeStubInterface) pb.Response {
	logger.Infof("Successfully initialized ESCC")

	return shim.Success(nil)
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
func (e *EndorserOneValidSignature) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()
	if len(args) < 5 {
		return shim.Error(fmt.Sprintf("Incorrect number of arguments (expected a minimum of 5, provided %d)", len(args)))
	} else if len(args) > 7 {
		return shim.Error(fmt.Sprintf("Incorrect number of arguments (expected a maximum of 7, provided %d)", len(args)))
	}

	logger.Infof("ESCC starts: %d args", len(args))

	// handle the header
	var hdr []byte
	if args[1] == nil {
		return shim.Error("serialized Header object is null")
	}

	hdr = args[1]

	// handle the proposal payload
	var payl []byte
	if args[2] == nil {
		return shim.Error("serialized ChaincodeProposalPayload object is null")
	}

	payl = args[2]

	// handle executing chaincode result
	// Status code < 500 can be endorsed
	if args[3] == nil {
		return shim.Error("Response of chaincode executing is null")
	}

	response, err := putils.GetResponse(args[3])
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed to get Response of executing chaincode: %s", err.Error()))
	}

	if response.Status >= shim.ERROR {
		return shim.Error(fmt.Sprintf("Status code less than 500 will be endorsed, get status code: %d", response.Status))
	}

	// handle simulation results
	var results []byte
	if args[4] == nil {
		return shim.Error("simulation results are null")
	}

	results = args[4]

	// Handle serialized events if they have been provided
	// they might be nil in case there's no events but there
	// is a visibility field specified as the next arg
	events := []byte("")
	if len(args) > 5 && args[5] != nil {
		events = args[5]
	}

	// Handle payload visibility (it's an optional argument)
	visibility := []byte("") // TODO: when visibility is properly defined, replace with the default
	if len(args) > 6 {
		if args[6] == nil {
			return shim.Error("serialized events are null")
		}
		visibility = args[6]
	}

	// obtain the default signing identity for this peer; it will be used to sign this proposal response
	localMsp := mspmgmt.GetLocalMSP()
	if localMsp == nil {
		return shim.Error("Nil local MSP manager")
	}

	signingEndorser, err := localMsp.GetDefaultSigningIdentity()
	if err != nil {
		return shim.Error(fmt.Sprintf("Could not obtain the default signing identity, err %s", err))
	}

	// obtain a proposal response
	presp, err := utils.CreateProposalResponse(hdr, payl, response, results, events, visibility, signingEndorser)
	if err != nil {
		return shim.Error(err.Error())
	}

	// marshall the proposal response so that we return its bytes
	prBytes, err := utils.GetBytesProposalResponse(presp)
	if err != nil {
		return shim.Error(fmt.Sprintf("Could not marshall ProposalResponse: err %s", err))
	}

	pResp, err := utils.GetProposalResponse(prBytes)
	if err != nil {
		return shim.Error(err.Error())
	}
	if pResp.Response == nil {
		fmt.Println("GetProposalResponse get empty Response")
	}

	logger.Infof("ESCC exits successfully")
	return shim.Success(prBytes)
}
