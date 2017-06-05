/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chainmgmt

import "github.com/spf13/viper"

// chainInitOp is a type that an experiment uses to specify how the chains
// should be initialized at the beginning of the experiment. See below the
// enum values for this type
type chainInitOp uint8

const (
	// ChainInitOpCreate indicates that the chains should be creates afresh
	ChainInitOpCreate chainInitOp = iota + 1
	// ChainInitOpOpen indicates that the existing chains should be opened
	ChainInitOpOpen
)

// TestEnv is a high level struct that the experiments are expeted to use as a starting point.
// See one of the Benchmark tests for the intended usage
type TestEnv struct {
	mgr *chainsMgr
}

// InitTestEnv initialize TestEnv with given configurations. The initialization cuases
// creation (or openning of existing) chains and the block creation and commit go routines
// for each of the chains. For configurations options, see comments on specific configuration type
func InitTestEnv(mgrConf *ChainMgrConf, batchConf *BatchConf, initOperation chainInitOp) *TestEnv {
	viper.Set("peer.fileSystemPath", mgrConf.DataDir)
	mgr := newChainsMgr(mgrConf, batchConf, initOperation)
	chains := mgr.createOrOpenChains()
	for _, chain := range chains {
		chain.startBlockPollingAndCommit()
	}
	return &TestEnv{mgr}
}

// Chains returns handle to all the chains
func (env TestEnv) Chains() []*Chain {
	return env.mgr.chains()
}

// WaitForTestCompletion waits till all the transactions are committed
// An experiment after launching all the goroutine should call this
// so that the process is alive till all the goroutines complete
func (env TestEnv) WaitForTestCompletion() {
	env.mgr.waitForChainsToExhaustAllBlocks()
}
