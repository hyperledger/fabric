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

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	putils "github.com/hyperledger/fabric/protos/utils"

	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
)

var logger = flogging.MustGetLogger("escc")

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
// args[3] - ChaincodeID of executing chaincode
// args[4] - result of executing chaincode
// args[5] - binary blob of simulation results
// args[6] - serialized events
// args[7] - payloadVisibility
//
// NOTE: this chaincode is meant to sign another chaincode's simulation
// results. It should not manipulate state as any state change will be
// silently discarded: the only state changes that will be persisted if
// this endorsement is successful is what we are about to sign, which by
// definition can't be a state change of our own.
func (e *EndorserOneValidSignature) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()
	if len(args) < 6 {
		return shim.Error(fmt.Sprintf("Incorrect number of arguments (expected a minimum of 5, provided %d)", len(args)))
	} else if len(args) > 8 {
		return shim.Error(fmt.Sprintf("Incorrect number of arguments (expected a maximum of 7, provided %d)", len(args)))
	}

	logger.Debugf("ESCC starts: %d args", len(args))

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

	// handle ChaincodeID
	if args[3] == nil {
		return shim.Error("ChaincodeID is null")
	}

	ccid, err := putils.UnmarshalChaincodeID(args[3])
	if err != nil {
		return shim.Error(err.Error())
	}

	// handle executing chaincode result
	// Status code < shim.ERRORTHRESHOLD can be endorsed
	if args[4] == nil {
		return shim.Error("Response of chaincode executing is null")
	}

	response, err := putils.GetResponse(args[4])
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed to get Response of executing chaincode: %s", err.Error()))
	}

	if response.Status >= shim.ERRORTHRESHOLD {
		return shim.Error(fmt.Sprintf("Status code less than %d will be endorsed, received status code: %d", shim.ERRORTHRESHOLD, response.Status))
	}

	// handle simulation results
	var results []byte
	if args[5] == nil {
		return shim.Error("simulation results are null")
	}

	results = args[5]

	// Handle serialized events if they have been provided
	// they might be nil in case there's no events but there
	// is a visibility field specified as the next arg
	events := []byte("")
	if len(args) > 6 && args[6] != nil {
		events = args[6]
	}

	// Handle payload visibility (it's an optional argument)
	// currently the fabric only supports full visibility: this means that
	// there are no restrictions on which parts of the proposal payload will
	// be visible in the final transaction; this default approach requires
	// no additional instructions in the PayloadVisibility field; however
	// the fabric may be extended to encode more elaborate visibility
	// mechanisms that shall be encoded in this field (and handled
	// appropriately by the peer)
	var visibility []byte
	if len(args) > 7 {
		visibility = args[7]
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
	presp, err := utils.CreateProposalResponse(hdr, payl, response, results, events, ccid, visibility, signingEndorser)
	if err != nil {
		return shim.Error(err.Error())
	}

	// marshall the proposal response so that we return its bytes
	prBytes, err := utils.GetBytesProposalResponse(presp)
	if err != nil {
		return shim.Error(fmt.Sprintf("Could not marshal ProposalResponse: err %s", err))
	}

	pResp, err := utils.GetProposalResponse(prBytes)
	if err != nil {
		return shim.Error(err.Error())
	}
	if pResp.Response == nil {
		fmt.Println("GetProposalResponse get empty Response")
	}

	logger.Debugf("ESCC exits successfully")
	return shim.Success(prBytes)
}
