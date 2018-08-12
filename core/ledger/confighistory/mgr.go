/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package confighistory

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("confighistory")

const (
	lsccNamespace = "lscc"
)

// Mgr should be registered as a state listener. The state listener builds the history and retriver helps in querying the history
type Mgr interface {
	ledger.StateListener
	GetRetriever(ledgerID string, ledgerInfoRetriever LedgerInfoRetriever) ledger.ConfigHistoryRetriever
	Close()
}

type mgr struct {
	dbProvider *dbProvider
}

// NewMgr constructs an instance that implements interface `Mgr`
func NewMgr() Mgr {
	return newMgr(dbPath())
}

func newMgr(dbPath string) Mgr {
	return &mgr{newDBProvider(dbPath)}
}

// InterestedInNamespaces implements function from the interface ledger.StateListener
func (m *mgr) InterestedInNamespaces() []string {
	return []string{lsccNamespace}
}

// StateCommitDone implements function from the interface ledger.StateListener
func (m *mgr) StateCommitDone(ledgerID string) {
	// Noop
}

// HandleStateUpdates implements function from the interface ledger.StateListener
// In this implementation, each collection configurations updates (in lscc namespace)
// are persisted as a separate entry in a separate db. The composite key for the entry
// is a tuple of <blockNum, namespace, key>
func (m *mgr) HandleStateUpdates(trigger *ledger.StateUpdateTrigger) error {
	batch := prepareDBBatch(trigger.StateUpdates, trigger.CommittingBlockNum)
	dbHandle := m.dbProvider.getDB(trigger.LedgerID)
	return dbHandle.writeBatch(batch, true)
}

// GetRetriever returns an implementation of `ledger.ConfigHistoryRetriever` for the given ledger id.
func (m *mgr) GetRetriever(ledgerID string, ledgerInfoRetriever LedgerInfoRetriever) ledger.ConfigHistoryRetriever {
	return &retriever{dbHandle: m.dbProvider.getDB(ledgerID), ledgerInfoRetriever: ledgerInfoRetriever}
}

// Close implements the function in the interface 'Mgr'
func (m *mgr) Close() {
	m.dbProvider.Close()
}

type retriever struct {
	ledgerInfoRetriever LedgerInfoRetriever
	dbHandle            *db
}

// MostRecentCollectionConfigBelow implements function from the interface ledger.ConfigHistoryRetriever
func (r *retriever) MostRecentCollectionConfigBelow(blockNum uint64, chaincodeName string) (*ledger.CollectionConfigInfo, error) {
	compositeKV, err := r.dbHandle.mostRecentEntryBelow(blockNum, lsccNamespace, constructCollectionConfigKey(chaincodeName))
	if err != nil || compositeKV == nil {
		return nil, err
	}
	return compositeKVToCollectionConfig(compositeKV)
}

// CollectionConfigAt implements function from the interface ledger.ConfigHistoryRetriever
func (r *retriever) CollectionConfigAt(blockNum uint64, chaincodeName string) (*ledger.CollectionConfigInfo, error) {
	info, err := r.ledgerInfoRetriever.GetBlockchainInfo()
	if err != nil {
		return nil, err
	}
	maxCommittedBlockNum := info.Height - 1
	if maxCommittedBlockNum < blockNum {
		return nil, &ledger.ErrCollectionConfigNotYetAvailable{MaxBlockNumCommitted: maxCommittedBlockNum,
			Msg: fmt.Sprintf("The maximum block number committed [%d] is less than the requested block number [%d]", maxCommittedBlockNum, blockNum)}
	}

	compositeKV, err := r.dbHandle.entryAt(blockNum, lsccNamespace, constructCollectionConfigKey(chaincodeName))
	if err != nil || compositeKV == nil {
		return nil, err
	}
	return compositeKVToCollectionConfig(compositeKV)
}

func prepareDBBatch(stateUpdates ledger.StateUpdates, committingBlock uint64) *batch {
	batch := newBatch()
	lsccWrites := stateUpdates[lsccNamespace]
	for _, kv := range lsccWrites.([]*kvrwset.KVWrite) {
		if !privdata.IsCollectionConfigKey(kv.Key) {
			continue
		}
		batch.add(lsccNamespace, kv.Key, committingBlock, kv.Value)
	}
	return batch
}

func compositeKVToCollectionConfig(compositeKV *compositeKV) (*ledger.CollectionConfigInfo, error) {
	conf := &common.CollectionConfigPackage{}
	if err := proto.Unmarshal(compositeKV.value, conf); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling compositeKV to collection config")
	}
	return &ledger.CollectionConfigInfo{CollectionConfig: conf, CommittingBlockNum: compositeKV.blockNum}, nil
}

func constructCollectionConfigKey(chaincodeName string) string {
	return privdata.BuildCollectionKVSKey(chaincodeName)
}

func isCollectionConfigKey(key string) bool {
	return privdata.IsCollectionConfigKey(key)
}

func dbPath() string {
	return ledgerconfig.GetConfigHistoryPath()
}

// LedgerInfoRetriever retrieves the relevant info from ledger
type LedgerInfoRetriever interface {
	GetBlockchainInfo() (*common.BlockchainInfo, error)
}
