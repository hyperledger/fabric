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
	"errors"
	"strconv"
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/shim"
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
func (t *AssetManagementChaincode) assignOwnership(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	myLogger.Debugf("+++++++++++++++++++++++++++++++++++assignOwnership+++++++++++++++++++++++++++++++++")

	if len(args) != 3 {
		return nil, errors.New("Incorrect number of arguments. Expecting 0")
	}

	//check is invoker has the correct role, only invokers with the "issuer" role is allowed to
	//assign asset to owners
	isAuthorized, err := cHandler.isAuthorized(stub, "issuer")
	if !isAuthorized {
		myLogger.Errorf("system error %v", err)
		return nil, errors.New("user is not aurthorized to assign assets")
	}

	owner, err := base64.StdEncoding.DecodeString(args[0])
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return nil, errors.New("Failed decoding owner")
	}
	accountAttribute := args[1]

	amount, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return nil, errors.New("Unable to parse amount" + args[2])
	}

	//retrieve account IDs from investor's TCert
	accountIDs, err := cHandler.getAccountIDsFromAttribute(owner, []string{accountAttribute})
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return nil, errors.New("Unable to retrieve account Ids from user certificate " + args[1])
	}

	//retreive investors' contact info (e.g. phone number, email, home address)
	//from investors' TCert. Ideally, this information shall be encrypted with issuer's pub key or KA key
	//between investor and issuer, so that only issuer can view such information
	contactInfo, err := cHandler.getContactInfo(owner)
	if err != nil {
		return nil, errors.New("Unable to retrieve contact info from user certificate " + args[1])
	}

	//call DeposistoryHandler.assign function to put the "amount" and "contact info" under this account ID
	return nil, dHandler.assign(stub, accountIDs[0], contactInfo, amount)
}

// transferOwnership moves x number of assets from account A to account B
// args[0]: Investor TCert that has account IDs which will their balances deducted
// args[1]: attribute names inside TCert (arg[0]) that countain the account IDs
// args[2]: Investor TCert that has account IDs which will have their balances increased
// args[3]: attribute names inside TCert (arg[2]) that countain the account IDs
func (t *AssetManagementChaincode) transferOwnership(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	myLogger.Debugf("+++++++++++++++++++++++++++++++++++transferOwnership+++++++++++++++++++++++++++++++++")

	if len(args) != 5 {
		return nil, errors.New("Incorrect number of arguments. Expecting 0")
	}

	fromOwner, err := base64.StdEncoding.DecodeString(args[0])
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return nil, errors.New("Failed decoding fromOwner")
	}
	fromAccountAttributes := strings.Split(args[1], ",")

	toOwner, err := base64.StdEncoding.DecodeString(args[2])
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return nil, errors.New("Failed decoding owner")
	}
	toAccountAttributes := strings.Split(args[3], ",")

	amount, err := strconv.ParseUint(args[4], 10, 64)
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return nil, errors.New("Unable to parse amount" + args[4])
	}

	// retrieve account IDs from "transfer from" TCert
	fromAccountIds, err := cHandler.getAccountIDsFromAttribute(fromOwner, fromAccountAttributes)
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return nil, errors.New("Unable to retrieve contact info from user certificate" + args[1])
	}

	// retrieve account IDs from "transfer to" TCert
	toAccountIds, err := cHandler.getAccountIDsFromAttribute(toOwner, toAccountAttributes)
	if err != nil {
		myLogger.Errorf("system error %v", err)
		return nil, errors.New("Unable to retrieve contact info from user certificate" + args[3])
	}

	// retrieve contact info from "transfer to" TCert
	contactInfo, err := cHandler.getContactInfo(toOwner)
	if err != nil {
		myLogger.Errorf("system error %v received", err)
		return nil, errors.New("Unable to retrieve contact info from user certificate" + args[4])
	}

	// call dHandler.transfer to transfer to transfer "amount" from "from account" IDs to "to account" IDs
	return nil, dHandler.transfer(stub, fromAccountIds, toAccountIds[0], contactInfo, amount)
}

// getOwnerContactInformation retrieves the contact information of the investor that owns a particular account ID
// Note: user contact information shall be encrypted with issuer's pub key or KA key
// between investor and issuer, so that only issuer can decrypt such information
// args[0]: one of the many account IDs owned by "some" investor
func (t *AssetManagementChaincode) getOwnerContactInformation(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	myLogger.Debugf("+++++++++++++++++++++++++++++++++++getOwnerContactInformation+++++++++++++++++++++++++++++++++")

	if len(args) != 1 {
		return nil, errors.New("Incorrect number of arguments. Expecting 0")
	}

	accountID := args[0]

	email, err := dHandler.queryContactInfo(stub, accountID)
	if err != nil {
		return nil, err
	}

	return []byte(email), nil
}

// getBalance retrieves the account balance information of the investor that owns a particular account ID
// args[0]: one of the many account IDs owned by "some" investor
func (t *AssetManagementChaincode) getBalance(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	myLogger.Debugf("+++++++++++++++++++++++++++++++++++getBalance+++++++++++++++++++++++++++++++++")

	if len(args) != 1 {
		return nil, errors.New("Incorrect number of arguments. Expecting 0")
	}

	accountID := args[0]

	balance, err := dHandler.queryBalance(stub, accountID)
	if err != nil {
		return nil, err
	}

	//convert balance (uint64) to []byte (Big Endian)
	ret := make([]byte, 8)
	binary.BigEndian.PutUint64(ret, balance)

	return ret, nil
}

// Init initialization, this method will create asset despository in the chaincode state
func (t *AssetManagementChaincode) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	myLogger.Debugf("********************************Init****************************************")

	myLogger.Info("[AssetManagementChaincode] Init")
	if len(args) != 0 {
		return nil, errors.New("Incorrect number of arguments. Expecting 0")
	}

	return nil, dHandler.createTable(stub)
}

// Invoke  method is the interceptor of all invocation transactions, its job is to direct
// invocation transactions to intended APIs
func (t *AssetManagementChaincode) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	myLogger.Debugf("********************************Invoke****************************************")

	//	 Handle different functions
	if function == "assignOwnership" {
		// Assign ownership
		return t.assignOwnership(stub, args)
	} else if function == "transferOwnership" {
		// Transfer ownership
		return t.transferOwnership(stub, args)
	}

	return nil, errors.New("Received unknown function invocation")
}

// Query method is the interceptor of all invocation transactions, its job is to direct
// query transactions to intended APIs, and return the result back to callers
func (t *AssetManagementChaincode) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	myLogger.Debugf("********************************Query****************************************")

	// Handle different functions
	if function == "getOwnerContactInformation" {
		return t.getOwnerContactInformation(stub, args)
	} else if function == "getBalance" {
		return t.getBalance(stub, args)
	}

	return nil, errors.New("Received unknown function query invocation with function " + function)
}

func main() {

	//	primitives.SetSecurityLevel("SHA3", 256)
	err := shim.Start(new(AssetManagementChaincode))
	if err != nil {
		myLogger.Debugf("Error starting AssetManagementChaincode: %s", err)
	}

}
