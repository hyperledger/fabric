/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package experiments

import (
	"flag"

	"github.com/hyperledger/fabric/core/ledger/kvledger/benchmark/chainmgmt"
)

// txConf captures the transaction related configurations
// numTotalTxs specifies the total transactions that should be executed and committed across chains
// numParallelTxsPerChain specifies the parallel transactions on each of the chains
// numWritesPerTx specifies the number of keys to write in each transaction
// numReadsPerTx specifies the number of keys to read in each transaction, Note:  this parameters
//
//	match the numWritesPerTx for normal benchmarks.  This can be set to zero to make batch update measurements.
type txConf struct {
	numTotalTxs            int
	numParallelTxsPerChain int
	numWritesPerTx         int
	numReadsPerTx          int
}

// dataConf captures the data related configurations
// numKVs specifies number of total key-values across chains
// kvSize specifies the size of a key-value (in bytes)
// useJSON specifies if the value stored is in JSON format
type dataConf struct {
	numKVs  int
	kvSize  int
	useJSON bool
}

// configuration captures all the configurations for an experiment
// For details of individual configuration, see comments on the specific type
type configuration struct {
	chainMgrConf *chainmgmt.ChainMgrConf
	batchConf    *chainmgmt.BatchConf
	dataConf     *dataConf
	txConf       *txConf
}

// emptyConf returns a an empty configuration (with nested structure only)
func emptyConf() *configuration {
	conf := &configuration{}
	conf.chainMgrConf = &chainmgmt.ChainMgrConf{}
	conf.batchConf = &chainmgmt.BatchConf{}
	conf.txConf = &txConf{}
	conf.dataConf = &dataConf{}
	return conf
}

// confFromTestParams consumes the parameters passed by an experiment
// and returns the configuration loaded with the parsed param values
func confFromTestParams(testParams []string) *configuration {
	conf := emptyConf()
	flags := flag.NewFlagSet("testParams", flag.ExitOnError)

	// chainMgrConf
	dataDir := flags.String("DataDir", conf.chainMgrConf.DataDir, "Dir for ledger data")
	numChains := flags.Int("NumChains", conf.chainMgrConf.NumChains, "Number of chains")

	// txConf
	numParallelTxsPerChain := flags.Int("NumParallelTxPerChain",
		conf.txConf.numParallelTxsPerChain, "Number of TxSimulators concurrently on each chain")

	numTotalTxs := flags.Int("NumTotalTx",
		conf.txConf.numTotalTxs, "Number of total transactions")

	numWritesPerTx := flags.Int("NumWritesPerTx",
		conf.txConf.numWritesPerTx, "number of keys written in each Tx")

	numReadsPerTx := flags.Int("NumReadsPerTx",
		conf.txConf.numReadsPerTx, "number of keys to read in each Tx")

	// batchConf
	batchSize := flags.Int("BatchSize",
		conf.batchConf.BatchSize, "number of Txs in each batch")

	// dataConf
	numKVs := flags.Int("NumKVs",
		conf.dataConf.numKVs, "the keys are named as key_0, key_1,... upto key_(NumKVs-1)")

	kvSize := flags.Int("KVSize",
		conf.dataConf.kvSize, "size of the key-value in bytes")

	useJSON := flags.Bool("UseJSONFormat", conf.dataConf.useJSON, "should CouchDB use JSON for values")

	flags.Parse(testParams)

	conf.chainMgrConf.DataDir = *dataDir
	conf.chainMgrConf.NumChains = *numChains
	conf.txConf.numParallelTxsPerChain = *numParallelTxsPerChain
	conf.txConf.numTotalTxs = *numTotalTxs
	conf.txConf.numWritesPerTx = *numWritesPerTx
	conf.txConf.numReadsPerTx = *numReadsPerTx
	conf.batchConf.BatchSize = *batchSize
	conf.dataConf.numKVs = *numKVs
	conf.dataConf.kvSize = *kvSize
	conf.dataConf.useJSON = *useJSON
	return conf
}
