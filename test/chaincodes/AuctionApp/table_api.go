/******************************************************************
Copyright IT People Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

******************************************************************/

///////////////////////////////////////////////////////////////////////
// Author : IT People - Mohan Venkataraman - table API for v1.0
// Purpose: Explore the Hyperledger/fabric and understand
// how to write an chain code, application/chain code boundaries
// The code is not the best as it has just hammered out in a day or two
// Feedback and updates are appreciated
///////////////////////////////////////////////////////////////////////

package main

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

//////////////////////////////////////////////////////////////////////////////////////////////////
// The recType is a mandatory attribute. The original app was written with a single table
// in mind. The only way to know how to process a record was the 70's style 80 column punch card
// which used a record type field. The array below holds a list of valid record types.
// This could be stored on a blockchain table or an application
//////////////////////////////////////////////////////////////////////////////////////////////////
var recType = []string{"ARTINV", "USER", "BID", "AUCREQ", "POSTTRAN", "OPENAUC", "CLAUC", "XFER", "VERIFY", "DOWNLOAD"}

//////////////////////////////////////////////////////////////////////////////////////////////////
// The following array holds the list of tables that should be created
// The deploy/init deletes the tables and recreates them every time a deploy is invoked
//////////////////////////////////////////////////////////////////////////////////////////////////
var Objects = []string{"PARTY", "CASHTXN", "User", "UserCat", "Item", "ItemCat", "ItemHistory", "Auction", "AucInit", "AucOpen", "Bid", "Trans"}

/////////////////////////////////////////////////////////////////////////////////////////////////////
// A Map that holds ObjectNames and the number of Keys
// This information is used to dynamically Create, Update
// Replace , and Query the Ledger
// In this model all attributes in a table are strings
// The chain code does both validation
// A dummy key like 2016 in some cases is used for a query to get all rows
//
//              "User":        1, Key: UserID
//              "Item":        1, Key: ItemID
//              "UserCat":     3, Key: "2016", UserType, UserID
//              "ItemCat":     3, Key: "2016", ItemSubject, ItemID
//              "Auction":     1, Key: AuctionID
//              "AucInit":     2, Key: Year, AuctionID
//              "AucOpen":     2, Key: Year, AuctionID
//              "Trans":       2, Key: AuctionID, ItemID
//              "Bid":         2, Key: AuctionID, BidNo
//              "ItemHistory": 4, Key: ItemID, Status, AuctionHouseID(if applicable),date-time
//
// The additional key is the ObjectType (aka ObjectName or Object). The keys  would be
// keys: {"User", UserId} or keys: {"AuctInit", "2016", "1134"}
/////////////////////////////////////////////////////////////////////////////////////////////////////

func GetNumberOfKeys(tname string) int {
	ObjectMap := map[string]int{
		"User":        1,
		"Item":        1,
		"UserCat":     3,
		"ItemCat":     3,
		"Auction":     1,
		"AucInit":     2,
		"AucOpen":     2,
		"Trans":       2,
		"Bid":         2,
		"ItemHistory": 4,
		"PARTY":       2,
		"CASHTXN":     1,
	}
	return ObjectMap[tname]
}

/////////////////////////////////////////////////////////////////
// This function checks the incoming args for a valid record
// type entry as per the declared array recType[]
// The rectType attribute can be anywhere in the args or struct
// not necessarily in args[1] as per my old logic
// The Request type is used to direct processing
// the record accordingly e: recType is "USER"
// "Args":["PostUser","100", "USER", "Ashley Hart", "TRD",  "Morrisville Parkway, #216, Morrisville, NC 27560",
//         "9198063535", "ashley@it people.com", "SUNTRUST", "0001732345", "0234678", "2017-01-02 15:04:05"]}'
/////////////////////////////////////////////////////////////////
func ChkRecType(args []string) bool {
	for _, rt := range args {
		for _, val := range recType {
			if val == rt {
				return true
			}
		}
	}
	return false
}

