/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chainmgmt

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt/ledgermgmttest"
)

// ChainID is a type used for the ids for the chains for experiments
type ChainID int

func (chainID ChainID) String() string {
	return fmt.Sprintf("chain-%04d", chainID)
}

// SimulationResult is a type used for simulation results
type SimulationResult []byte

// chainsMgr manages chains for experiments
type chainsMgr struct {
	ledgerMgr *ledgermgmt.LedgerMgr
	mgrConf   *ChainMgrConf
	batchConf *BatchConf
	initOp    chainInitOp
	chainsMap map[ChainID]*Chain
	wg        *sync.WaitGroup
}

func newChainsMgr(mgrConf *ChainMgrConf, batchConf *BatchConf, initOp chainInitOp) *chainsMgr {
	dataDir := filepath.Join(mgrConf.DataDir, "ledgersData")
	ledgermgmtInitializer := ledgermgmttest.NewInitializer(dataDir)
	ledgermgmtInitializer.Config.HistoryDBConfig.Enabled = true
	if os.Getenv("useCouchDB") == "true" {
		couchdbAddr, set := os.LookupEnv("COUCHDB_ADDR")
		if !set {
			panic("environment variable 'useCouchDB' is set to true but 'COUCHDB_ADDR' is not set")
		}
		ledgermgmtInitializer.Config.StateDBConfig.StateDatabase = ledger.CouchDB
		ledgermgmtInitializer.Config.StateDBConfig.CouchDB = &ledger.CouchDBConfig{
			Address:            couchdbAddr,
			RedoLogPath:        filepath.Join(dataDir, "couchdbRedologs"),
			UserCacheSizeMBs:   500,
			MaxBatchUpdateSize: 500,
		}
	}
	ledgerMgr := ledgermgmt.NewLedgerMgr(ledgermgmtInitializer)
	return &chainsMgr{ledgerMgr, mgrConf, batchConf, initOp, make(map[ChainID]*Chain), &sync.WaitGroup{}}
}

func (m *chainsMgr) createOrOpenChains() []*Chain {
	numChains := m.mgrConf.NumChains
	switch m.initOp {
	case ChainInitOpCreate:
		for i := 0; i < numChains; i++ {
			chainID := ChainID(i)
			ledgerID := chainID.String()
			gb, err := test.MakeGenesisBlock(ledgerID)
			panicOnError(err)
			peerLedger, err := m.ledgerMgr.CreateLedger(ledgerID, gb)
			panicOnError(err)
			c := newChain(chainID, peerLedger, m)
			m.chainsMap[chainID] = c
		}

	case ChainInitOpOpen:
		for i := 0; i < numChains; i++ {
			chainID := ChainID(i)
			peerLedger, err := m.ledgerMgr.OpenLedger(chainID.String())
			panicOnError(err)
			c := newChain(chainID, peerLedger, m)
			m.chainsMap[chainID] = c
		}

	default:
		panic(fmt.Errorf("unknown chain init opeartion"))
	}
	return m.chains()
}

func (m *chainsMgr) chains() []*Chain {
	chains := []*Chain{}
	for _, chain := range m.chainsMap {
		chains = append(chains, chain)
	}
	return chains
}

func (m *chainsMgr) waitForChainsToExhaustAllBlocks() {
	m.wg.Wait()
	m.ledgerMgr.Close()
}

// Chain embeds ledger.PeerLedger and the experiments invoke ledger functions via a chain type
type Chain struct {
	ledger.PeerLedger
	ID           ChainID
	blkGenerator *blkGenerator
	m            *chainsMgr
}

func newChain(id ChainID, peerLedger ledger.PeerLedger, m *chainsMgr) *Chain {
	bcInfo, err := peerLedger.GetBlockchainInfo()
	panicOnError(err)
	return &Chain{peerLedger, id, newBlkGenerator(m.batchConf, bcInfo.Height, bcInfo.CurrentBlockHash), m}
}

func (c *Chain) startBlockPollingAndCommit() {
	c.m.wg.Add(1)
	go func() {
		defer c.close()
		for {
			block := c.blkGenerator.nextBlock()
			if block == nil {
				break
			}
			panicOnError(c.PeerLedger.CommitLegacy(
				&ledger.BlockAndPvtData{Block: block},
				&ledger.CommitOptions{},
			))
		}
	}()
}

// SubmitTx is expected to be called by an experiment for submitting the transactions
func (c *Chain) SubmitTx(sr SimulationResult) {
	c.blkGenerator.addTx(sr)
}

// Done is expected to be called by an experiment when the experiment does not have any more transactions to submit
func (c *Chain) Done() {
	c.blkGenerator.close()
}

// Commit overrides the Commit function in ledger.PeerLedger because,
// experiments are not expected to call Commit directly to the ledger
func (c *Chain) Commit(block *common.Block) {
	panic(fmt.Errorf("Commit should not be invoked directly"))
}

func (c *Chain) close() {
	c.PeerLedger.Close()
	c.m.wg.Done()
}
