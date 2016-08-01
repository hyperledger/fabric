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

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

// consts associated with chaincode table
const (
	tableColumn       = "AssetsOwnership"
	columnAccountID   = "Account"
	columnContactInfo = "ContactInfo"
	columnAmount      = "Amount"
)

//DepositoryHandler provides APIs used to perform operations on CC's KV store
type depositoryHandler struct {
}

// NewDepositoryHandler create a new reference to CertHandler
func NewDepositoryHandler() *depositoryHandler {
	return &depositoryHandler{}
}

// createTable initiates a new asset depository table in the chaincode state
// stub: chaincodestub
func (t *depositoryHandler) createTable(stub shim.ChaincodeStubInterface) error {

	// Create asset depository table
	return stub.CreateTable(tableColumn, []*shim.ColumnDefinition{
		&shim.ColumnDefinition{Name: columnAccountID, Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: columnContactInfo, Type: shim.ColumnDefinition_STRING, Key: false},
		&shim.ColumnDefinition{Name: columnAmount, Type: shim.ColumnDefinition_UINT64, Key: false},
	})

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

	//insert a new row for this account ID that includes contact information and balance
	ok, err := stub.InsertRow(tableColumn, shim.Row{
		Columns: []*shim.Column{
			&shim.Column{Value: &shim.Column_String_{String_: accountID}},
			&shim.Column{Value: &shim.Column_String_{String_: contactInfo}},
			&shim.Column{Value: &shim.Column_Uint64{Uint64: amount}}},
	})

	// you can only assign balances to new account IDs
	if !ok && err == nil {
		myLogger.Errorf("system error %v", err)
		return errors.New("Asset was already assigned.")
	}

	return nil
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
	ok, err := stub.ReplaceRow(tableColumn, shim.Row{
		Columns: []*shim.Column{
			&shim.Column{Value: &shim.Column_String_{String_: accountID}},
			&shim.Column{Value: &shim.Column_String_{String_: contactInfo}},
			&shim.Column{Value: &shim.Column_Uint64{Uint64: amount}}},
	})

	if !ok && err == nil {
		myLogger.Errorf("system error %v", err)
		return errors.New("failed to replace row with account Id." + accountID)
	}
	return nil
}

// deleteAccountRecord deletes the record row associated with an account ID on the chaincode state table
// stub: chaincodestub
// accountID: account ID (record matching this account ID will be deleted after calling this method)
func (t *depositoryHandler) deleteAccountRecord(stub shim.ChaincodeStubInterface, accountID string) error {

	myLogger.Debugf("insert accountID= %v", accountID)

	//delete record matching account ID passed in
	err := stub.DeleteRow(
		"AssetsOwnership",
		[]shim.Column{shim.Column{Value: &shim.Column_String_{String_: accountID}}},
	)

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
	row, err := t.queryTable(stub, accountID)
	if err != nil {
		return "", err
	}

	return row.Columns[1].GetString_(), nil
}

// queryBalance queries the balance information matching a correponding account ID on the chaincode state table
// stub: chaincodestub
// accountID: account ID
func (t *depositoryHandler) queryBalance(stub shim.ChaincodeStubInterface, accountID string) (uint64, error) {

	myLogger.Debugf("insert accountID= %v", accountID)

	row, err := t.queryTable(stub, accountID)
	if err != nil {
		return 0, err
	}
	if len(row.Columns) == 0 || row.Columns[2] == nil {
		return 0, errors.New("row or column value not found")
	}

	return row.Columns[2].GetUint64(), nil
}

// queryAccount queries the balance and contact information matching a correponding account ID on the chaincode state table
// stub: chaincodestub
// accountID: account ID
func (t *depositoryHandler) queryAccount(stub shim.ChaincodeStubInterface, accountID string) (string, uint64, error) {
	row, err := t.queryTable(stub, accountID)
	if err != nil {
		return "", 0, err
	}
	if len(row.Columns) == 0 || row.Columns[2] == nil {
		return "", 0, errors.New("row or column value not found")
	}

	return row.Columns[1].GetString_(), row.Columns[2].GetUint64(), nil
}

// queryTable returns the record row matching a correponding account ID on the chaincode state table
// stub: chaincodestub
// accountID: account ID
func (t *depositoryHandler) queryTable(stub shim.ChaincodeStubInterface, accountID string) (shim.Row, error) {

	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: accountID}}
	columns = append(columns, col1)

	return stub.GetRow(tableColumn, columns)
}
