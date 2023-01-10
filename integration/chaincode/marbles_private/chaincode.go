/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package marbles_private

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-chaincode-go/pkg/statebased"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

// MarblesPrivateChaincode example Chaincode implementation
type MarblesPrivateChaincode struct{}

type marble struct {
	ObjectType string `json:"docType"` // docType is used to distinguish the various types of objects in state database
	Name       string `json:"name"`    // the fieldtags are needed to keep case from bouncing around
	Color      string `json:"color"`
	Size       int    `json:"size"`
	Owner      string `json:"owner"`
}

type marblePrivateDetails struct {
	ObjectType string `json:"docType"` // docType is used to distinguish the various types of objects in state database
	Name       string `json:"name"`    // the fieldtags are needed to keep case from bouncing around
	Price      int    `json:"price"`
}

// Init initializes chaincode
// ===========================
func (t *MarblesPrivateChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Success(nil)
}

// Invoke - Our entry point for Invocations
// ========================================
func (t *MarblesPrivateChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	fmt.Println("invoke is running " + function)

	// Handle different functions
	switch function {
	case "initMarble":
		// create a new marble
		return t.initMarble(stub, args)
	case "readMarble":
		// read a marble
		return t.readMarble(stub, args)
	case "readMarblePrivateDetails":
		// read a marble private details
		return t.readMarblePrivateDetails(stub, args)
	case "transferMarble":
		// change owner of a specific marble
		return t.transferMarble(stub, args)
	case "delete":
		// delete a marble
		return t.delete(stub, args)
	case "purge":
		// purge a marble
		return t.purge(stub, args)
	case "getMarblesByRange":
		// get marbles based on range query
		return t.getMarblesByRange(stub, args)
	case "getMarbleHash":
		// get private data hash for collectionMarbles
		return t.getMarbleHash(stub, args)
	case "getMarblePrivateDetailsHash":
		// get private data hash for collectionMarblePrivateDetails
		return t.getMarblePrivateDetailsHash(stub, args)
	case "setStateBasedEndorsementPolicy":
		// get private data hash for collectionMarblePrivateDetails
		return t.setStateBasedEndorsementPolicy(stub, args)
	case "checkEndorsingOrg":
		// check mspid of the current peer
		return t.checkEndorsingOrg(stub)
	default:
		// error
		fmt.Println("invoke did not find func: " + function)
		return shim.Error("Received unknown function invocation")
	}
}

