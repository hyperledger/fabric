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
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/accesscontrol/impl"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/op/go-logging"
)

var myLogger = logging.MustGetLogger("asset_mgm")

// AssetManagementChaincode is simple chaincode implementing a basic Asset Management system
// with access control enforcement at chaincode level.
// Look here for more information on how to implement access control at chaincode level:
// https://github.com/hyperledger/fabric/blob/master/docs/tech/application-ACL.md
// An asset is simply represented by a string.
type AssetManagementChaincode struct {
}

// Init method will be called during deployment.
// The deploy transaction metadata is supposed to contain the administrator cert
func (t *AssetManagementChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	_, args := stub.GetFunctionAndParameters()
	myLogger.Debug("Init Chaincode...")
	if len(args) != 0 {
		return shim.Error("Incorrect number of arguments. Expecting 0")
	}

	// Create ownership table
	err := stub.CreateTable("AssetsOwnership", []*shim.ColumnDefinition{
		&shim.ColumnDefinition{Name: "Asset", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "Owner", Type: shim.ColumnDefinition_BYTES, Key: false},
	})
	if err != nil {
		return shim.Error("Failed creating AssetsOnwership table.")
	}

	// Set the admin
	// The metadata will contain the certificate of the administrator
	adminCert, err := stub.GetCallerMetadata()
	if err != nil {
		myLogger.Debug("Failed getting metadata")
		return shim.Error("Failed getting metadata.")
	}
	if len(adminCert) == 0 {
		myLogger.Debug("Invalid admin certificate. Empty.")
		return shim.Error("Invalid admin certificate. Empty.")
	}

	myLogger.Debug("The administrator is [%x]", adminCert)

	stub.PutState("admin", adminCert)

	myLogger.Debug("Init Chaincode...done")

	return shim.Success(nil)
}

func (t *AssetManagementChaincode) assign(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	myLogger.Debug("Assign...")

	if len(args) != 2 {
		return shim.Error("Incorrect number of arguments. Expecting 2")
	}

	asset := args[0]
	owner, err := base64.StdEncoding.DecodeString(args[1])
	if err != nil {
		return shim.Error("Failed decodinf owner")
	}

	// Verify the identity of the caller
	// Only an administrator can invoker assign
	adminCertificate, err := stub.GetState("admin")
	if err != nil {
		return shim.Error("Failed fetching admin identity")
	}

	ok, err := t.isCaller(stub, adminCertificate)
	if err != nil {
		return shim.Error("Failed checking admin identity")
	}
	if !ok {
		return shim.Error("The caller is not an administrator")
	}

	// Register assignment
	myLogger.Debugf("New owner of [%s] is [% x]", asset, owner)

	ok, err = stub.InsertRow("AssetsOwnership", shim.Row{
		Columns: []*shim.Column{
			&shim.Column{Value: &shim.Column_String_{String_: asset}},
			&shim.Column{Value: &shim.Column_Bytes{Bytes: owner}}},
	})

	if !ok && err == nil {
		return shim.Error("Asset was already assigned.")
	}

	if err != nil {
		return shim.Error(err.Error())
	}

	myLogger.Debug("Assign...done!")

	return shim.Success(nil)
}

func (t *AssetManagementChaincode) transfer(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	myLogger.Debug("Transfer...")

	if len(args) != 2 {
		return shim.Error("Incorrect number of arguments. Expecting 2")
	}

	asset := args[0]
	newOwner, err := base64.StdEncoding.DecodeString(args[1])
	if err != nil {
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
	ok, err := t.isCaller(stub, prvOwner)
	if err != nil {
		return shim.Error("Failed checking asset owner identity")
	}
	if !ok {
		return shim.Error("The caller is not the owner of the asset")
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
				&shim.Column{Value: &shim.Column_Bytes{Bytes: newOwner}},
			},
		})
	if err != nil {
		return shim.Error("Failed inserting row.")
	}

	myLogger.Debug("New owner of [%s] is [% x]", asset, newOwner)

	myLogger.Debug("Transfer...done")

	return shim.Success(nil)
}

