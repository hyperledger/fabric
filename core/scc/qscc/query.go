/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package qscc

import (
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric/common/flogging"

	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

// New returns an instance of QSCC.
// Typically this is called once per peer.
func New(aclProvider aclmgmt.ACLProvider) *LedgerQuerier {
	return &LedgerQuerier{
		aclProvider: aclProvider,
	}
}

func (e *LedgerQuerier) Name() string              { return "qscc" }
func (e *LedgerQuerier) Path() string              { return "github.com/hyperledger/fabric/core/scc/qscc" }
func (e *LedgerQuerier) InitArgs() [][]byte        { return nil }
func (e *LedgerQuerier) Chaincode() shim.Chaincode { return e }
func (e *LedgerQuerier) InvokableExternal() bool   { return true }
func (e *LedgerQuerier) InvokableCC2CC() bool      { return true }
func (e *LedgerQuerier) Enabled() bool             { return true }

// LedgerQuerier implements the ledger query functions, including:
// - GetChainInfo returns BlockchainInfo
// - GetBlockByNumber returns a block
// - GetBlockByHash returns a block
// - GetTransactionByID returns a transaction
type LedgerQuerier struct {
	aclProvider aclmgmt.ACLProvider
}

var qscclogger = flogging.MustGetLogger("qscc")

// These are function names from Invoke first parameter
const (
	GetChainInfo       string = "GetChainInfo"
	GetBlockByNumber   string = "GetBlockByNumber"
	GetBlockByHash     string = "GetBlockByHash"
	GetTransactionByID string = "GetTransactionByID"
	GetBlockByTxID     string = "GetBlockByTxID"
)

// Init is called once per chain when the chain is created.
// This allows the chaincode to initialize any variables on the ledger prior
// to any transaction execution on the chain.
func (e *LedgerQuerier) Init(stub shim.ChaincodeStubInterface) pb.Response {
	qscclogger.Info("Init QSCC")

	return shim.Success(nil)
}

// Invoke is called with args[0] contains the query function name, args[1]
// contains the chain ID, which is temporary for now until it is part of stub.
// Each function requires additional parameters as described below:
// # GetChainInfo: Return a BlockchainInfo object marshalled in bytes
// # GetBlockByNumber: Return the block specified by block number in args[2]
// # GetBlockByHash: Return the block specified by block hash in args[2]
// # GetTransactionByID: Return the transaction specified by ID in args[2]
func (e *LedgerQuerier) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()

	if len(args) < 2 {
		return shim.Error(fmt.Sprintf("Incorrect number of arguments, %d", len(args)))
	}
	fname := string(args[0])
	cid := string(args[1])

	if fname != GetChainInfo && len(args) < 3 {
		return shim.Error(fmt.Sprintf("missing 3rd argument for %s", fname))
	}

	targetLedger := peer.GetLedger(cid)
	if targetLedger == nil {
		return shim.Error(fmt.Sprintf("Invalid chain ID, %s", cid))
	}

	qscclogger.Debugf("Invoke function: %s on chain: %s", fname, cid)

	// Handle ACL:
	// 1. get the signed proposal
	sp, err := stub.GetSignedProposal()
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed getting signed proposal from stub, %s: %s", cid, err))
	}

	// 2. check the channel reader policy
	res := getACLResource(fname)
	if err = e.aclProvider.CheckACL(res, cid, sp); err != nil {
		return shim.Error(fmt.Sprintf("access denied for [%s][%s]: [%s]", fname, cid, err))
	}

	switch fname {
	case GetTransactionByID:
		return getTransactionByID(targetLedger, args[2])
	case GetBlockByNumber:
		return getBlockByNumber(targetLedger, args[2])
	case GetBlockByHash:
		return getBlockByHash(targetLedger, args[2])
	case GetChainInfo:
		return getChainInfo(targetLedger)
	case GetBlockByTxID:
		return getBlockByTxID(targetLedger, args[2])
	}

	return shim.Error(fmt.Sprintf("Requested function %s not found.", fname))
}

func getTransactionByID(vledger ledger.PeerLedger, tid []byte) pb.Response {
	if tid == nil {
		return shim.Error("Transaction ID must not be nil.")
	}

	processedTran, err := vledger.GetTransactionByID(string(tid))
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed to get transaction with id %s, error %s", string(tid), err))
	}

	bytes, err := utils.Marshal(processedTran)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(bytes)
}

func getBlockByNumber(vledger ledger.PeerLedger, number []byte) pb.Response {
	if number == nil {
		return shim.Error("Block number must not be nil.")
	}
	bnum, err := strconv.ParseUint(string(number), 10, 64)
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed to parse block number with error %s", err))
	}
	block, err := vledger.GetBlockByNumber(bnum)
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed to get block number %d, error %s", bnum, err))
	}
	// TODO: consider trim block content before returning
	//  Specifically, trim transaction 'data' out of the transaction array Payloads
	//  This will preserve the transaction Payload header,
	//  and client can do GetTransactionByID() if they want the full transaction details

	bytes, err := utils.Marshal(block)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(bytes)
}

func getBlockByHash(vledger ledger.PeerLedger, hash []byte) pb.Response {
	if hash == nil {
		return shim.Error("Block hash must not be nil.")
	}
	block, err := vledger.GetBlockByHash(hash)
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed to get block hash %s, error %s", string(hash), err))
	}
	// TODO: consider trim block content before returning
	//  Specifically, trim transaction 'data' out of the transaction array Payloads
	//  This will preserve the transaction Payload header,
	//  and client can do GetTransactionByID() if they want the full transaction details

	bytes, err := utils.Marshal(block)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(bytes)
}

func getChainInfo(vledger ledger.PeerLedger) pb.Response {
	binfo, err := vledger.GetBlockchainInfo()
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed to get block info with error %s", err))
	}
	bytes, err := utils.Marshal(binfo)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(bytes)
}

func getBlockByTxID(vledger ledger.PeerLedger, rawTxID []byte) pb.Response {
	txID := string(rawTxID)
	block, err := vledger.GetBlockByTxID(txID)

	if err != nil {
		return shim.Error(fmt.Sprintf("Failed to get block for txID %s, error %s", txID, err))
	}

	bytes, err := utils.Marshal(block)

	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(bytes)
}

func getACLResource(fname string) string {
	return "qscc/" + fname
}