// ============================================================
// initMarble - create a new marble, store into chaincode state
// ============================================================
func (t *MarblesPrivateChaincode) initMarble(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var err error

	type marbleTransientInput struct {
		Name  string `json:"name"` // the fieldtags are needed to keep case from bouncing around
		Color string `json:"color"`
		Size  int    `json:"size"`
		Owner string `json:"owner"`
		Price int    `json:"price"`
	}

	// ==== Input sanitation ====
	fmt.Println("- start init marble")

	if len(args) != 0 {
		return shim.Error("Incorrect number of arguments. Private marble data must be passed in transient map.")
	}

	transMap, err := stub.GetTransient()
	if err != nil {
		return shim.Error("Error getting transient: " + err.Error())
	}

	marbleJsonBytes, ok := transMap["marble"]
	if !ok {
		return shim.Error("marble must be a key in the transient map")
	}

	if len(marbleJsonBytes) == 0 {
		return shim.Error("marble value in the transient map must be a non-empty JSON string")
	}

	var marbleInput marbleTransientInput
	err = json.Unmarshal(marbleJsonBytes, &marbleInput)
	if err != nil {
		return shim.Error("Failed to decode JSON of: " + string(marbleJsonBytes))
	}

	if len(marbleInput.Name) == 0 {
		return shim.Error("name field must be a non-empty string")
	}
	if len(marbleInput.Color) == 0 {
		return shim.Error("color field must be a non-empty string")
	}
	if marbleInput.Size <= 0 {
		return shim.Error("size field must be a positive integer")
	}
	if len(marbleInput.Owner) == 0 {
		return shim.Error("owner field must be a non-empty string")
	}
	if marbleInput.Price <= 0 {
		return shim.Error("price field must be a positive integer")
	}

	// ==== Check if marble already exists ====
	marbleAsBytes, err := stub.GetPrivateData("collectionMarbles", marbleInput.Name)
	if err != nil {
		return shim.Error("Failed to get marble: " + err.Error())
	} else if marbleAsBytes != nil {
		fmt.Println("This marble already exists: " + marbleInput.Name)
		return shim.Error("This marble already exists: " + marbleInput.Name)
	}

	// ==== Create marble object and marshal to JSON ====
	marble := &marble{
		ObjectType: "marble",
		Name:       marbleInput.Name,
		Color:      marbleInput.Color,
		Size:       marbleInput.Size,
		Owner:      marbleInput.Owner,
	}
	marbleJSONasBytes, err := json.Marshal(marble)
	if err != nil {
		return shim.Error(err.Error())
	}

	// === Save marble to state ===
	err = stub.PutPrivateData("collectionMarbles", marbleInput.Name, marbleJSONasBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	// ==== Create marble private details object with price, marshal to JSON, and save to state ====
	marblePrivateDetails := &marblePrivateDetails{
		ObjectType: "marblePrivateDetails",
		Name:       marbleInput.Name,
		Price:      marbleInput.Price,
	}
	marblePrivateDetailsBytes, err := json.Marshal(marblePrivateDetails)
	if err != nil {
		return shim.Error(err.Error())
	}
	err = stub.PutPrivateData("collectionMarblePrivateDetails", marbleInput.Name, marblePrivateDetailsBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	//  ==== Index the marble to enable color-based range queries, e.g. return all blue marbles ====
	//  An 'index' is a normal key/value entry in state.
	//  The key is a composite key, with the elements that you want to range query on listed first.
	//  In our case, the composite key is based on indexName~color~name.
	//  This will enable very efficient state range queries based on composite keys matching indexName~color~*
	indexName := "color~name"
	colorNameIndexKey, err := stub.CreateCompositeKey(indexName, []string{marble.Color, marble.Name})
	if err != nil {
		return shim.Error(err.Error())
	}
	//  Save index entry to state. Only the key name is needed, no need to store a duplicate copy of the marble.
	//  Note - passing a 'nil' value will effectively delete the key from state, therefore we pass null character as value
	value := []byte{0x00}
	err = stub.PutPrivateData("collectionMarbles", colorNameIndexKey, value)
	if err != nil {
		return shim.Error(err.Error())
	}

	// ==== Marble saved and indexed. Return success ====
	fmt.Println("- end init marble")
	return shim.Success(nil)
}

// ===============================================
// readMarble - read a marble from chaincode state
// ===============================================
func (t *MarblesPrivateChaincode) readMarble(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var name, jsonResp string
	var err error

	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting name of the marble to query")
	}

	name = args[0]
	valAsbytes, err := stub.GetPrivateData("collectionMarbles", name) // get the marble from chaincode state
	if err != nil {
		jsonResp = "{\"Error\":\"Failed to get state for " + name + ": " + err.Error() + "\"}"
		return shim.Error(jsonResp)
	} else if valAsbytes == nil {
		jsonResp = "{\"Error\":\"Marble does not exist: " + name + "\"}"
		return shim.Error(jsonResp)
	}

	return shim.Success(valAsbytes)
}

// ===============================================
// readMarblereadMarblePrivateDetails - read a marble private details from chaincode state
// ===============================================
func (t *MarblesPrivateChaincode) readMarblePrivateDetails(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var name, jsonResp string
	var err error

	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting name of the marble to query")
	}

	name = args[0]
	valAsbytes, err := stub.GetPrivateData("collectionMarblePrivateDetails", name) // get the marble private details from chaincode state
	if err != nil {
		jsonResp = "{\"Error\":\"Failed to get private details for " + name + ": " + err.Error() + "\"}"
		return shim.Error(jsonResp)
	} else if valAsbytes == nil {
		jsonResp = "{\"Error\":\"Marble private details does not exist: " + name + "\"}"
		return shim.Error(jsonResp)
	}

	return shim.Success(valAsbytes)
}

// ===============================================
// getMarbleHash - get marble private data hash for collectionMarbles from chaincode state
// ===============================================
func (t *MarblesPrivateChaincode) getMarbleHash(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var name, jsonResp string
	var err error

	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting name of the marble to query")
	}

	name = args[0]
	valAsbytes, err := stub.GetPrivateDataHash("collectionMarbles", name)
	if err != nil {
		jsonResp = "{\"Error\":\"Failed to get marble private data hash for " + name + "\"}"
		return shim.Error(jsonResp)
	} else if valAsbytes == nil {
		jsonResp = "{\"Error\":\"Marble private marble data hash does not exist: " + name + "\"}"
		return shim.Error(jsonResp)
	}

	return shim.Success(valAsbytes)
}