func (t *AssetManagementChaincode) isCaller(stub shim.ChaincodeStubInterface, certificate []byte) (bool, error) {
	myLogger.Debug("Check caller...")

	// In order to enforce access control, we require that the
	// metadata contains the signature under the signing key corresponding
	// to the verification key inside certificate of
	// the payload of the transaction (namely, function name and args) and
	// the transaction binding (to avoid copying attacks)

	// Verify \sigma=Sign(certificate.sk, tx.Payload||tx.Binding) against certificate.vk
	// \sigma is in the metadata

	sigma, err := stub.GetCallerMetadata()
	if err != nil {
		return false, errors.New("Failed getting metadata")
	}
	payload, err := stub.GetPayload()
	if err != nil {
		return false, errors.New("Failed getting payload")
	}
	binding, err := stub.GetBinding()
	if err != nil {
		return false, errors.New("Failed getting binding")
	}

	myLogger.Debugf("passed certificate [% x]", certificate)
	myLogger.Debugf("passed sigma [% x]", sigma)
	myLogger.Debugf("passed payload [% x]", payload)
	myLogger.Debugf("passed binding [% x]", binding)

	ok, err := impl.NewAccessControlShim(stub).VerifySignature(
		certificate,
		sigma,
		append(payload, binding...),
	)
	if err != nil {
		myLogger.Errorf("Failed checking signature [%s]", err)
		return ok, err
	}
	if !ok {
		myLogger.Error("Invalid signature")
	}

	myLogger.Debug("Check caller...Verified!")

	return ok, err
}

// Invoke will be called for every transaction.
// Supported functions are the following:
// "assign(asset, owner)": to assign ownership of assets. An asset can be owned by a single entity.
// Only an administrator can call this function.
// "transfer(asset, newOwner)": to transfer the ownership of an asset. Only the owner of the specific
// asset can call this function.
// An asset is any string to identify it. An owner is representated by one of his ECert/TCert.
func (t *AssetManagementChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	// Handle different functions
	if function == "assign" {
		// Assign ownership
		return t.assign(stub, args)
	} else if function == "transfer" {
		// Transfer ownership
		return t.transfer(stub, args)
	}

	return shim.Error("Received unknown function invocation")
}

// Query callback representing the query of a chaincode
// Supported functions are the following:
// "query(asset)": returns the owner of the asset.
// Anyone can invoke this function.
func (t *AssetManagementChaincode) Query(stub shim.ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	myLogger.Debugf("Query [%s]", function)

	if function != "query" {
		return shim.Error("Invalid query function name. Expecting 'query' but found '" + function + "'")
	}

	var err error

	if len(args) != 1 {
		myLogger.Debug("Incorrect number of arguments. Expecting name of an asset to query")
		return shim.Error("Incorrect number of arguments. Expecting name of an asset to query")
	}

	// Who is the owner of the asset?
	asset := args[0]

	myLogger.Debugf("Arg [%s]", string(asset))

	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: asset}}
	columns = append(columns, col1)

	row, err := stub.GetRow("AssetsOwnership", columns)
	if err != nil {
		myLogger.Debugf("Failed retriving asset [%s]: [%s]", string(asset), err)
		return shim.Error(fmt.Sprintf("Failed retriving asset [%s]: [%s]", string(asset), err))
	}

	myLogger.Debugf("Query done [% x]", row.Columns[1].GetBytes())

	return shim.Success(row.Columns[1].GetBytes())
}

func main() {
	primitives.SetSecurityLevel("SHA3", 256)
	err := shim.Start(new(AssetManagementChaincode))
	if err != nil {
		fmt.Printf("Error starting AssetManagementChaincode: %s", err)
	}
}
