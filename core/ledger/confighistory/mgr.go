/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package confighistory

import (
	"fmt"
	"path/filepath"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/snapshot"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/pkg/errors"
)

var (
	logger                 = flogging.MustGetLogger("confighistory")
	importConfigsBatchSize = 1024 * 1024
)

const (
	collectionConfigNamespace = "lscc" // lscc namespace was introduced in version 1.2 and we continue to use this in order to be compatible with existing data
	snapshotFileFormat        = byte(1)
	snapshotDataFileName      = "confighistory.data"
	snapshotMetadataFileName  = "confighistory.metadata"
)

// Mgr manages the history of configurations such as chaincode's collection configurations.
// It should be registered as a state listener. The state listener builds the history.
type Mgr struct {
	ccInfoProvider ledger.DeployedChaincodeInfoProvider
	dbProvider     *dbProvider
}

// NewMgr constructs an instance that implements interface `Mgr`
func NewMgr(dbPath string, ccInfoProvider ledger.DeployedChaincodeInfoProvider) (*Mgr, error) {
	p, err := newDBProvider(dbPath)
	if err != nil {
		return nil, err
	}
	return &Mgr{ccInfoProvider, p}, nil
}

// Name returns the name of the listener
func (m *Mgr) Name() string {
	return "collection configuration history listener"
}

// Initialize implements function from the interface ledger.StateListener
func (m *Mgr) Initialize(ledgerID string, qe ledger.SimpleQueryExecutor) error {
	// Noop
	return nil
}

// InterestedInNamespaces implements function from the interface ledger.StateListener
func (m *Mgr) InterestedInNamespaces() []string {
	return m.ccInfoProvider.Namespaces()
}

// StateCommitDone implements function from the interface ledger.StateListener
func (m *Mgr) StateCommitDone(ledgerID string) {
	// Noop
}

// HandleStateUpdates implements function from the interface ledger.StateListener
// In this implementation, the latest collection config package is retrieved via
// ledger.DeployedChaincodeInfoProvider and is persisted as a separate entry in a separate db.
// The composite key for the entry is a tuple of <blockNum, namespace, key>
func (m *Mgr) HandleStateUpdates(trigger *ledger.StateUpdateTrigger) error {
	updatedCCs, err := m.ccInfoProvider.UpdatedChaincodes(extractPublicUpdates(trigger.StateUpdates))
	if err != nil {
		return err
	}
	// updated chaincodes can be empty if the invocation to this function is triggered
	// because of state updates that contains only chaincode approval transaction output
	if len(updatedCCs) == 0 {
		return nil
	}
	updatedCollConfigs := map[string]*peer.CollectionConfigPackage{}
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
	dbHandle := m.dbProvider.getDB(trigger.LedgerID)
	batch := dbHandle.newBatch()
	err = prepareDBBatch(batch, updatedCollConfigs, trigger.CommittingBlockNum)
	if err != nil {
		return err
	}
	return dbHandle.writeBatch(batch, true)
}

// ImportConfigHistory imports the collection config history associated with a given
// ledgerID from the snapshot files present in the dir
func (m *Mgr) ImportFromSnapshot(ledgerID string, dir string) error {
	exist, _, err := fileutil.FileExists(filepath.Join(dir, snapshotDataFileName))
	if err != nil {
		return err
	}
	if !exist {
		// when the ledger being bootstapped never had a private data collection for
		// any chaincode, the snapshot files associated with the confighistory store
		// will not be present in the snapshot directory. Hence, we can return early
		return nil
	}
	db := m.dbProvider.getDB(ledgerID)
	empty, err := db.IsEmpty()
	if err != nil {
		return err
	}
	if !empty {
		return errors.New(fmt.Sprintf(
			"config history for ledger [%s] exists. Incremental import is not supported. "+
				"Remove the existing ledger data before retry",
			ledgerID,
		))
	}

	configMetadata, err := snapshot.OpenFile(filepath.Join(dir, snapshotMetadataFileName), snapshotFileFormat)
	if err != nil {
		return err
	}
	numCollectionConfigs, err := configMetadata.DecodeUVarInt()
	if err != nil {
		return err
	}
	collectionConfigData, err := snapshot.OpenFile(filepath.Join(dir, snapshotDataFileName), snapshotFileFormat)
	if err != nil {
		return err
	}

	batch := db.NewUpdateBatch()
	currentBatchSize := 0
	for i := uint64(0); i < numCollectionConfigs; i++ {
		key, err := collectionConfigData.DecodeBytes()
		if err != nil {
			return err
		}
		val, err := collectionConfigData.DecodeBytes()
		if err != nil {
			return err
		}
		batch.Put(key, val)
		currentBatchSize += len(key) + len(val)
		if currentBatchSize >= importConfigsBatchSize {
			if err := db.WriteBatch(batch, true); err != nil {
				return err
			}
			currentBatchSize = 0
			batch.Reset()
		}
	}
	return db.WriteBatch(batch, true)
}

