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

package chainmgmt

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/protos/common"
	benchcommon "github.com/hyperledger/fabric/test/tools/LTE/common"
)

// ChainID is a type used for the ids for the chains for experiments
type ChainID int

func (chainID ChainID) String() string {
	return fmt.Sprintf("%s%04d", "chain_", chainID)
}

// SimulationResult is a type used for simulation results
type SimulationResult []byte

// chainsMgr manages chains for experiments
type chainsMgr struct {
	mgrConf   *ChainMgrConf
	batchConf *BatchConf
	initOp    chainInitOp
	chainsMap map[ChainID]*Chain
	wg        *sync.WaitGroup
}

func newChainsMgr(mgrConf *ChainMgrConf, batchConf *BatchConf, initOp chainInitOp) *chainsMgr {
	ledgermgmt.Initialize()
	return &chainsMgr{mgrConf, batchConf, initOp, make(map[ChainID]*Chain), &sync.WaitGroup{}}
}

func (m *chainsMgr) createOrOpenChains() []*Chain {
	var ledgerInitFunc func(string) (ledger.PeerLedger, error)
	switch m.initOp {
	case ChainInitOpCreate:
		ledgerInitFunc = createLedgerByID
	case ChainInitOpOpen:
		ledgerInitFunc = ledgermgmt.OpenLedger
	default:
		panic(fmt.Errorf("unknown chain init opeartion"))
	}

	numChains := m.mgrConf.NumChains
	for i := 0; i < numChains; i++ {
		chainID := ChainID(i)
		peerLedger, err := ledgerInitFunc(chainID.String())
		benchcommon.PanicOnError(err)
		c := newChain(chainID, peerLedger, m)
		m.chainsMap[chainID] = c
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
	ledgermgmt.Close()
}

// Chain extends ledger.PeerLedger and the experiments invoke ledger functions via a chain type
type Chain struct {
	ledger.PeerLedger
	ID           ChainID
	blkGenerator *blkGenerator
	m            *chainsMgr
}

func newChain(id ChainID, peerLedger ledger.PeerLedger, m *chainsMgr) *Chain {
	bcInfo, err := peerLedger.GetBlockchainInfo()
	benchcommon.PanicOnError(err)
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
			benchcommon.PanicOnError(c.PeerLedger.Commit(block))
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

func createLedgerByID(ledgerid string) (ledger.PeerLedger, error) {
	gb, err := test.MakeGenesisBlock(ledgerid)
	benchcommon.PanicOnError(err)
	return ledgermgmt.CreateLedger(gb)
}
