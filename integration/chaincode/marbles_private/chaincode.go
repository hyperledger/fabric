/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package marbles_private

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// MarblesPrivateChaincode example Chaincode implementation
type MarblesPrivateChaincode struct {
}

type marble struct {
	ObjectType string `json:"docType"` //docType is used to distinguish the various types of objects in state database
	Name       string `json:"name"`    //the fieldtags are needed to keep case from bouncing around
	Color      string `json:"color"`
	Size       int    `json:"size"`
	Owner      string `json:"owner"`
}

type marblePrivateDetails struct {
	ObjectType string `json:"docType"` //docType is used to distinguish the various types of objects in state database
	Name       string `json:"name"`    //the fieldtags are needed to keep case from bouncing around
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
		//create a new marble
		return t.initMarble(stub, args)
	case "readMarble":
		//read a marble
		return t.readMarble(stub, args)
	case "readMarblePrivateDetails":
		//read a marble private details
		return t.readMarblePrivateDetails(stub, args)
	case "getMarbleHash":
		// get private data hash for collectionMarbles
		return t.getMarbleHash(stub, args)
	case "getMarblePrivateDetailsHash":
		// get private data hash for collectionMarblePrivateDetails
		return t.getMarblePrivateDetailsHash(stub, args)
	case "delete":
		//delete a marble
		return t.delete(stub, args)
	default:
		//error
		fmt.Println("invoke did not find func: " + function)
		return shim.Error("Received unknown function invocation")
	}
}

// ============================================================
// initMarble - create a new marble, store into chaincode state
// ============================================================
func (t *MarblesPrivateChaincode) initMarble(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var err error

	//  0-name  1-color  2-size  3-owner  4-price
	// "asdf",  "blue",  "35",   "bob",   "99"
	if len(args) != 5 {
		return shim.Error("Incorrect number of arguments. Expecting 5")
	}

	// ==== Input sanitation ====
	fmt.Println("- start init marble")
	if len(args[0]) == 0 {
		return shim.Error("1st argument must be a non-empty string")
	}
	if len(args[1]) == 0 {
		return shim.Error("2nd argument must be a non-empty string")
	}
	if len(args[2]) == 0 {
		return shim.Error("3rd argument must be a non-empty string")
	}
	if len(args[3]) == 0 {
		return shim.Error("4th argument must be a non-empty string")
	}
	if len(args[4]) == 0 {
		return shim.Error("5th argument must be a non-empty string")
	}
	marbleName := args[0]
	color := strings.ToLower(args[1])
	owner := strings.ToLower(args[3])
	size, err := strconv.Atoi(args[2])
	if err != nil {
		return shim.Error("3rd argument must be a numeric string")
	}
	price, err := strconv.Atoi(args[4])
	if err != nil {
		return shim.Error("5th argument must be a numeric string")
	}

	// ==== Check if marble already exists ====
	marbleAsBytes, err := stub.GetPrivateData("collectionMarbles", marbleName)
	if err != nil {
		return shim.Error("Failed to get marble: " + err.Error())
	} else if marbleAsBytes != nil {
		fmt.Println("This marble already exists: " + marbleName)
		return shim.Error("This marble already exists: " + marbleName)
	}

	// ==== Create marble object and marshal to JSON ====
	objectType := "marble"
	marble := &marble{objectType, marbleName, color, size, owner}
	marbleJSONasBytes, err := json.Marshal(marble)
	if err != nil {
		return shim.Error(err.Error())
	}
	//Alternatively, build the marble json string manually if you don't want to use struct marshalling
	//marbleJSONasString := `{"docType":"Marble",  "name": "` + marbleName + `", "color": "` + color + `", "size": ` + strconv.Itoa(size) + `, "owner": "` + owner + `"}`
	//marbleJSONasBytes := []byte(str)

	// === Save marble to state ===
	err = stub.PutPrivateData("collectionMarbles", marbleName, marbleJSONasBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	// ==== Save marble private details ====
	objectType = "marblePrivateDetails"
	marblePrivateDetails := &marblePrivateDetails{objectType, marbleName, price}
	marblePrivateDetailsBytes, err := json.Marshal(marblePrivateDetails)
	if err != nil {
		return shim.Error(err.Error())
	}
	err = stub.PutPrivateData("collectionMarblePrivateDetails", marbleName, marblePrivateDetailsBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	// ==== Marble saved. Return success ====
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
	valAsbytes, err := stub.GetPrivateData("collectionMarbles", name) //get the marble from chaincode state
	if err != nil {
		jsonResp = "{\"Error\":\"Failed to get state for " + name + "\"}"
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
	valAsbytes, err := stub.GetPrivateData("collectionMarblePrivateDetails", name) //get the marble private details from chaincode state
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
	var jsonResp string
	var marbleJSON marble
	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting 1")
	}
	marbleName := args[0]

	// to maintain the color~name index, we need to read the marble first and get its color
	valAsbytes, err := stub.GetPrivateData("collectionMarbles", marbleName) //get the marble from chaincode state
	if err != nil {
		jsonResp = "{\"Error\":\"Failed to get state for " + marbleName + "\"}"
		return shim.Error(jsonResp)
	} else if valAsbytes == nil {
		jsonResp = "{\"Error\":\"Marble does not exist: " + marbleName + "\"}"
		return shim.Error(jsonResp)
	}

	err = json.Unmarshal([]byte(valAsbytes), &marbleJSON)
	if err != nil {
		jsonResp = "{\"Error\":\"Failed to decode JSON of: " + marbleName + "\"}"
		return shim.Error(jsonResp)
	}

	err = stub.DelPrivateData("collectionMarbles", marbleName) //remove the marble from chaincode state
	if err != nil {
		return shim.Error("Failed to delete state:" + err.Error())
	}

	// maintain the index
	indexName := "color~name"
	colorNameIndexKey, err := stub.CreateCompositeKey(indexName, []string{marbleJSON.Color, marbleJSON.Name})
	if err != nil {
		return shim.Error(err.Error())
	}

	//  Delete index entry to state.
	err = stub.DelPrivateData("collectionMarbles", colorNameIndexKey)
	if err != nil {
		return shim.Error("Failed to delete state:" + err.Error())
	}

	//  Delete private details of marble
	err = stub.DelPrivateData("collectionMarblePrivateDetails", marbleName)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}