// GetRetriever returns an implementation of `ledger.ConfigHistoryRetriever` for the given ledger id.
func (m *Mgr) GetRetriever(ledgerID string) *Retriever {
	return &Retriever{
		ledgerID: ledgerID,
		dbHandle: m.dbProvider.getDB(ledgerID),
	}
}

// Close implements the function in the interface 'Mgr'
func (m *Mgr) Close() {
	m.dbProvider.Close()
}

// Drop drops channel-specific data from the config history db
func (m *Mgr) Drop(ledgerid string) error {
	return m.dbProvider.Drop(ledgerid)
}

// Retriever helps consumer retrieve collection config history
type Retriever struct {
	ledgerID string
	dbHandle *db
}

// MostRecentCollectionConfigBelow implements function from the interface ledger.ConfigHistoryRetriever
func (r *Retriever) MostRecentCollectionConfigBelow(blockNum uint64, chaincodeName string) (*ledger.CollectionConfigInfo, error) {
	compositeKV, err := r.dbHandle.mostRecentEntryBelow(blockNum, collectionConfigNamespace, constructCollectionConfigKey(chaincodeName))
	if err != nil || compositeKV == nil {
		return nil, err
	}
	return compositeKVToCollectionConfig(compositeKV)
}

// ExportConfigHistory exports configuration history from the confighistoryDB to
// a file. Currently, we store only one type of configuration in the db, i.e.,
// private data collection configuration.
// We write the full key and value stored in the database as is to the file.
// Though we could decode the key and write a proto message with exact ns, key,
// block number, and collection config, we store the full key and value to avoid
// unnecessary encoding and decoding of proto messages.
// The key format stored in db is "s" + ns + byte(0) + key + "~collection" + byte(0)
// + blockNum. As we store the key as is, we store 13 extra bytes. For a million
// records, it would add only 12 MB overhead. Note that the protobuf also adds some
// extra bytes. Further, the collection config namespace is not expected to have
// millions of entries.
func (r *Retriever) ExportConfigHistory(dir string, newHashFunc snapshot.NewHashFunc) (map[string][]byte, error) {
	nsItr, err := r.dbHandle.getNamespaceIterator(collectionConfigNamespace)
	if err != nil {
		return nil, err
	}
	defer nsItr.Release()

	var numCollectionConfigs uint64 = 0
	var dataFileWriter *snapshot.FileWriter
	for nsItr.Next() {
		if err := nsItr.Error(); err != nil {
			return nil, errors.Wrap(err, "internal leveldb error while iterating for collection config history")
		}
		if numCollectionConfigs == 0 { // first iteration, create the data file
			dataFileWriter, err = snapshot.CreateFile(filepath.Join(dir, snapshotDataFileName), snapshotFileFormat, newHashFunc)
			if err != nil {
				return nil, err
			}
			defer dataFileWriter.Close()
		}
		if err := dataFileWriter.EncodeBytes(nsItr.Key()); err != nil {
			return nil, err
		}
		if err := dataFileWriter.EncodeBytes(nsItr.Value()); err != nil {
			return nil, err
		}
		numCollectionConfigs++
	}

	if dataFileWriter == nil {
		return nil, nil
	}

	dataHash, err := dataFileWriter.Done()
	if err != nil {
		return nil, err
	}
	metadataFileWriter, err := snapshot.CreateFile(filepath.Join(dir, snapshotMetadataFileName), snapshotFileFormat, newHashFunc)
	if err != nil {
		return nil, err
	}
	defer metadataFileWriter.Close()
	if err = metadataFileWriter.EncodeUVarint(numCollectionConfigs); err != nil {
		return nil, err
	}
	metadataHash, err := metadataFileWriter.Done()
	if err != nil {
		return nil, err
	}

	return map[string][]byte{
		snapshotDataFileName:     dataHash,
		snapshotMetadataFileName: metadataHash,
	}, nil
}

func prepareDBBatch(batch *batch, chaincodeCollConfigs map[string]*peer.CollectionConfigPackage, committingBlockNum uint64) error {
	for ccName, collConfig := range chaincodeCollConfigs {
		key := constructCollectionConfigKey(ccName)
		var configBytes []byte
		var err error
		if configBytes, err = proto.Marshal(collConfig); err != nil {
			return errors.WithStack(err)
		}
		batch.add(collectionConfigNamespace, key, committingBlockNum, configBytes)
	}
	return nil
}

func compositeKVToCollectionConfig(compositeKV *compositeKV) (*ledger.CollectionConfigInfo, error) {
	conf := &peer.CollectionConfigPackage{}
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
