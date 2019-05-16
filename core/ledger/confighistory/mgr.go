/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package confighistory

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("confighistory")

const (
	collectionConfigNamespace = "lscc" // lscc namespace was introduced in version 1.2 and we continue to use this in order to be compatible with existing data
)

// Mgr should be registered as a state listener. The state listener builds the history and retriver helps in querying the history
type Mgr interface {
	ledger.StateListener
	GetRetriever(ledgerID string, ledgerInfoRetriever LedgerInfoRetriever) ledger.ConfigHistoryRetriever
	Close()
}

type mgr struct {
	ccInfoProvider ledger.DeployedChaincodeInfoProvider
	dbProvider     *dbProvider
}

// NewMgr constructs an instance that implements interface `Mgr`
func NewMgr(dbPath string, ccInfoProvider ledger.DeployedChaincodeInfoProvider) Mgr {
	return &mgr{ccInfoProvider, newDBProvider(dbPath)}
}

func (m *mgr) Initialize(ledgerID string, qe ledger.SimpleQueryExecutor) error {
	// Noop
	return nil
}

// InterestedInNamespaces implements function from the interface ledger.StateListener
func (m *mgr) InterestedInNamespaces() []string {
	return m.ccInfoProvider.Namespaces()
}

// StateCommitDone implements function from the interface ledger.StateListener
func (m *mgr) StateCommitDone(ledgerID string) {
	// Noop
}

// HandleStateUpdates implements function from the interface ledger.StateListener
// In this implementation, the latest collection config package is retrieved via
// ledger.DeployedChaincodeInfoProvider and is persisted as a separate entry in a separate db.
// The composite key for the entry is a tuple of <blockNum, namespace, key>
func (m *mgr) HandleStateUpdates(trigger *ledger.StateUpdateTrigger) error {
	updatedCCs, err := m.ccInfoProvider.UpdatedChaincodes(extractPublicUpdates(trigger.StateUpdates))
	if err != nil {
		return err
	}
	// updated chaincodes can be empty if the invocation to this function is triggered
	// because of state updates that contains only chaincode approval transaction output
	if len(updatedCCs) == 0 {
		return nil
	}
	updatedCollConfigs := map[string]*common.CollectionConfigPackage{}
	for _, cc := range updatedCCs {
		ccInfo, err := m.ccInfoProvider.ChaincodeInfo(trigger.LedgerID, cc.Name, trigger.PostCommitQueryExecutor)
		if err != nil {
			return err
		}

		// DeployedChaincodeInfoProvider implementation in new lifecycle return an empty 'CollectionConfigPackage'
		// (instead of a nil) to indicate the absence of collection config, so check for both conditions
		if ccInfo.ExplicitCollectionConfigPkg == nil || len(ccInfo.ExplicitCollectionConfigPkg.Config) == 0 {
			continue
		}
		updatedCollConfigs[ccInfo.Name] = ccInfo.ExplicitCollectionConfigPkg
	}
	if len(updatedCollConfigs) == 0 {
		return nil
	}
	batch, err := prepareDBBatch(updatedCollConfigs, trigger.CommittingBlockNum)
	if err != nil {
		return err
	}
	dbHandle := m.dbProvider.getDB(trigger.LedgerID)
	return dbHandle.writeBatch(batch, true)
}

// GetRetriever returns an implementation of `ledger.ConfigHistoryRetriever` for the given ledger id.
func (m *mgr) GetRetriever(ledgerID string, ledgerInfoRetriever LedgerInfoRetriever) ledger.ConfigHistoryRetriever {
	return &retriever{
		ledgerInfoRetriever:    ledgerInfoRetriever,
		ledgerID:               ledgerID,
		deployedCCInfoProvider: m.ccInfoProvider,
		dbHandle:               m.dbProvider.getDB(ledgerID),
	}
}

// Close implements the function in the interface 'Mgr'
func (m *mgr) Close() {
	m.dbProvider.Close()
}

type retriever struct {
	ledgerInfoRetriever    LedgerInfoRetriever
	ledgerID               string
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider
	dbHandle               *db
}

