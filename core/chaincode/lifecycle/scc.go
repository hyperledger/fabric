/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/pkg/errors"
)

const (
	//InstalledChaincodeFuncName is the chaincode function name used to install a chaincode
	InstallChaincodeFuncName = "InstallChaincode"

	// QueryInstalledChaincodeFuncName is the chaincode function name used to query an installed chaincode
	QueryInstalledChaincodeFuncName = "QueryInstalledChaincode"
)

// SCCFunctions provides a backing implementation with concrete arguments
// for each of the SCC functions
type SCCFunctions interface {
	// InstallChaincode persists a chaincode definition to disk
	InstallChaincode(name, version string, chaincodePackage []byte) (hash []byte, err error)

	// QueryInstalledChaincode returns the hash for a given name and version of an installed chaincode
	QueryInstalledChaincode(name, version string) (hash []byte, err error)
}

// SCC implements the required methods to satisfy the chaincode interface.
// It routes the invocation calls to the backing implementations.
type SCC struct {
	Protobuf  Protobuf
	Functions SCCFunctions
}

// Name returns "+lifecycle"
func (scc *SCC) Name() string {
	return "+lifecycle"
}

// Path returns "github.com/hyperledger/fabric/core/chaincode/lifecycle"
func (scc *SCC) Path() string {
	return "github.com/hyperledger/fabric/core/chaincode/lifecycle"
}

// InitArgs returns nil
func (scc *SCC) InitArgs() [][]byte {
	return nil
}

// Chaincode returns a reference to itself
func (scc *SCC) Chaincode() shim.Chaincode {
	return scc
}

// InvokableExternal returns true
func (scc *SCC) InvokableExternal() bool {
	return true
}

// InvokableCC2CC returns true
func (scc *SCC) InvokableCC2CC() bool {
	return true
}

// Enabled returns true
func (scc *SCC) Enabled() bool {
	return true
}

// Init is mostly useless for system chaincodes and always returns success
func (scc *SCC) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Success(nil)
}

// Invoke takes chaincode invocation arguments and routes them to the correct
// underlying lifecycle operation.  All functions take a single argument of
// type marshaled lb.<FunctionName>Args and return a marshaled lb.<FunctionName>Result
func (scc *SCC) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()
	if len(args) == 0 {
		return shim.Error("lifecycle scc must be invoked with arguments")
	}

	if len(args) != 2 {
		return shim.Error(fmt.Sprintf("lifecycle scc operations require exactly two arguments but received %d", len(args)))
	}

	funcName := args[0]
	inputBytes := args[1]

	// TODO add ACLs

	switch string(funcName) {
	// Each lifecycle SCC function gets a case here
	case InstallChaincodeFuncName:
		input := &lb.InstallChaincodeArgs{}
		err := scc.Protobuf.Unmarshal(inputBytes, input)
		if err != nil {
			err = errors.WithMessage(err, "failed to decode input arg to InstallChaincode")
			return shim.Error(err.Error())
		}

		hash, err := scc.Functions.InstallChaincode(input.Name, input.Version, input.ChaincodeInstallPackage)
		if err != nil {
			err = errors.WithMessage(err, "failed to invoke backing InstallChaincode")
			return shim.Error(err.Error())
		}

		resultBytes, err := scc.Protobuf.Marshal(&lb.InstallChaincodeResult{
			Hash: hash,
		})
		if err != nil {
			err = errors.WithMessage(err, "failed to marshal result")
			return shim.Error(err.Error())
		}

		return shim.Success(resultBytes)
	case QueryInstalledChaincodeFuncName:
		input := &lb.QueryInstalledChaincodeArgs{}
		err := scc.Protobuf.Unmarshal(inputBytes, input)
		if err != nil {
			err = errors.WithMessage(err, "failed to decode input arg to QueryInstalledChaincode")
			return shim.Error(err.Error())
		}

		hash, err := scc.Functions.QueryInstalledChaincode(input.Name, input.Version)
		if err != nil {
			err = errors.WithMessage(err, "failed to invoke backing QueryInstalledChaincode")
			return shim.Error(err.Error())
		}

		resultBytes, err := scc.Protobuf.Marshal(&lb.QueryInstalledChaincodeResult{
			Hash: hash,
		})
		if err != nil {
			err = errors.WithMessage(err, "failed to marshal result")
			return shim.Error(err.Error())
		}

		return shim.Success(resultBytes)
	default:
		return shim.Error(fmt.Sprintf("unknown lifecycle function: %s", funcName))
	}
}