// ===============================================
// getMarblePrivateDetailsHash - get marble private data hash for collectionMarblePrivateDetails from chaincode state
// ===============================================
func (t *MarblesPrivateChaincode) getMarblePrivateDetailsHash(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var name, jsonResp string
	var err error

	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting name of the marble to query")
	}

	name = args[0]
	valAsbytes, err := stub.GetPrivateDataHash("collectionMarblePrivateDetails", name)
	if err != nil {
		jsonResp = "{\"Error\":\"Failed to get marble private details hash for " + name + ": " + err.Error() + "\"}"
		return shim.Error(jsonResp)
	} else if valAsbytes == nil {
		jsonResp = "{\"Error\":\"Marble private details hash does not exist: " + name + "\"}"
		return shim.Error(jsonResp)
	}

	return shim.Success(valAsbytes)
}

// ==================================================
// delete - remove a marble key/value pair from state
// ==================================================
func (t *MarblesPrivateChaincode) delete(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	fmt.Println("- start delete marble")

	type marbleDeleteTransientInput struct {
		Name string `json:"name"`
	}

	if len(args) != 0 {
		return shim.Error("Incorrect number of arguments. Private marble name must be passed in transient map.")
	}

	transMap, err := stub.GetTransient()
	if err != nil {
		return shim.Error("Error getting transient: " + err.Error())
	}

	marbleDeleteJsonBytes, ok := transMap["marble_delete"]
	if !ok {
		return shim.Error("marble_delete must be a key in the transient map")
	}

	if len(marbleDeleteJsonBytes) == 0 {
		return shim.Error("marble_delete value in the transient map must be a non-empty JSON string")
	}

	var marbleDeleteInput marbleDeleteTransientInput
	err = json.Unmarshal(marbleDeleteJsonBytes, &marbleDeleteInput)
	if err != nil {
		return shim.Error("Failed to decode JSON of: " + string(marbleDeleteJsonBytes))
	}

	if len(marbleDeleteInput.Name) == 0 {
		return shim.Error("name field must be a non-empty string")
	}

	// to maintain the color~name index, we need to read the marble first and get its color
	valAsbytes, err := stub.GetPrivateData("collectionMarbles", marbleDeleteInput.Name) // get the marble from chaincode state
	if err != nil {
		return shim.Error("Failed to get state for " + marbleDeleteInput.Name)
	} else if valAsbytes == nil {
		return shim.Error("Marble does not exist: " + marbleDeleteInput.Name)
	}

	var marbleToDelete marble
	err = json.Unmarshal([]byte(valAsbytes), &marbleToDelete)
	if err != nil {
		return shim.Error("Failed to decode JSON of: " + string(valAsbytes))
	}

	// delete the marble from state
	err = stub.DelPrivateData("collectionMarbles", marbleDeleteInput.Name)
	if err != nil {
		return shim.Error("Failed to delete state:" + err.Error())
	}

	// Also delete the marble from the color~name index
	indexName := "color~name"
	colorNameIndexKey, err := stub.CreateCompositeKey(indexName, []string{marbleToDelete.Color, marbleToDelete.Name})
	if err != nil {
		return shim.Error(err.Error())
	}
	err = stub.DelPrivateData("collectionMarbles", colorNameIndexKey)
	if err != nil {
		return shim.Error("Failed to delete state:" + err.Error())
	}

	// Finally, delete private details of marble
	err = stub.DelPrivateData("collectionMarblePrivateDetails", marbleDeleteInput.Name)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

// =====================================================
// purge - remove a marble key/value pair from state and
// remove all trace of private details
// =====================================================
func (t *MarblesPrivateChaincode) purge(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	fmt.Println("- start purge marble")

	type marblePurgeTransientInput struct {
		Name string `json:"name"`
	}

	if len(args) != 0 {
		return shim.Error("Incorrect number of arguments. Private marble name must be passed in transient map.")
	}

	transMap, err := stub.GetTransient()
	if err != nil {
		return shim.Error("Error getting transient: " + err.Error())
	}

	marblePurgeJsonBytes, ok := transMap["marble_purge"]
	if !ok {
		return shim.Error("marble_purge must be a key in the transient map")
	}

	if len(marblePurgeJsonBytes) == 0 {
		return shim.Error("marble_purge value in the transient map must be a non-empty JSON string")
	}

	var marblePurgeInput marblePurgeTransientInput
	err = json.Unmarshal(marblePurgeJsonBytes, &marblePurgeInput)
	if err != nil {
		return shim.Error("Failed to decode JSON of: " + string(marblePurgeJsonBytes))
	}

	if len(marblePurgeInput.Name) == 0 {
		return shim.Error("name field must be a non-empty string")
	}

	// to maintain the color~name index, we need to read the marble first and get its color
	valAsbytes, err := stub.GetPrivateData("collectionMarbles", marblePurgeInput.Name) // get the marble from chaincode state
	if err != nil {
		return shim.Error("Failed to get state for " + marblePurgeInput.Name)
	} else if valAsbytes == nil {
		return shim.Error("Marble does not exist: " + marblePurgeInput.Name)
	}

	var marbleToPurge marble
	err = json.Unmarshal([]byte(valAsbytes), &marbleToPurge)
	if err != nil {
		return shim.Error("Failed to decode JSON of: " + string(valAsbytes))
	}

	// purge the marble from state
	err = stub.PurgePrivateData("collectionMarbles", marblePurgeInput.Name)
	if err != nil {
		return shim.Error("Failed to purge state:" + err.Error())
	}

	// Also purge the marble from the color~name index
	indexName := "color~name"
	colorNameIndexKey, err := stub.CreateCompositeKey(indexName, []string{marbleToPurge.Color, marbleToPurge.Name})
	if err != nil {
		return shim.Error(err.Error())
	}
	err = stub.PurgePrivateData("collectionMarbles", colorNameIndexKey)
	if err != nil {
		return shim.Error("Failed to purge state:" + err.Error())
	}

	// Finally, purge private details of marble
	err = stub.PurgePrivateData("collectionMarblePrivateDetails", marblePurgeInput.Name)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

// ===========================================================
// transfer a marble by setting a new owner name on the marble
// ===========================================================
func (t *MarblesPrivateChaincode) transferMarble(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	fmt.Println("- start transfer marble")

	type marbleTransferTransientInput struct {
		Name  string `json:"name"`
		Owner string `json:"owner"`
	}

	if len(args) != 0 {
		return shim.Error("Incorrect number of arguments. Private marble data must be passed in transient map.")
	}

	transMap, err := stub.GetTransient()
	if err != nil {
		return shim.Error("Error getting transient: " + err.Error())
	}

	marbleOwnerJsonBytes, ok := transMap["marble_owner"]
	if !ok {
		return shim.Error("marble_owner must be a key in the transient map")
	}

	if len(marbleOwnerJsonBytes) == 0 {
		return shim.Error("marble_owner value in the transient map must be a non-empty JSON string")
	}

	var marbleTransferInput marbleTransferTransientInput
	err = json.Unmarshal(marbleOwnerJsonBytes, &marbleTransferInput)
	if err != nil {
		return shim.Error("Failed to decode JSON of: " + string(marbleOwnerJsonBytes))
	}

	if len(marbleTransferInput.Name) == 0 {
		return shim.Error("name field must be a non-empty string")
	}
	if len(marbleTransferInput.Owner) == 0 {
		return shim.Error("owner field must be a non-empty string")
	}

	marbleAsBytes, err := stub.GetPrivateData("collectionMarbles", marbleTransferInput.Name)
	if err != nil {
		return shim.Error("Failed to get marble:" + err.Error())
	} else if marbleAsBytes == nil {
		return shim.Error("Marble does not exist: " + marbleTransferInput.Name)
	}

	marbleToTransfer := marble{}
	err = json.Unmarshal(marbleAsBytes, &marbleToTransfer) // unmarshal it aka JSON.parse()
	if err != nil {
		return shim.Error(err.Error())
	}
	marbleToTransfer.Owner = marbleTransferInput.Owner // change the owner

	marbleJSONasBytes, _ := json.Marshal(marbleToTransfer)
	err = stub.PutPrivateData("collectionMarbles", marbleToTransfer.Name, marbleJSONasBytes) // rewrite the marble
	if err != nil {
		return shim.Error(err.Error())
	}

	fmt.Println("- end transferMarble (success)")
	return shim.Success(nil)
}

// ===========================================================================================
// getMarblesByRange performs a range query based on the start and end keys provided.

// Read-only function results are not typically submitted to ordering. If the read-only
// results are submitted to ordering, or if the query is used in an update transaction
// and submitted to ordering, then the committing peers will re-execute to guarantee that
// result sets are stable between endorsement time and commit time. The transaction is
// invalidated by the committing peers if the result set has changed between endorsement
// time and commit time.
// Therefore, range queries are a safe option for performing update transactions based on query results.
// ===========================================================================================
func (t *MarblesPrivateChaincode) getMarblesByRange(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	if len(args) < 2 {
		return shim.Error("Incorrect number of arguments. Expecting 2")
	}

	startKey := args[0]
	endKey := args[1]

	resultsIterator, err := stub.GetPrivateDataByRange("collectionMarbles", startKey, endKey)
	if err != nil {
		return shim.Error(err.Error())
	}
	defer resultsIterator.Close()

	// buffer is a JSON array containing QueryResults
	var buffer bytes.Buffer
	buffer.WriteString("[")

	bArrayMemberAlreadyWritten := false
	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return shim.Error(err.Error())
		}
		// Add a comma before array members, suppress it for the first array member
		if bArrayMemberAlreadyWritten {
			buffer.WriteString(",")
		}

		buffer.WriteString(
			fmt.Sprintf(
				`{"Key":"%s", "Record":%s}`,
				queryResponse.Key, queryResponse.Value,
			),
		)
		bArrayMemberAlreadyWritten = true
	}
	buffer.WriteString("]")

	fmt.Printf("- getMarblesByRange queryResult:\n%s\n", buffer.String())

	return shim.Success(buffer.Bytes())
}

