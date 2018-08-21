/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package keylevelep

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/chaincode/shim/ext/statebased"
	pb "github.com/hyperledger/fabric/protos/peer"
)

/*
EndorsementCC is an example chaincode that uses state-based endorsement.
In the init function, it creates a single KVS state "endorsed_state" that
can then be modified through chaincode functions that use the state-based
endorsement chaincode convenience layer. The following chaincode functions
are provided:
-) "addorgs": supply a list of MSP IDs that will be added to the
   state's endorsement policy
-) "delorgs": supply a list of MSP IDs that will be removed from
   the state's endorsement policy
-) "delep": delete the key-level endorsement policy for the state altogether
-) "listorgs": list the orgs included in the state's endorsement policy
*/
type EndorsementCC struct {
}

// Init callback
func (cc *EndorsementCC) Init(stub shim.ChaincodeStubInterface) pb.Response {
	err := stub.PutState("endorsed_state", []byte("foo"))
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success(nil)
}

// Invoke dispatcher
func (cc *EndorsementCC) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	funcName, _ := stub.GetFunctionAndParameters()
	if function, ok := functions[funcName]; ok {
		return function(stub)
	}
	return shim.Error(fmt.Sprintf("Unknown function %s", funcName))
}

// function dispatch map used by Invoke()
var functions = map[string]func(stub shim.ChaincodeStubInterface) pb.Response{
	"addorgs":  addOrgs,
	"delorgs":  delOrgs,
	"listorgs": listOrgs,
	"delep":    delEP,
	"setval":   setVal,
	"getval":   getVal,
	"cc2cc":    invokeCC,
}

// addOrgs adds the list of MSP IDs from the invocation parameters
// to the state's endorsement policy
func addOrgs(stub shim.ChaincodeStubInterface) pb.Response {
	_, parameters := stub.GetFunctionAndParameters()
	if len(parameters) == 0 {
		return shim.Error("No orgs to add specified")
	}

	// get the endorsement policy for the key
	epBytes, err := stub.GetStateValidationParameter("endorsed_state")
	if err != nil {
		return shim.Error(err.Error())
	}
	ep, err := statebased.NewStateEP(epBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	// add organizations to endorsement policy
	err = ep.AddOrgs(statebased.RoleTypePeer, parameters...)
	if err != nil {
		return shim.Error(err.Error())
	}
	epBytes, err = ep.Policy()
	if err != nil {
		return shim.Error(err.Error())
	}

	// set the modified endorsement policy for the key
	err = stub.SetStateValidationParameter("endorsed_state", epBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success([]byte{})
}

// delOrgs removes the list of MSP IDs from the invocation parameters
// from the state's endorsement policy
func delOrgs(stub shim.ChaincodeStubInterface) pb.Response {
	_, parameters := stub.GetFunctionAndParameters()
	if len(parameters) == 0 {
		return shim.Error("No orgs to delete specified")
	}

	// get the endorsement policy for the key
	epBytes, err := stub.GetStateValidationParameter("endorsed_state")
	if err != nil {
		return shim.Error(err.Error())
	}
	ep, err := statebased.NewStateEP(epBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	// delete organizations from the endorsement policy of that key
	ep.DelOrgs(parameters...)
	epBytes, err = ep.Policy()
	if err != nil {
		return shim.Error(err.Error())
	}

	// set the modified endorsement policy for the key
	err = stub.SetStateValidationParameter("endorsed_state", epBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success([]byte{})
}

// listOrgs returns the list of organizations currently part of
// the state's endorsement policy
func listOrgs(stub shim.ChaincodeStubInterface) pb.Response {
	// get the endorsement policy for the key
	epBytes, err := stub.GetStateValidationParameter("endorsed_state")
	if err != nil {
		return shim.Error(err.Error())
	}
	ep, err := statebased.NewStateEP(epBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	// get the list of organizations in the endorsement policy
	orgs := ep.ListOrgs()
	orgsList, err := json.Marshal(orgs)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(orgsList)
}

// delEP deletes the state-based endorsement policy for the key altogether
func delEP(stub shim.ChaincodeStubInterface) pb.Response {
	// set the modified endorsement policy for the key to nil
	err := stub.SetStateValidationParameter("endorsed_state", nil)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success([]byte{})
}

// setVal sets the value of the KVS key
func setVal(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()
	if len(args) != 2 {
		return shim.Error("setval expects one argument")
	}
	err := stub.PutState("endorsed_state", args[1])
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success([]byte{})
}

// getVal retrieves the value of the KVS key
func getVal(stub shim.ChaincodeStubInterface) pb.Response {
	val, err := stub.GetState("endorsed_state")
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success(val)
}

// invokeCC is used for chaincode to chaincode invocation of a given cc on another channel
func invokeCC(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()
	if len(args) < 3 {
		return shim.Error("cc2cc expects at least two arguments (channel and chaincode)")
	}
	channel := string(args[1])
	cc := string(args[2])
	nargs := args[3:]
	resp := stub.InvokeChaincode(cc, nargs, channel)
	return resp
}
