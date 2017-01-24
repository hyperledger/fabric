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
	"bytes"
	"encoding/base64"
	"fmt"

	"github.com/hyperledger/fabric/accesscontrol/crypto/attr"
	"github.com/hyperledger/fabric/accesscontrol/impl"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/op/go-logging"
)

var myLogger = logging.MustGetLogger("asset_mgm")

// AssetManagementChaincode example simple Asset Management Chaincode implementation
// with access control enforcement at chaincode level.
//
// This example implements asset transfer using attributes support and specifically Attribute Based Access Control (ABAC).
// There are three users in this example:
// - alice
// - bob
// - admin
//
// This users are defined in the section "eca" of asset.yaml file.
// In the section "aca" of asset.yaml file two attributes are defined to this users:
// The first attribute is called 'role' with this values:
// - alice has role = client
// - bob has role = client
// - admin has role = assigner
//
// The second attribute is called 'account' with this values:
// - alice has account = 12345-56789
// - bob has account = 23456-67890
//
// In the present example only users with role 'assigner' can associate an 'asset' as is implemented in function 'assign' and
// user with role 'client' can transfers theirs assets to other clients as is implemented in function 'transfer'.
// Asset ownership is stored in the ledger state and is linked to the client account.
// Attribute 'account' is used to associate transaction certificates with account owner.
type AssetManagementChaincode struct {
}

// Init initialization
func (t *AssetManagementChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	_, args := stub.GetFunctionAndParameters()
	myLogger.Info("[AssetManagementChaincode] Init")
	if len(args) != 0 {
		return shim.Error("Incorrect number of arguments. Expecting 0")
	}

	// Create ownership table
	err := stub.CreateTable("AssetsOwnership", []*shim.ColumnDefinition{
		&shim.ColumnDefinition{Name: "Asset", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "Owner", Type: shim.ColumnDefinition_BYTES, Key: false},
	})
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed creating AssetsOnwership table, [%v]", err))
	}

	// Set the role of the users that are allowed to assign assets
	// The metadata will contain the role of the users that are allowed to assign assets
	assignerRole, err := stub.GetCallerMetadata()
	fmt.Printf("Assiger role is %v\n", string(assignerRole))

	if err != nil {
		return shim.Error(fmt.Sprintf("Failed getting metadata, [%v]", err))
	}

	if len(assignerRole) == 0 {
		return shim.Error("Invalid assigner role. Empty.")
	}

	stub.PutState("assignerRole", assignerRole)

	return shim.Success(nil)
}