// ============================================================
// setStateBasedEndorsementPolicy - set key based endorsement policy
// ============================================================
func (t *MarblesPrivateChaincode) setStateBasedEndorsementPolicy(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	fmt.Println("- start set marble endorsement policy")

	type marblePolicyTransientInput struct {
		Name string `json:"name"`
		Org  string `json:"org"`
	}

	if len(args) != 0 {
		return shim.Error("Incorrect number of arguments. Private marble data must be passed in transient map.")
	}

	transMap, err := stub.GetTransient()
	if err != nil {
		return shim.Error("Error getting transient: " + err.Error())
	}

	marblePolicyJsonBytes, ok := transMap["marble_ep"]
	if !ok {
		return shim.Error("marble_ep must be a key in the transient map")
	}

	if len(marblePolicyJsonBytes) == 0 {
		return shim.Error("marble_purge value in the transient map must be a non-empty JSON string")
	}

	var marblePolicyInput marblePolicyTransientInput
	err = json.Unmarshal(marblePolicyJsonBytes, &marblePolicyInput)
	if err != nil {
		return shim.Error("Failed to decode JSON of: " + string(marblePolicyJsonBytes))
	}

	if len(marblePolicyInput.Name) == 0 {
		return shim.Error("name field must be a non-empty string")
	}

	if len(marblePolicyInput.Org) == 0 {
		return shim.Error("org field must be a non-empty string")
	}

	// Set new endorsement policy
	var epBytes []byte
	ep, err := statebased.NewStateEP(epBytes)
	if err != nil {
		return shim.Error("Error creating state-based endorsement policy: " + err.Error())
	}

	err = ep.AddOrgs(statebased.RoleTypePeer, marblePolicyInput.Org)
	if err != nil {
		return shim.Error("Error updating state-based endorsement policy: " + err.Error())
	}

	epBytes, err = ep.Policy()
	if err != nil {
		return shim.Error("Error encoding state-based endorsement policy: " + err.Error())
	}

	err = stub.SetPrivateDataValidationParameter("collectionMarbles", marblePolicyInput.Name, epBytes)
	if err != nil {
		return shim.Error("Error setting private data key-level endorsement policy: " + err.Error())
	}

	fmt.Println("- end set marble endorsement policy (success)")
	return shim.Success(nil)
}

// CheckEndorsingOrg checks that the peer org is present in the given transient data
func (t *MarblesPrivateChaincode) checkEndorsingOrg(stub shim.ChaincodeStubInterface) pb.Response {
	transient, err := stub.GetTransient()
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get transient data: %v", err))
	}

	peerOrgMSP, err := shim.GetMSPID()
	if err != nil {
		return shim.Error(fmt.Sprintf("failed getting client's orgID: %v", err))
	}

	var result string
	if _, ok := transient[peerOrgMSP]; ok {
		result = "Peer mspid OK"
	} else {
		expectedMSPs := make([]string, 0, len(transient))
		for k := range transient {
			expectedMSPs = append(expectedMSPs, k)
		}

		result = fmt.Sprintf("Unexpected peer mspid! Expected MSP IDs: %s Actual MSP ID: %s", expectedMSPs, peerOrgMSP)
	}

	return shim.Success([]byte(result))
}
