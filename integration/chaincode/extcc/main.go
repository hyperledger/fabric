/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

var cert string = `-----BEGIN CERTIFICATE-----
MIICJzCCAc6gAwIBAgIRAOG3lHMXyOfMnnwuhmFadacwCgYIKoZIzj0EAwIwWDEL
MAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNhbiBG
cmFuY2lzY28xDTALBgNVBAoTBE9yZzExDTALBgNVBAMTBE9yZzEwHhcNMTgwODIx
MDgyNTMzWhcNMjgwODE4MDgyNTMzWjBlMQswCQYDVQQGEwJVUzETMBEGA1UECBMK
Q2FsaWZvcm5pYTEWMBQGA1UEBxMNU2FuIEZyYW5jaXNjbzEVMBMGA1UEChMMT3Jn
MS1zZXJ2ZXIxMRIwEAYDVQQDEwlsb2NhbGhvc3QwWTATBgcqhkjOPQIBBggqhkjO
PQMBBwNCAAR3TOK0+NiNmc4ykv9SnVcVv47SAuyvU57477JxknlRPepEDZTM2hRp
QMDCQ5Xhlj3hzWJZ7pwBlwgsBHLKFZYNo2wwajAOBgNVHQ8BAf8EBAMCBaAwHQYD
VR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAwDwYDVR0j
BAgwBoAEAQIDBDAaBgNVHREEEzARgglsb2NhbGhvc3SHBH8AAAEwCgYIKoZIzj0E
AwIDRwAwRAIgcGXiuBaP5eam8d3qR6+R8Jc2KuzqxrmI5OvkYl0O4WoCIGhTz0TK
OKsG+IMU9QXw0+4aHVvI78yHC3/L+w+lAGg8
-----END CERTIFICATE-----
`

