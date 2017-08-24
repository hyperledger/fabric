/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatatxmgr

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr/lockbasedtxmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

var logger = flogging.MustGetLogger("pvtdatatxmgr")

// TransientHandlerTxMgr wraps a specific txmgr implementation (such as lockbasedtxmgr)
// and adds the additional functionality of persisting the private writesets into transient store
// Txmgr does this job temporarily for phase-1 and will be moved out to endorser
type TransientHandlerTxMgr struct {
	txmgr.TxMgr
}

// NewLockbasedTxMgr constructs a new instance of TransientHandlerTxMgr
func NewLockbasedTxMgr(db privacyenabledstate.DB) *TransientHandlerTxMgr {
	return &TransientHandlerTxMgr{lockbasedtxmgr.NewLockBasedTxMgr(db)}
}

// NewTxSimulator extends the implementation of this function in the wrapped txmgr.
func (w *TransientHandlerTxMgr) NewTxSimulator(txid string) (ledger.TxSimulator, error) {
	var ht *version.Height
	var err error
	var actualTxSim ledger.TxSimulator
	var simBlkHt uint64

	if ht, err = w.TxMgr.GetLastSavepoint(); err != nil {
		return nil, err
	}

	if ht != nil {
		simBlkHt = ht.BlockNum
	}

	if actualTxSim, err = w.TxMgr.NewTxSimulator(txid); err != nil {
		return nil, err
	}
	return newSimulatorWrapper(actualTxSim, txid, simBlkHt), nil
}

// transientHandlerTxSimulator wraps a txsimulator and adds the additional functionality of persisting
// the private writesets into transient store
type transientHandlerTxSimulator struct {
	ledger.TxSimulator
	txid     string
	simBlkHt uint64
}

func newSimulatorWrapper(actualSim ledger.TxSimulator, txid string, simBlkHt uint64) *transientHandlerTxSimulator {
	return &transientHandlerTxSimulator{actualSim, txid, simBlkHt}
}

func (w *transientHandlerTxSimulator) GetTxSimulationResults() (*ledger.TxSimulationResults, error) {
	return w.TxSimulator.GetTxSimulationResults()
}
