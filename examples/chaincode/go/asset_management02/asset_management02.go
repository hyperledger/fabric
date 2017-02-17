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

package main

import (
	"encoding/base64"
	"encoding/binary"
	"strconv"
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/op/go-logging"
)

var myLogger = logging.MustGetLogger("asset_mgm")

var cHandler = NewCertHandler()
var dHandler = NewDepositoryHandler()

//AssetManagementChaincode APIs exposed to chaincode callers
type AssetManagementChaincode struct {
}

// assignOwnership assigns assets to a given account ID, only entities with the "issuer" are allowed to call this function
// Note: this issuer can only allocate balance to one account ID at a time
// args[0]: investor's TCert
// args[1]: attribute name inside the investor's TCert that contains investor's account ID
// args[2]: amount to be assigned to this investor's account ID
func (t *AssetManagementChaincode) assignOwnership(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	myLogger.Debugf("+++++++++++++++++++++++++++++++++++assignOwnership+++++++++++++++++++++++++++++++++")

	if len(args) != 3 {
		return shim.Error("Incorrect number of arguments. Expecting 0")
	}

	//check is invoker has the correct role, only invokers with the "issuer" role is allowed to
	//assign asset to owners
	isAuthorized, err := cHandler.isAuthorized(stub, "issuer")
	if !isAuthorized {
		myLogger.Errorf("system error %v", err)
		return shim.Error("user is not aurthorized to assign assets")
	}

	owner, err := base64.StdEncoding.DecodeString(args[0])
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return shim.Error("Failed decoding owner")
	}
	accountAttribute := args[1]

	amount, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return shim.Error("Unable to parse amount" + args[2])
	}

	//retrieve account IDs from investor's TCert
	accountIDs, err := cHandler.getAccountIDsFromAttribute(owner, []string{accountAttribute})
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return shim.Error("Unable to retrieve account Ids from user certificate " + args[1])
	}

	//retreive investors' contact info (e.g. phone number, email, home address)
	//from investors' TCert. Ideally, this information shall be encrypted with issuer's pub key or KA key
	//between investor and issuer, so that only issuer can view such information
	contactInfo, err := cHandler.getContactInfo(owner)
	if err != nil {
		return shim.Error("Unable to retrieve contact info from user certificate " + args[1])
	}

	//call DeposistoryHandler.assign function to put the "amount" and "contact info" under this account ID
	err = dHandler.assign(stub, accountIDs[0], contactInfo, amount)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

// transferOwnership moves x number of assets from account A to account B
// args[0]: Investor TCert that has account IDs which will their balances deducted
// args[1]: attribute names inside TCert (arg[0]) that countain the account IDs
// args[2]: Investor TCert that has account IDs which will have their balances increased
// args[3]: attribute names inside TCert (arg[2]) that countain the account IDs
func (t *AssetManagementChaincode) transferOwnership(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	myLogger.Debugf("+++++++++++++++++++++++++++++++++++transferOwnership+++++++++++++++++++++++++++++++++")

	if len(args) != 5 {
		return shim.Error("Incorrect number of arguments. Expecting 0")
	}

	fromOwner, err := base64.StdEncoding.DecodeString(args[0])
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return shim.Error("Failed decoding fromOwner")
	}
	fromAccountAttributes := strings.Split(args[1], ",")

	toOwner, err := base64.StdEncoding.DecodeString(args[2])
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return shim.Error("Failed decoding owner")
	}
	toAccountAttributes := strings.Split(args[3], ",")

	amount, err := strconv.ParseUint(args[4], 10, 64)
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return shim.Error("Unable to parse amount" + args[4])
	}

	// retrieve account IDs from "transfer from" TCert
	fromAccountIds, err := cHandler.getAccountIDsFromAttribute(fromOwner, fromAccountAttributes)
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return shim.Error("Unable to retrieve contact info from user certificate" + args[1])
	}

	// retrieve account IDs from "transfer to" TCert
	toAccountIds, err := cHandler.getAccountIDsFromAttribute(toOwner, toAccountAttributes)
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return shim.Error("Unable to retrieve contact info from user certificate" + args[3])
	}

	// retrieve contact info from "transfer to" TCert
	contactInfo, err := cHandler.getContactInfo(toOwner)
	if err != nil {
		myLogger.Errorf("system error %v received", err)
		return shim.Error("Unable to retrieve contact info from user certificate" + args[4])
	}

	// call dHandler.transfer to transfer to transfer "amount" from "from account" IDs to "to account" IDs
	err = dHandler.transfer(stub, fromAccountIds, toAccountIds[0], contactInfo, amount)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

// getOwnerContactInformation retrieves the contact information of the investor that owns a particular account ID
// Note: user contact information shall be encrypted with issuer's pub key or KA key
// between investor and issuer, so that only issuer can decrypt such information
// args[0]: one of the many account IDs owned by "some" investor
func (t *AssetManagementChaincode) getOwnerContactInformation(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	myLogger.Debugf("+++++++++++++++++++++++++++++++++++getOwnerContactInformation+++++++++++++++++++++++++++++++++")

	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting 0")
	}

	accountID := args[0]

	email, err := dHandler.queryContactInfo(stub, accountID)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success([]byte(email))
}

// getBalance retrieves the account balance information of the investor that owns a particular account ID
// args[0]: one of the many account IDs owned by "some" investor
func (t *AssetManagementChaincode) getBalance(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	myLogger.Debugf("+++++++++++++++++++++++++++++++++++getBalance+++++++++++++++++++++++++++++++++")

	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting 0")
	}

	accountID := args[0]

	balance, err := dHandler.queryBalance(stub, accountID)
	if err != nil {
		return shim.Error(err.Error())
	}

	//convert balance (uint64) to []byte (Big Endian)
	ret := make([]byte, 8)
	binary.BigEndian.PutUint64(ret, balance)

	return shim.Success(ret)
}

// Init initialization, this method will create asset despository in the chaincode state
func (t *AssetManagementChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	_, args := stub.GetFunctionAndParameters()
	myLogger.Debugf("********************************Init****************************************")

	myLogger.Info("[AssetManagementChaincode] Init")
	if len(args) != 0 {
		return shim.Error("Incorrect number of arguments. Expecting 0")
	}

	return shim.Success(nil)
}

// Invoke method is the interceptor of all invocation transactions, its job is to direct
// invocation transactions to intended APIs
func (t *AssetManagementChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	myLogger.Debugf("********************************Invoke****************************************")

	//	 Handle different functions
	if function == "assignOwnership" {
		// Assign ownership
		return t.assignOwnership(stub, args)
	} else if function == "transferOwnership" {
		// Transfer ownership
		return t.transferOwnership(stub, args)
	} else if function == "getOwnerContactInformation" {
		return t.getOwnerContactInformation(stub, args)
	} else if function == "getBalance" {
		return t.getBalance(stub, args)
	}

	return shim.Error("Received unknown function invocation")
}

func main() {

	//	primitives.SetSecurityLevel("SHA3", 256)
	err := shim.Start(new(AssetManagementChaincode))
	if err != nil {
		myLogger.Debugf("Error starting AssetManagementChaincode: %s", err)
	}

}
