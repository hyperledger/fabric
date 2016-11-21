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

// Package cscc (configuration system chaincode) provides functions to manage
// configuration transactions as the network is being reconfigured. The
// configuration transactions arrive from the ordering service to the committer
// who calls this chaincode. The chaincode also provides peer configuration
// services such as joining a chain or getting configuration data.
package cscc

import (
	"errors"
	"fmt"

	//	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

// PeerConfiger implements the configuration handler for the peer. For every
// configuration transaction coming in from the ordering service, the
// committer calls this system chaincode to process the transaction.
type PeerConfiger struct {
}

var logger = logging.MustGetLogger("cscc")

// These are function names from Invoke first parameter
const (
	JoinChain         string = "JoinChain"
	UpdateConfigBlock string = "UpdateConfigBlock"
	GetConfigBlock    string = "GetConfigBlock"
)

// Init is called once per chain when the chain is created.
// This allows the chaincode to initialize any variables on the ledger prior
// to any transaction execution on the chain.
func (e *PeerConfiger) Init(stub shim.ChaincodeStubInterface) ([]byte, error) {
	logger.Info("Init CSCC")

	return nil, nil
}

// Invoke is called for the following:
// # to process joining a chain (called by app as a transaction proposal)
// # to get the current configuration block (called by app)
// # to update the configuration block (called by commmitter)
// Peer calls this function with 2 arguments:
// # args[0] is the function name, which must be JoinChain, GetConfigBlock or
// UpdateConfigBlock
// # args[1] is a configuration Block if args[0] is JoinChain or
// UpdateConfigBlock; otherwise it is the chain id
// TODO: Improve the scc interface to avoid marshal/unmarshal args
func (e *PeerConfiger) Invoke(stub shim.ChaincodeStubInterface) ([]byte, error) {
	args := stub.GetArgs()

	if len(args) < 2 {
		return nil, fmt.Errorf("Incorrect number of arguments, %d", len(args))
	}
	fname := string(args[0])

	logger.Debugf("Invoke function: %s", fname)

	// TODO: Handle ACL

	if fname == JoinChain {
		return joinChain(args[1])
	} else if fname == GetConfigBlock {
		return getConfigBlock(args[1])
	} else if fname == UpdateConfigBlock {
		return updateConfigBlock(args[1])
	}

	return nil, fmt.Errorf("Requested function %s not found.", fname)
}

// joinChain will join the specified chain in the configuration block.
// Since it is the first block, it is the genesis block containing configuration
// for this chain, so we want to update the Chain object with this info
func joinChain(blockBytes []byte) ([]byte, error) {
	if blockBytes == nil {
		return nil, fmt.Errorf("Genesis block must not be nil.")
	}

	block, err := utils.GetBlockFromBlockBytes(blockBytes)
	if err != nil {
		return nil, fmt.Errorf("Failed to reconstruct the genesis block, %s", err)
	}

	if err := peer.CreateChainFromBlock(block); err != nil {
		return nil, err
	}

	return nil, nil
}

func updateConfigBlock(blockBytes []byte) ([]byte, error) {
	if blockBytes == nil {
		return nil, errors.New("Configuration block must not be nil.")
	}
	block, err := utils.GetBlockFromBlockBytes(blockBytes)
	if err != nil {
		return nil, fmt.Errorf("Failed to reconstruct the configuration block, %s", err)
	}
	chainID, err := utils.GetChainIDFromBlock(block)
	if err != nil {
		return nil, fmt.Errorf("Failed to get the chain ID from the configuration block, %s", err)
	}

	if err := peer.SetCurrConfigBlock(block, chainID); err != nil {
		return nil, err
	}

	// TODO: would committer get ledger and update ?

	return nil, nil
}

// Return the current configuration block for the specified chainID. If the
// peer doesn't belong to the chain, return error
func getConfigBlock(chainID []byte) ([]byte, error) {
	if chainID == nil {
		return nil, errors.New("ChainID must not be nil.")
	}
	block := peer.GetCurrConfigBlock(string(chainID))
	if block == nil {
		return nil, fmt.Errorf("Unknown chain ID, %s", string(chainID))
	}
	blockBytes, err := utils.Marshal(block)
	if err != nil {
		return nil, err
	}

	return blockBytes, nil
}
