/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package chaincode

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/op/go-logging"
	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

// LedgerQuerier implements the ledger query functions, including:
// - GetChainInfo returns BlockchainInfo
// - GetBlockByNumber returns a block
// - GetBlockByHash returns a block
// - GetTransactionByID returns a transaction
// - GetQueryResult returns result of a freeform query
type LedgerQuerier struct {
}

var qscclogger = logging.MustGetLogger("qscc")

// These are function names from Invoke first parameter
const (
	GetChainInfo       string = "GetChainInfo"
	GetBlockByNumber   string = "GetBlockByNumber"
	GetBlockByHash     string = "GetBlockByHash"
	GetTransactionByID string = "GetTransactionByID"
	GetQueryResult     string = "GetQueryResult"
)

// Init is called once per chain when the chain is created.
// This allows the chaincode to initialize any variables on the ledger prior
// to any transaction execution on the chain.
func (e *LedgerQuerier) Init(stub shim.ChaincodeStubInterface) ([]byte, error) {
	qscclogger.Info("Init QSCC")

	return nil, nil
}

// Invoke is called with args[0] contains the query function name, args[1]
// contains the chain ID, which is temporary for now until it is part of stub.
// Each function requires additional parameters as described below:
// # GetChainInfo: Return a BlockchainInfo object marshalled in bytes
// # GetBlockByNumber: Return the block specified by block number in args[2]
// # GetBlockByHash: Return the block specified by block hash in args[2]
// # GetTransactionByID: Return the transaction specified by ID in args[2]
// # GetQueryResult: Return the result of executing the specified native
// query string in args[2]. Note that this only works if plugged in database
// supports it. The result is a JSON array in a byte array. Note that error
// may be returned together with a valid partial result as error might occur
// during accummulating records from the ledger
func (e *LedgerQuerier) Invoke(stub shim.ChaincodeStubInterface) ([]byte, error) {
	args := stub.GetArgs()

	if len(args) < 2 {
		return nil, fmt.Errorf("Incorrect number of arguments, %d", len(args))
	}
	fname := string(args[0])
	cid := string(args[1])

	if fname != GetChainInfo && len(args) < 3 {
		return nil, fmt.Errorf("missing 3rd argument for %s", fname)
	}

	targetLedger := peer.GetLedger(cid)
	if targetLedger == nil {
		return nil, fmt.Errorf("Invalid chain ID, %s", cid)
	}
	if qscclogger.IsEnabledFor(logging.DEBUG) {
		qscclogger.Debugf("Invoke function: %s on chain: %s", fname, cid)
	}

	// TODO: Handle ACL

	switch fname {
	case GetQueryResult:
		return getQueryResult(targetLedger, args[2])
	case GetTransactionByID:
		return getTransactionByID(targetLedger, args[2])
	case GetBlockByNumber:
		return getBlockByNumber(targetLedger, args[2])
	case GetBlockByHash:
		return getBlockByHash(targetLedger, args[2])
	case GetChainInfo:
		return getChainInfo(targetLedger)
	}

	return nil, fmt.Errorf("Requested function %s not found.", fname)
}

// Execute the specified query string
func getQueryResult(vledger ledger.PeerLedger, query []byte) (ret []byte, err error) {
	if query == nil {
		return nil, fmt.Errorf("Query string must not be nil.")
	}
	qstring := string(query)
	var qexe ledger.QueryExecutor
	var ri ledger.ResultsIterator

	// We install a recover() to gain control in 2 cases
	// 1) bytes.Buffer panics, which happens when out of memory
	// This is a safety measure beyond the config limit variable
	// 2) plugin db driver might panic
	// We recover by stopping the query and return the panic error.
	defer func() {
		if panicValue := recover(); panicValue != nil {
			if qscclogger.IsEnabledFor(logging.DEBUG) {
				qscclogger.Debugf("Recovering panic: %s", panicValue)
			}
			err = fmt.Errorf("Error recovery: %s", panicValue)
		}
	}()

	if qexe, err = vledger.NewQueryExecutor(); err != nil {
		return nil, err
	}
	if ri, err = qexe.ExecuteQuery(qstring); err != nil {
		return nil, err
	}
	defer ri.Close()

	limit := viper.GetInt("ledger.state.couchDBConfig.queryLimit")

	// buffer is a JSON array containing QueryRecords
	var buffer bytes.Buffer
	buffer.WriteString("[")

	var qresult ledger.QueryResult
	qresult, err = ri.Next()
	for r := 0; qresult != nil && err == nil && r < limit; r++ {
		if qr, ok := qresult.(*ledger.QueryRecord); ok {
			collectRecord(&buffer, qr)
		}
		qresult, err = ri.Next()
	}

	buffer.WriteString("]")

	// Return what we have accummulated
	ret = buffer.Bytes()
	return ret, err
}

// Append QueryRecord into buffer as a JSON record of the form {namespace, key, record}
// type QueryRecord struct {
// 	Namespace string
// 	Key       string
// 	Record    []byte
// }
func collectRecord(buffer *bytes.Buffer, rec *ledger.QueryRecord) {
	buffer.WriteString("{\"Namespace\":")
	buffer.WriteString("\"")
	buffer.WriteString(rec.Namespace)
	buffer.WriteString("\"")

	buffer.WriteString(", \"Key\":")
	buffer.WriteString("\"")
	buffer.WriteString(rec.Key)
	buffer.WriteString("\"")

	buffer.WriteString(", \"Record\":")
	// Record is a JSON object, so we write as-is
	buffer.WriteString(string(rec.Record))
	buffer.WriteString("}")
}

func getTransactionByID(vledger ledger.PeerLedger, tid []byte) ([]byte, error) {
	if tid == nil {
		return nil, fmt.Errorf("Transaction ID must not be nil.")
	}
	tx, err := vledger.GetTransactionByID(string(tid))
	if err != nil {
		return nil, fmt.Errorf("Failed to get transaction with id %s, error %s", string(tid), err)
	}
	// TODO: tx is *pb.Transaction, what should we return?

	return utils.Marshal(tx)
}

func getBlockByNumber(vledger ledger.PeerLedger, number []byte) ([]byte, error) {
	if number == nil {
		return nil, fmt.Errorf("Block number must not be nil.")
	}
	bnum, err := strconv.ParseUint(string(number), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse block number with error %s", err)
	}
	block, err := vledger.GetBlockByNumber(bnum)
	if err != nil {
		return nil, fmt.Errorf("Failed to get block number %d, error %s", bnum, err)
	}
	// TODO: consider trim block content before returning

	return utils.Marshal(block)
}

func getBlockByHash(vledger ledger.PeerLedger, hash []byte) ([]byte, error) {
	if hash == nil {
		return nil, fmt.Errorf("Block hash must not be nil.")
	}
	block, err := vledger.GetBlockByHash(hash)
	if err != nil {
		return nil, fmt.Errorf("Failed to get block hash %s, error %s", string(hash), err)
	}
	// TODO: consider trim block content before returning

	return utils.Marshal(block)
}

func getChainInfo(vledger ledger.PeerLedger) ([]byte, error) {
	binfo, err := vledger.GetBlockchainInfo()
	if err != nil {
		return nil, fmt.Errorf("Failed to get block info with error %s", err)
	}
	return utils.Marshal(binfo)
}
