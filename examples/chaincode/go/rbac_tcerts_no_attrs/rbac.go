/*
 Copyright IBM Corp All Rights Reserved.

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
	"fmt"

	"encoding/asn1"
	"encoding/base64"
	"strings"

	"github.com/hyperledger/fabric/accesscontrol/impl"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/op/go-logging"
)

var myLogger = logging.MustGetLogger("rbac_chaincode")

// RBACMetadata metadata structure for RBAC information
type RBACMetadata struct {
	Cert  []byte
	Sigma []byte
}

// RBACChaincode RBAC chaincode structure
type RBACChaincode struct {
}

// Init method will be called during deployment
func (t *RBACChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	myLogger.Info("Init")

	myLogger.Debug("Creating RBAC Table...")

	// Create RBAC table
	err := stub.CreateTable("RBAC", []*shim.ColumnDefinition{
		&shim.ColumnDefinition{Name: "ID", Type: shim.ColumnDefinition_BYTES, Key: true},
		&shim.ColumnDefinition{Name: "Roles", Type: shim.ColumnDefinition_STRING, Key: false},
	})
	if err != nil {
		return shim.Error("Failed creating RBAC table.")
	}

	myLogger.Debug("Assign 'admin' role...")

	// Give to the deployer the role 'admin'
	deployer, err := stub.GetCallerMetadata()
	if err != nil {
		return shim.Error("Failed getting metadata.")
	}
	if len(deployer) == 0 {
		return shim.Error("Invalid admin certificate. Empty.")
	}

	myLogger.Debug("Add admin [% x][%s]", deployer, "admin")
	ok, err := stub.InsertRow("RBAC", shim.Row{
		Columns: []*shim.Column{
			&shim.Column{Value: &shim.Column_Bytes{Bytes: deployer}},
			&shim.Column{Value: &shim.Column_String_{String_: "admin"}},
		},
	})
	if !ok && err == nil {
		return shim.Error("Failed initiliazing RBAC entries.")
	}
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed initiliazing RBAC entries [%s]", err))
	}

	myLogger.Debug("Done.")

	return shim.Success(nil)
}

// Invoke Run callback representing the invocation of a chaincode
func (t *RBACChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	// Handle different functions
	switch function {
	case "addRole":
		return t.addRole(stub, args)
	case "write":
		return t.write(stub, args)
	case "read":
		return t.read(stub, args)
	}

	return shim.Error(fmt.Sprintf("Received unknown function invocation [%s]", function))
}

func (t *RBACChaincode) addRole(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	if len(args) != 2 {
		return shim.Error("Incorrect number of arguments. Expecting 2")
	}
	id, err := base64.StdEncoding.DecodeString(args[0])
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed decoding tcert:  %s", err))
	}
	//id := []byte(args[0])
	role := args[1]

	myLogger.Debug("Add role [% x][%s]", id, role)

	// Verify that the invoker has the 'admin' role
	ok, _, err := t.hasInvokerRole(stub, "admin")
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed checking role [%s]", err))
	}
	if !ok {
		return shim.Error("The invoker does not have the required roles.")
	}

	// Add role to id
	myLogger.Debug("Permission granted to the invoker")

	// Retrieve id's row
	var columns []shim.Column
	idCol := shim.Column{Value: &shim.Column_Bytes{Bytes: id}}
	columns = append(columns, idCol)
	row, err := stub.GetRow("RBAC", columns)
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed retriving associated row [%s]", err))
	}
	if len(row.Columns) == 0 {
		// Insert row
		ok, err = stub.InsertRow("RBAC", shim.Row{
			Columns: []*shim.Column{
				&shim.Column{Value: &shim.Column_Bytes{Bytes: id}},
				&shim.Column{Value: &shim.Column_String_{String_: role}},
			},
		})
		if err != nil {
			return shim.Error(fmt.Sprintf("Failed inserting row [%s]", err))
		}
		if !ok {
			return shim.Error("Failed inserting row.")
		}

	} else {
		// Update row
		ok, err = stub.ReplaceRow("RBAC", shim.Row{
			Columns: []*shim.Column{
				&shim.Column{Value: &shim.Column_Bytes{Bytes: id}},
				&shim.Column{Value: &shim.Column_String_{String_: row.Columns[1].GetString_() + " " + role}},
			},
		})
		if err != nil {
			return shim.Error(fmt.Sprintf("Failed replacing row [%s]", err))
		}
		if !ok {
			return shim.Error("Failed replacing row.")
		}
	}

	return shim.Success(nil)
}

func (t *RBACChaincode) read(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	if len(args) != 0 {
		return shim.Error("Incorrect number of arguments. Expecting 0")
	}
	myLogger.Debug("Read...")

	// Verify that the invoker has the 'reader' role
	ok, _, err := t.hasInvokerRole(stub, "reader")
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed checking role [%s]", err))
	}
	if !ok {
		return shim.Error("The invoker does not have the required roles")
	}

	res, err := stub.GetState("state")
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed getting state [%s]", err))
	}

	myLogger.Debug("State [%s]", string(res))

	return shim.Success(res)
}

func (t *RBACChaincode) write(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting 1")
	}

	value := args[0]

	myLogger.Debug("Write [%s]", value)

	// Verify that the invoker has the 'writer' role
	ok, _, err := t.hasInvokerRole(stub, "writer")
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed checking role [%s]", err))
	}
	if !ok {
		return shim.Error("The invoker does not have the required roles.")
	}

	err = stub.PutState("state", []byte(value))
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

func (t *RBACChaincode) hasInvokerRole(stub shim.ChaincodeStubInterface, role string) (bool, []byte, error) {
	// In order to enforce access control, we require that the
	// metadata contains the following items:
	// 1. a certificate Cert
	// 2. a signature Sigma under the signing key corresponding
	// to the verification key inside Cert of :
	// (a) Cert;
	// (b) The payload of the transaction (namely, function name and args) and
	// (c) the transaction binding.

	// Verify Sigma=Sign(certificate.sk, Cert||tx.Payload||tx.Binding) against Cert.vk

	// Unmarshall metadata
	metadata, err := stub.GetCallerMetadata()
	rbacMetadata := new(RBACMetadata)
	_, err = asn1.Unmarshal(metadata, rbacMetadata)
	if err != nil {
		return false, nil, fmt.Errorf("Failed unmarshalling metadata [%s]", err)
	}

	// Verify signature
	payload, err := stub.GetPayload()
	if err != nil {
		return false, nil, errors.New("Failed getting payload")
	}
	binding, err := stub.GetBinding()
	if err != nil {
		return false, nil, errors.New("Failed getting binding")
	}

	myLogger.Debug("passed certificate [% x]", rbacMetadata.Cert)
	myLogger.Debug("passed sigma [% x]", rbacMetadata.Sigma)
	myLogger.Debug("passed payload [% x]", payload)
	myLogger.Debug("passed binding [% x]", binding)

	ok, err := impl.NewAccessControlShim(stub).VerifySignature(
		rbacMetadata.Cert,
		rbacMetadata.Sigma,
		append(rbacMetadata.Cert, append(payload, binding...)...),
	)
	if err != nil {
		return false, nil, fmt.Errorf("Failed verifying signature [%s]", err)
	}
	if !ok {
		return false, nil, fmt.Errorf("Signature is not valid!")
	}

	myLogger.Debug("Signature verified. Check for role...")

	myLogger.Debug("ID [% x]", rbacMetadata.Cert)

	// Check role
	var columns []shim.Column
	idCol := shim.Column{Value: &shim.Column_Bytes{Bytes: rbacMetadata.Cert}}
	columns = append(columns, idCol)
	row, err := stub.GetRow("RBAC", columns)
	if err != nil {
		return false, nil, fmt.Errorf("Failed retrieveing RBAC row [%s]", err)
	}
	if len(row.GetColumns()) == 0 {
		return false, nil, fmt.Errorf("Failed retrieveing RBAC row [%s]", err)
	}

	myLogger.Debug("#Columns: [%d]", len(row.Columns))
	roles := row.Columns[1].GetString_()

	result := strings.Split(roles, " ")
	for i := range result {
		if result[i] == role {
			myLogger.Debug("Role found.")
			return true, rbacMetadata.Cert, nil
		}
	}

	myLogger.Debug("Role not found.")

	return false, nil, nil
}

func main() {
	err := shim.Start(new(RBACChaincode))
	if err != nil {
		fmt.Printf("Error starting AssetManagementChaincode: %s", err)
	}
}