// MostRecentCollectionConfigBelow implements function from the interface ledger.ConfigHistoryRetriever
func (r *retriever) MostRecentCollectionConfigBelow(blockNum uint64, chaincodeName string) (*ledger.CollectionConfigInfo, error) {
	compositeKV, err := r.dbHandle.mostRecentEntryBelow(blockNum, collectionConfigNamespace, constructCollectionConfigKey(chaincodeName))
	if err != nil {
		return nil, err
	}
	implicitColls, err := r.getImplicitCollection(chaincodeName)
	if err != nil {
		return nil, err
	}

	return constructCollectionConfigInfo(compositeKV, implicitColls)
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

	compositeKV, err := r.dbHandle.entryAt(blockNum, collectionConfigNamespace, constructCollectionConfigKey(chaincodeName))
	if err != nil {
		return nil, err
	}
	implicitColls, err := r.getImplicitCollection(chaincodeName)
	if err != nil {
		return nil, err
	}
	return constructCollectionConfigInfo(compositeKV, implicitColls)
}

func (r *retriever) getImplicitCollection(chaincodeName string) ([]*common.StaticCollectionConfig, error) {
	qe, err := r.ledgerInfoRetriever.NewQueryExecutor()
	if err != nil {
		return nil, err
	}
	defer qe.Done()
	return r.deployedCCInfoProvider.ImplicitCollections(r.ledgerID, chaincodeName, qe)
}

func prepareDBBatch(chaincodeCollConfigs map[string]*common.CollectionConfigPackage, committingBlockNum uint64) (*batch, error) {
	batch := newBatch()
	for ccName, collConfig := range chaincodeCollConfigs {
		key := constructCollectionConfigKey(ccName)
		var configBytes []byte
		var err error
		if configBytes, err = proto.Marshal(collConfig); err != nil {
			return nil, errors.WithStack(err)
		}
		batch.add(collectionConfigNamespace, key, committingBlockNum, configBytes)
	}
	return batch, nil
}

func compositeKVToCollectionConfig(compositeKV *compositeKV) (*ledger.CollectionConfigInfo, error) {
	conf := &common.CollectionConfigPackage{}
	if err := proto.Unmarshal(compositeKV.value, conf); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling compositeKV to collection config")
	}
	return &ledger.CollectionConfigInfo{
		CollectionConfig:   conf,
		CommittingBlockNum: compositeKV.blockNum,
	}, nil
}

func constructCollectionConfigKey(chaincodeName string) string {
	return chaincodeName + "~collection" // collection config key as in version 1.2 and we continue to use this in order to be compatible with existing data
}

func extractPublicUpdates(stateUpdates ledger.StateUpdates) map[string][]*kvrwset.KVWrite {
	m := map[string][]*kvrwset.KVWrite{}
	for ns, updates := range stateUpdates {
		m[ns] = updates.PublicUpdates
	}
	return m
}

func constructCollectionConfigInfo(
	compositeKV *compositeKV,
	implicitColls []*common.StaticCollectionConfig,
) (*ledger.CollectionConfigInfo, error) {
	var collConf *ledger.CollectionConfigInfo
	var err error

	if compositeKV == nil && len(implicitColls) == 0 {
		return nil, nil
	}

	collConf = &ledger.CollectionConfigInfo{
		CollectionConfig: &common.CollectionConfigPackage{},
	}
	if compositeKV != nil {
		if collConf, err = compositeKVToCollectionConfig(compositeKV); err != nil {
			return nil, err
		}
	}

	for _, implicitColl := range implicitColls {
		cc := &common.CollectionConfig{}
		cc.Payload = &common.CollectionConfig_StaticCollectionConfig{StaticCollectionConfig: implicitColl}
		collConf.CollectionConfig.Config = append(
			collConf.CollectionConfig.Config,
			cc,
		)
	}
	return collConf, nil
}

// LedgerInfoRetriever retrieves the relevant info from ledger
type LedgerInfoRetriever interface {
	GetBlockchainInfo() (*common.BlockchainInfo, error)
	NewQueryExecutor() (ledger.QueryExecutor, error)
}