/////////////////////////////////////////////////////////////////
// Checks if the incoming invoke has a valid requesType
// The Request type is used to process the record accordingly
// Old Logic (see new logic up)
/////////////////////////////////////////////////////////////////
func CheckRecType(rt string) bool {
	for _, val := range recType {
		if val == rt {
			fmt.Println("CheckRequestType() : Valid Request Type , val : ", val, rt, "\n")
			return true
		}
	}
	fmt.Println("CheckRequestType() : Invalid Request Type , val : ", rt, "\n")
	return false
}

/////////////////////////////////////////////////////////////////
// Checks if the args contain a valid Record Type. Typically, this
// model expects the Object Type to be args[2] but
// for the sake of flexibility, it scans the input data for
// a valid type if available
/////////////////////////////////////////////////////////////////
func IdentifyRecType(args []string) (string, error) {
	for _, rt := range args {
		for _, val := range recType {
			if val == rt {
				return rt, nil
			}
		}
	}
	return "", fmt.Errorf("IdentifyRecType: Not Found")
}

/////////////////////////////////////////////////////////////////
// Checks if the args contain a valid Object Type. Typically, this
// model expects the Object Type to be args[0] but
// for the sake of flexibility, it scans the input data for
// a valid type if available
/////////////////////////////////////////////////////////////////
func IdentifyObjectType(args []string) (string, error) {
	for _, rt := range args {
		for _, val := range Objects {
			if val == rt {
				return rt, nil
			}
		}
	}
	return "", fmt.Errorf("IdentifyObjectType: Object Not Found")
}

////////////////////////////////////////////////////////////////////////////
// Open a Ledgers if one does not exist
// These ledgers will be used to write /  read data
////////////////////////////////////////////////////////////////////////////
func InitObject(stub shim.ChaincodeStubInterface, objectType string, keys []string) error {

	fmt.Println(">> Not Implemented Yet << Initializing Object : ", objectType, " Keys: ", keys)
	return nil
}