var key string = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIAkkp8rbeZ7rFXOFai94Bhnnli2ziX4eJ2jDMX0urCAxoAoGCCqGSM49
AwEHoUQDQgAEd0zitPjYjZnOMpL/Up1XFb+O0gLsr1Oe+O+ycZJ5UT3qRA2UzNoU
aUDAwkOV4ZY94c1iWe6cAZcILARyyhWWDQ==
-----END EC PRIVATE KEY-----
`

var clientcert string = `-----BEGIN CERTIFICATE-----
MIIB8TCCAZegAwIBAgIQUigdJy6IudO7sVOXsKVrtzAKBggqhkjOPQQDAjBYMQsw
CQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMNU2FuIEZy
YW5jaXNjbzENMAsGA1UEChMET3JnMTENMAsGA1UEAxMET3JnMTAeFw0xODA4MjEw
ODI1MzNaFw0yODA4MTgwODI1MzNaMFgxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpD
YWxpZm9ybmlhMRYwFAYDVQQHEw1TYW4gRnJhbmNpc2NvMQ0wCwYDVQQKEwRPcmcx
MQ0wCwYDVQQDEwRPcmcxMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEVOI+oAAB
Pl+iRsCcGq81WbXap2L1r432T5gbzUNKYRvVsyFFYmdO8ql8uDi4UxSY64eaeRFT
uxdcsTG7M5K2yaNDMEEwDgYDVR0PAQH/BAQDAgGmMA8GA1UdJQQIMAYGBFUdJQAw
DwYDVR0TAQH/BAUwAwEB/zANBgNVHQ4EBgQEAQIDBDAKBggqhkjOPQQDAgNIADBF
AiEA6U7IRGf+S7e9U2+jSI2eFiBsVEBIi35LgYoKqjELj5oCIAD7DfVMaMHzzjiQ
XIlJQdS/9afDi32qZWZfe3kAUAs0
-----END CERTIFICATE-----
`

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct{}

func (t *SimpleChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	fmt.Println("Init invoked")
	_, args := stub.GetFunctionAndParameters()
	var A, B string    // Entities
	var Aval, Bval int // Asset holdings
	var err error

	if len(args) != 4 {
		return shim.Error("Incorrect number of arguments. Expecting 4")
	}

	// Initialize the chaincode
	A = args[0]
	Aval, err = strconv.Atoi(args[1])
	if err != nil {
		return shim.Error("Expecting integer value for asset holding")
	}
	B = args[2]
	Bval, err = strconv.Atoi(args[3])
	if err != nil {
		return shim.Error("Expecting integer value for asset holding")
	}
	fmt.Printf("Aval = %d, Bval = %d\n", Aval, Bval)

	// Write the state to the ledger
	err = stub.PutState(A, []byte(strconv.Itoa(Aval)))
	if err != nil {
		return shim.Error(err.Error())
	}

	err = stub.PutState(B, []byte(strconv.Itoa(Bval)))
	if err != nil {
		return shim.Error(err.Error())
	}

	fmt.Println("Init returning with success")
	return shim.Success(nil)
}

func (t *SimpleChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	fmt.Println("ex02 Invoke")
	function, args := stub.GetFunctionAndParameters()
	switch function {
	case "invoke":
		// Make payment of X units from A to B
		return t.invoke(stub, args)
	case "delete":
		// Deletes an entity from its state
		return t.delete(stub, args)
	case "query":
		// the old "Query" is now implemtned in invoke
		return t.query(stub, args)
	case "respond":
		// return with an error
		return t.respond(stub, args)
	default:
		return shim.Error(`Invalid invoke function name. Expecting "invoke", "delete", "query", or "respond"`)
	}
}

// Transaction makes payment of X units from A to B
func (t *SimpleChaincode) invoke(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var A, B string    // Entities
	var Aval, Bval int // Asset holdings
	var X int          // Transaction value
	var err error

	if len(args) != 3 {
		return shim.Error("Incorrect number of arguments. Expecting 3")
	}

	A = args[0]
	B = args[1]

	// Get the state from the ledger
	// TODO: will be nice to have a GetAllState call to ledger
	Avalbytes, err := stub.GetState(A)
	if err != nil {
		return shim.Error("Failed to get state")
	}
	if Avalbytes == nil {
		return shim.Error("Entity not found")
	}
	Aval, _ = strconv.Atoi(string(Avalbytes))

	Bvalbytes, err := stub.GetState(B)
	if err != nil {
		return shim.Error("Failed to get state")
	}
	if Bvalbytes == nil {
		return shim.Error("Entity not found")
	}
	Bval, _ = strconv.Atoi(string(Bvalbytes))

	// Perform the execution
	X, err = strconv.Atoi(args[2])
	if err != nil {
		return shim.Error("Invalid transaction amount, expecting a integer value")
	}
	Aval = Aval - X
	Bval = Bval + X
	fmt.Printf("Aval = %d, Bval = %d\n", Aval, Bval)

	// Write the state back to the ledger
	err = stub.PutState(A, []byte(strconv.Itoa(Aval)))
	if err != nil {
		return shim.Error(err.Error())
	}

	err = stub.PutState(B, []byte(strconv.Itoa(Bval)))
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

// Deletes an entity from state
func (t *SimpleChaincode) delete(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting 1")
	}

	A := args[0]

	// Delete the key from the state in ledger
	err := stub.DelState(A)
	if err != nil {
		return shim.Error("Failed to delete state")
	}

	return shim.Success(nil)
}

// query callback representing the query of a chaincode
func (t *SimpleChaincode) query(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var A string // Entities
	var err error

	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting name of the person to query")
	}

	A = args[0]

	// Get the state from the ledger
	Avalbytes, err := stub.GetState(A)
	if err != nil {
		jsonResp := "{\"Error\":\"Failed to get state for " + A + "\"}"
		return shim.Error(jsonResp)
	}

	if Avalbytes == nil {
		jsonResp := "{\"Error\":\"Nil amount for " + A + "\"}"
		return shim.Error(jsonResp)
	}

	jsonResp := "{\"Name\":\"" + A + "\",\"Amount\":\"" + string(Avalbytes) + "\"}"
	fmt.Printf("Query Response:%s\n", jsonResp)
	return shim.Success(Avalbytes)
}

// respond simply generates a response payload from the args
func (t *SimpleChaincode) respond(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	if len(args) != 3 {
		return shim.Error("expected three arguments")
	}

	status, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		return shim.Error(err.Error())
	}
	message := args[1]
	payload := []byte(args[2])

	return pb.Response{
		Status:  int32(status),
		Message: message,
		Payload: payload,
	}
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: <package-id> <listener-address>")
		os.Exit(1)
	}

	server := &shim.ChaincodeServer{
		CCID:    os.Args[1],
		Address: os.Args[2],
		CC:      new(SimpleChaincode),
		TLSProps: shim.TLSProperties{
			Key:           []byte(key),
			Cert:          []byte(cert),
			ClientCACerts: []byte(clientcert),
		},
	}
	// do not modify - needed for integration test
	fmt.Printf("Starting chaincode %s at %s\n", server.CCID, server.Address)
	err := server.Start()
	if err != nil {
		fmt.Printf("Error starting Simple chaincode: %s", err)
	}
}