func (t *AssetManagementChaincode) assign(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	fmt.Println("Assigning Asset...")

	if len(args) != 2 {
		return shim.Error("Incorrect number of arguments. Expecting 2")
	}

	asset := args[0]
	owner, err := base64.StdEncoding.DecodeString(args[1])
	if err != nil {
		fmt.Printf("Error decoding [%v] \n", err)
		return shim.Error("Failed decodinf owner")
	}

	// Recover the role that is allowed to make assignments
	assignerRole, err := stub.GetState("assignerRole")
	if err != nil {
		fmt.Printf("Error getting role [%v] \n", err)
		return shim.Error("Failed fetching assigner role")
	}

	callerRole, err := impl.NewAccessControlShim(stub).ReadCertAttribute("role")
	if err != nil {
		fmt.Printf("Error reading attribute 'role' [%v] \n", err)
		return shim.Error(fmt.Sprintf("Failed fetching caller role. Error was [%v]", err))
	}

	caller := string(callerRole[:])
	assigner := string(assignerRole[:])

	if caller != assigner {
		fmt.Printf("Caller is not assigner - caller %v assigner %v\n", caller, assigner)
		return shim.Error(fmt.Sprintf("The caller does not have the rights to invoke assign. Expected role [%v], caller role [%v]", assigner, caller))
	}

	account, err := attr.GetValueFrom("account", owner)
	if err != nil {
		fmt.Printf("Error reading account [%v] \n", err)
		return shim.Error(fmt.Sprintf("Failed fetching recipient account. Error was [%v]", err))
	}

	// Register assignment
	myLogger.Debugf("New owner of [%s] is [% x]", asset, owner)

	ok, err := stub.InsertRow("AssetsOwnership", shim.Row{
		Columns: []*shim.Column{
			&shim.Column{Value: &shim.Column_String_{String_: asset}},
			&shim.Column{Value: &shim.Column_Bytes{Bytes: account}}},
	})

	if !ok && err == nil {
		fmt.Println("Error inserting row")
		return shim.Error("Asset was already assigned.")
	}

	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

func (t *AssetManagementChaincode) transfer(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	if len(args) != 2 {
		return shim.Error("Incorrect number of arguments. Expecting 2")
	}

	asset := args[0]

	newOwner, err := base64.StdEncoding.DecodeString(args[1])
	if err != nil {
		fmt.Printf("Error decoding [%v] \n", err)
		return shim.Error("Failed decoding owner")
	}

	// Verify the identity of the caller
	// Only the owner can transfer one of his assets
	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: asset}}
	columns = append(columns, col1)

	row, err := stub.GetRow("AssetsOwnership", columns)
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed retrieving asset [%s]: [%s]", asset, err))
	}

	prvOwner := row.Columns[1].GetBytes()
	myLogger.Debugf("Previous owener of [%s] is [% x]", asset, prvOwner)
	if len(prvOwner) == 0 {
		return shim.Error("Invalid previous owner. Nil")
	}

	// Verify ownership
	callerAccount, err := impl.NewAccessControlShim(stub).ReadCertAttribute("account")
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed fetching caller account. Error was [%v]", err))
	}

	if bytes.Compare(prvOwner, callerAccount) != 0 {
		return shim.Error("Failed verifying caller ownership.")
	}

	newOwnerAccount, err := attr.GetValueFrom("account", newOwner)
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed fetching new owner account. Error was [%v]", err))
	}

	// At this point, the proof of ownership is valid, then register transfer
	err = stub.DeleteRow(
		"AssetsOwnership",
		[]shim.Column{shim.Column{Value: &shim.Column_String_{String_: asset}}},
	)
	if err != nil {
		return shim.Error("Failed deliting row.")
	}

	_, err = stub.InsertRow(
		"AssetsOwnership",
		shim.Row{
			Columns: []*shim.Column{
				&shim.Column{Value: &shim.Column_String_{String_: asset}},
				&shim.Column{Value: &shim.Column_Bytes{Bytes: newOwnerAccount}},
			},
		})
	if err != nil {
		return shim.Error("Failed inserting row.")
	}

	return shim.Success(nil)
}

// Invoke runs callback representing the invocation of a chaincode
func (t *AssetManagementChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	// Handle different functions
	if function == "assign" {
		// Assign ownership
		return t.assign(stub, args)
	} else if function == "transfer" {
		// Transfer ownership
		return t.transfer(stub, args)
	} else if function == "query" {
		// Query asset
		return t.query(stub, args)
	}

	return shim.Error("Received unknown function invocation")
}

func (t *AssetManagementChaincode) query(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var err error

	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting name of an asset to query")
	}

	// Who is the owner of the asset?
	asset := args[0]

	fmt.Printf("ASSET: %v", string(asset))

	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: asset}}
	columns = append(columns, col1)

	row, err := stub.GetRow("AssetsOwnership", columns)
	if err != nil {
		jsonResp := "{\"Error\":\"Failed retrieving asset " + asset + ". Error " + err.Error() + ". \"}"
		return shim.Error(jsonResp)
	}

	if len(row.Columns) == 0 {
		jsonResp := "{\"Error\":\"Failed retrieving owner for " + asset + ". \"}"
		return shim.Error(jsonResp)
	}

	jsonResp := "{\"Owner\":\"" + string(row.Columns[1].GetBytes()) + "\"}"
	fmt.Printf("Query Response:%s\n", jsonResp)

	return shim.Success(row.Columns[1].GetBytes())
}

func main() {
	err := shim.Start(new(AssetManagementChaincode))
	if err != nil {
		fmt.Printf("Error starting AssetManagementChaincode: %s", err)
	}
}
