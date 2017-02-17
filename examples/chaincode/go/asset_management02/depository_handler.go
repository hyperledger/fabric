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
	"errors"

	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

//DepositoryHandler provides APIs used to perform operations on CC's KV store
type depositoryHandler struct {
}

// NewDepositoryHandler create a new reference to CertHandler
func NewDepositoryHandler() *depositoryHandler {
	return &depositoryHandler{}
}

type depositoryAccount struct {
	AccountID   string `json:"account_id"`
	ContactInfo string `json:"contact_info"`
	Amount      uint64 `json:"amount"`
}

// assign allocates assets to account IDs in the chaincode state for each of the
// account ID passed in.
// accountID: account ID to be allocated with requested amount
// contactInfo: contact information of the owner of the account ID passed in
// amount: amount to be allocated to this account ID
func (t *depositoryHandler) assign(stub shim.ChaincodeStubInterface,
	accountID string,
	contactInfo string,
	amount uint64) error {

	myLogger.Debugf("insert accountID= %v", accountID)

	// you can only assign balances to new account IDs
	accountBytes, err := stub.GetState(accountID)
	if err == nil && len(accountBytes) > 0 {
		myLogger.Errorf("system error %v", err)
		return errors.New("Asset was already assigned.")
	}

	account := depositoryAccount{
		AccountID:   accountID,
		ContactInfo: contactInfo,
		Amount:      amount,
	}
	accountBytes, err = json.Marshal(account)
	if err != nil {
		myLogger.Errorf("account marshaling error %v", err)
		return errors.New("Failed to serialize account info." + err.Error())
	}

	//update this account that includes contact information and balance
	err = stub.PutState(accountID, accountBytes)
	return err
}

// updateAccountBalance updates the balance amount of an account ID
// stub: chaincodestub
// accountID: account will be updated with the new balance
// contactInfo: contact information associated with the account owner (chaincode table does not allow me to perform updates on specific columns)
// amount: new amount to be udpated with
func (t *depositoryHandler) updateAccountBalance(stub shim.ChaincodeStubInterface,
	accountID string,
	contactInfo string,
	amount uint64) error {

	myLogger.Debugf("insert accountID= %v", accountID)

	//replace the old record row associated with the account ID with the new record row
	account := depositoryAccount{
		AccountID:   accountID,
		ContactInfo: contactInfo,
		Amount:      amount,
	}
	accountBytes, err := json.Marshal(account)
	if err != nil {
		myLogger.Errorf("account marshaling error %v", err)
		return errors.New("Failed to serialize account info." + err.Error())
	}

	//update this account that includes contact information and balance
	err = stub.PutState(accountID, accountBytes)
	return err
}

// deleteAccountRecord deletes the record row associated with an account ID on the chaincode state table
// stub: chaincodestub
// accountID: account ID (record matching this account ID will be deleted after calling this method)
func (t *depositoryHandler) deleteAccountRecord(stub shim.ChaincodeStubInterface, accountID string) error {

	myLogger.Debugf("insert accountID= %v", accountID)

	//delete record matching account ID passed in
	err := stub.DelState(accountID)

	if err != nil {
		myLogger.Errorf("system error %v", err)
		return errors.New("error in deleting account record")
	}
	return nil
}

// transfer transfers X amount of assets from "from account IDs" to a new account ID
// stub: chaincodestub
// fromAccounts: from account IDs with assets to be transferred
// toAccount: a new account ID on the table that will get assets transfered to
// toContact: contact information of the owner of "to account ID"
func (t *depositoryHandler) transfer(stub shim.ChaincodeStubInterface, fromAccounts []string, toAccount string, toContact string, amount uint64) error {

	myLogger.Debugf("insert params= %v , %v , %v , %v ", fromAccounts, toAccount, toContact, amount)

	//collecting assets need to be transfered
	remaining := amount
	for i := range fromAccounts {
		contactInfo, acctBalance, err := t.queryAccount(stub, fromAccounts[i])
		if err != nil {
			myLogger.Errorf("system error %v", err)
			return errors.New("error in deleting account record")
		}

		if remaining > 0 {
			//check if this account need to be spent entirely; if so, delete the
			//account record row, otherwise just take out what' needed
			if remaining >= acctBalance {
				remaining -= acctBalance
				//delete accounts with 0 balance, this step is optional
				t.deleteAccountRecord(stub, fromAccounts[i])
			} else {
				acctBalance -= remaining
				remaining = 0
				t.updateAccountBalance(stub, fromAccounts[i], contactInfo, acctBalance)
				break
			}
		}
	}

	//check if toAccount already exist
	acctBalance, err := t.queryBalance(stub, toAccount)
	if err == nil || acctBalance > 0 {
		myLogger.Errorf("system error %v", err)
		return errors.New("error in deleting account record")
	}

	//create new toAccount in the Chaincode state table, and assign the total amount
	//to its balance
	return t.assign(stub, toAccount, toContact, amount)

}

// queryContactInfo queries the contact information matching a correponding account ID on the chaincode state table
// stub: chaincodestub
// accountID: account ID
func (t *depositoryHandler) queryContactInfo(stub shim.ChaincodeStubInterface, accountID string) (string, error) {
	account, err := t.queryTable(stub, accountID)
	if err != nil {
		return "", err
	}

	return account.ContactInfo, nil
}

// queryBalance queries the balance information matching a correponding account ID on the chaincode state table
// stub: chaincodestub
// accountID: account ID
func (t *depositoryHandler) queryBalance(stub shim.ChaincodeStubInterface, accountID string) (uint64, error) {

	myLogger.Debugf("insert accountID= %v", accountID)

	account, err := t.queryTable(stub, accountID)
	if err != nil {
		return 0, err
	}

	return account.Amount, nil
}

// queryAccount queries the balance and contact information matching a correponding account ID on the chaincode state table
// stub: chaincodestub
// accountID: account ID
func (t *depositoryHandler) queryAccount(stub shim.ChaincodeStubInterface, accountID string) (string, uint64, error) {
	account, err := t.queryTable(stub, accountID)
	if err != nil {
		return "", 0, err
	}

	return account.ContactInfo, account.Amount, nil
}

// queryTable returns the record row matching a correponding account ID on the chaincode state table
// stub: chaincodestub
// accountID: account ID
func (t *depositoryHandler) queryTable(stub shim.ChaincodeStubInterface, accountID string) (*depositoryAccount, error) {

	accountBytes, err := stub.GetState(accountID)
	if err != nil {
		return nil, errors.New("Failed to get account." + err.Error())
	}
	if len(accountBytes) == 0 {
		return nil, fmt.Errorf("Account %s not exists.", accountID)
	}

	account := &depositoryAccount{}
	err = json.Unmarshal(accountBytes, account)
	if err != nil {
		return nil, errors.New("Failed to parse account Info. " + err.Error())
	}
	return account, nil
}