////////////////////////////////////////////////////////////////////////////
// Update the Object - Replace current data with replacement
// Register users into this table
////////////////////////////////////////////////////////////////////////////
func UpdateObject(stub shim.ChaincodeStubInterface, objectType string, keys []string, objectData []byte) error {

	// Check how many keys

	err := VerifyAtLeastOneKeyIsPresent(objectType, keys)
	if err != nil {
		return err
	}

	// Convert keys to  compound key
	compositeKey, _ := stub.CreateCompositeKey(objectType, keys)

	// Add Object JSON to state
	err = stub.PutState(compositeKey, objectData)
	if err != nil {
		fmt.Println("UpdateObject() : Error inserting Object into State Database %s", err)
		return err
	}

	return nil

}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Retrieve the object based on the key and simply delete it
//
////////////////////////////////////////////////////////////////////////////////////////////////////////////
func DeleteObject(stub shim.ChaincodeStubInterface, objectType string, keys []string) error {

	// Check how many keys

	err := VerifyAtLeastOneKeyIsPresent(objectType, keys)
	if err != nil {
		return err
	}

	// Convert keys to  compound key
	compositeKey, _ := stub.CreateCompositeKey(objectType, keys)

	// Remove object from the State Database
	err = stub.DelState(compositeKey)
	if err != nil {
		fmt.Println("DeleteObject() : Error deleting Object into State Database %s", err)
		return err
	}
	fmt.Println("DeleteObject() : ", "Object : ", objectType, " Key : ", compositeKey)

	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Delete all objects of ObjectType
//
////////////////////////////////////////////////////////////////////////////////////////////////////////////
func DeleteAllObjects(stub shim.ChaincodeStubInterface, objectType string) error {

	// Convert keys to  compound key
	compositeKey, _ := stub.CreateCompositeKey(objectType, []string{""})

	// Remove object from the State Database
	err := stub.DelState(compositeKey)
	if err != nil {
		fmt.Println("DeleteAllObjects() : Error deleting all Object into State Database %s", err)
		return err
	}
	fmt.Println("DeleteObject() : ", "Object : ", objectType, " Key : ", compositeKey)

	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Replaces the Entry in the Ledger
// The existing object is simply queried and the data contents is replaced with
// new content
////////////////////////////////////////////////////////////////////////////////////////////////////////////
func ReplaceObject(stub shim.ChaincodeStubInterface, objectType string, keys []string, objectData []byte) error {

	// Check how many keys

	err := VerifyAtLeastOneKeyIsPresent(objectType, keys)
	if err != nil {
		return err
	}

	// Convert keys to  compound key
	compositeKey, _ := stub.CreateCompositeKey(objectType, keys)

	// Add Party JSON to state
	err = stub.PutState(compositeKey, objectData)
	if err != nil {
		fmt.Println("ReplaceObject() : Error replacing Object in State Database %s", err)
		return err
	}

	fmt.Println("ReplaceObject() : - end init object ", objectType)
	return nil
}

////////////////////////////////////////////////////////////////////////////
// Query a User Object by Object Name and Key
// This has to be a full key and should return only one unique object
////////////////////////////////////////////////////////////////////////////
func QueryObject(stub shim.ChaincodeStubInterface, objectType string, keys []string) ([]byte, error) {

	// Check how many keys

	err := VerifyAtLeastOneKeyIsPresent(objectType, keys)
	if err != nil {
		return nil, err
	}

	compoundKey, _ := stub.CreateCompositeKey(objectType, keys)
	fmt.Println("QueryObject() : Compound Key : ", compoundKey)

	Avalbytes, err := stub.GetState(compoundKey)
	if err != nil {
		return nil, err
	}

	return Avalbytes, nil
}

////////////////////////////////////////////////////////////////////////////
// Query a User Object by Object Name and Key
// This has to be a full key and should return only one unique object
////////////////////////////////////////////////////////////////////////////
func QueryObjectWithProcessingFunction(stub shim.ChaincodeStubInterface, objectType string, keys []string, fname func(shim.ChaincodeStubInterface, []byte, []string) error) ([]byte, error) {

	// Check how many keys

	err := VerifyAtLeastOneKeyIsPresent(objectType, keys)
	if err != nil {
		return nil, err
	}

	compoundKey, _ := stub.CreateCompositeKey(objectType, keys)
	fmt.Println("QueryObject: Compound Key : ", compoundKey)

	Avalbytes, err := stub.GetState(compoundKey)
	if err != nil {
		return nil, err
	}

	if Avalbytes == nil {
		return nil, fmt.Errorf("QueryObject: No Data Found for Compound Key : ", compoundKey)
	}

	// Perform Any additional processing of data
	fmt.Println("fname() : Successful - Proceeding to fname")

	err = fname(stub, Avalbytes, keys)
	if err != nil {
		fmt.Println("QueryLedger() : Cannot execute  : ", fname)
		jsonResp := "{\"fname() Error\":\" Cannot create Object for key " + compoundKey + "\"}"
		return Avalbytes, errors.New(jsonResp)
	}

	return Avalbytes, nil
}

////////////////////////////////////////////////////////////////////////////
// Get a List of Rows based on query criteria from the OBC
// The getList Function
////////////////////////////////////////////////////////////////////////////
func GetKeyList(stub shim.ChaincodeStubInterface, args []string) (shim.StateQueryIteratorInterface, error) {

	// Define partial key to query within objects namespace (objectType)
	objectType := args[0]

	// Check how many keys

	err := VerifyAtLeastOneKeyIsPresent(objectType, args[1:])
	if err != nil {
		return nil, err
	}

	// Execute the Query
	// This will execute a key range query on all keys starting with the compound key
	resultsIterator, err := stub.GetStateByPartialCompositeKey(objectType, args[1:])
	if err != nil {
		return nil, err
	}

	defer resultsIterator.Close()

	// Iterate through result set
	var i int
	for i = 0; resultsIterator.HasNext(); i++ {

		// Retrieve the Key and Object
		myCompositeKey, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}
		fmt.Println("GetList() : my Value : ", myCompositeKey)
	}
	return resultsIterator, nil
}

///////////////////////////////////////////////////////////////////////////////////////////
// GetQueryResultForQueryString executes the passed in query string.
// Result set is built and returned as a byte array containing the JSON results.
///////////////////////////////////////////////////////////////////////////////////////////
func GetQueryResultForQueryString(stub shim.ChaincodeStubInterface, queryString string) ([]byte, error) {

	fmt.Println("GetQueryResultForQueryString() : getQueryResultForQueryString queryString:\n%s\n", queryString)

	resultsIterator, err := stub.GetQueryResult(queryString)
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	// buffer is a JSON array containing QueryRecords
	var buffer bytes.Buffer
	buffer.WriteString("[")

	bArrayMemberAlreadyWritten := false
	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}
		// Add a comma before array members, suppress it for the first array member
		if bArrayMemberAlreadyWritten == true {
			buffer.WriteString(",")
		}
		buffer.WriteString("{\"Key\":")
		buffer.WriteString("\"")
		buffer.WriteString(queryResponse.Key)
		buffer.WriteString("\"")

		buffer.WriteString(", \"Record\":")
		// Record is a JSON object, so we write as-is
		buffer.WriteString(string(queryResponse.Value))
		buffer.WriteString("}")
		bArrayMemberAlreadyWritten = true
	}
	buffer.WriteString("]")

	fmt.Println("GetQueryResultForQueryString(): getQueryResultForQueryString queryResult:\n%s\n", buffer.String())

	return buffer.Bytes(), nil
}

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Retrieve a list of Objects from the Query
// The function returns an iterator from which objects can be retrieved.
//      defer rs.Close()
//
//      // Iterate through result set
//      var i int
//      for i = 0; rs.HasNext(); i++ {
//
//              // We can process whichever return value is of interest
//              myKey , myKeyVal , err := rs.Next()
//              if err != nil {
//                      return shim.Success(nil)
//              }
//              bob, _ := JSONtoUser(myKeyVal)
//              fmt.Println("GetList() : my Value : ", bob)
//      }
//
// eg: Args":["fetchlist", "PARTY","CHK"]}
// fetchList is the function that calls getList : ObjectType = "Party" and key is "CHK"
//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
func GetList(stub shim.ChaincodeStubInterface, objectType string, keys []string) (shim.StateQueryIteratorInterface, error) {

	// Check how many keys

	err := VerifyAtLeastOneKeyIsPresent(objectType, keys)
	if err != nil {
		return nil, err
	}

	// Get Result set
	resultIter, err := stub.GetStateByPartialCompositeKey(objectType, keys)
	fmt.Println("GetList(): Retrieving Objects into an array")
	if err != nil {
		return nil, err
	}

	// Return iterator for result set
	// Use code above to retrieve objects
	return resultIter, nil
}

////////////////////////////////////////////////////////////////////////////
// This function verifies if the number of key provided is at least 1 and
// < the max keys defined for the Object
////////////////////////////////////////////////////////////////////////////

func VerifyAtLeastOneKeyIsPresent(objectType string, args []string) error {

	// Check how many keys
	nKeys := GetNumberOfKeys(objectType)
	nCol := len(args)
	if nCol == 1 {
		return nil
	}

	if nCol < 1 {
		error_str := fmt.Sprintf("VerifyAtLeastOneKeyIsPresent() Failed: Atleast 1 Key must is needed :  nKeys : %s, nCol : %s ", nKeys, nCol)
		fmt.Println(error_str)
		return errors.New(error_str)
	}

	return nil
}
