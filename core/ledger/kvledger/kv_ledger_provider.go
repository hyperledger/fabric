/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/dataformat"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/confighistory"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history"
	"github.com/hyperledger/fabric/core/ledger/kvledger/msgs"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/pvtdatastorage"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
)

var (
	// genesisBlkKeyPrefix is the prefix for each ledger key in idStore db
	genesisBlkKeyPrefix = []byte{'l'}
	// genesisBlkKeyStop is the end key when querying idStore db by ledger key
	genesisBlkKeyStop = []byte{'l' + 1}
	// metadataKeyPrefix is the prefix for each metadata key in idStore db
	metadataKeyPrefix = []byte{'s'}
	// metadataKeyStop is the end key when querying idStore db by metadata key
	metadataKeyStop = []byte{'s' + 1}

	// formatKey
	formatKey = []byte("f")

	attrsToIndex = []blkstorage.IndexableAttr{
		blkstorage.IndexableAttrBlockHash,
		blkstorage.IndexableAttrBlockNum,
		blkstorage.IndexableAttrTxID,
		blkstorage.IndexableAttrBlockNumTranNum,
	}
)

const maxBlockFileSize = 64 * 1024 * 1024

// Provider implements interface ledger.PeerLedgerProvider
type Provider struct {
	idStore              *idStore
	blkStoreProvider     *blkstorage.BlockStoreProvider
	pvtdataStoreProvider *pvtdatastorage.Provider
	dbProvider           *privacyenabledstate.DBProvider
	historydbProvider    *history.DBProvider
	configHistoryMgr     *confighistory.Mgr
	stateListeners       []ledger.StateListener
	bookkeepingProvider  *bookkeeping.Provider
	initializer          *ledger.Initializer
	collElgNotifier      *collElgNotifier
	stats                *stats
	fileLock             *leveldbhelper.FileLock
}

// NewProvider instantiates a new Provider.
// This is not thread-safe and assumed to be synchronized by the caller
func NewProvider(initializer *ledger.Initializer) (pr *Provider, e error) {
	p := &Provider{
		initializer: initializer,
	}

	defer func() {
		if e != nil {
			p.Close()
			if errFormatMismatch, ok := e.(*dataformat.ErrFormatMismatch); ok {
				if errFormatMismatch.Format == dataformat.PreviousFormat && errFormatMismatch.ExpectedFormat == dataformat.CurrentFormat {
					logger.Errorf("Please execute the 'peer node upgrade-dbs' command to upgrade the database format: %s", errFormatMismatch)
				} else {
					logger.Errorf("Please check the Fabric version matches the ledger data format: %s", errFormatMismatch)
				}
			}
		}
	}()

	fileLockPath := fileLockPath(initializer.Config.RootFSPath)
	fileLock := leveldbhelper.NewFileLock(fileLockPath)
	if err := fileLock.Lock(); err != nil {
		return nil, errors.Wrap(err, "as another peer node command is executing,"+
			" wait for that command to complete its execution or terminate it before retrying")
	}

	p.fileLock = fileLock

	if err := p.initLedgerIDInventory(); err != nil {
		return nil, err
	}
	if err := p.initBlockStoreProvider(); err != nil {
		return nil, err
	}
	if err := p.initPvtDataStoreProvider(); err != nil {
		return nil, err
	}
	if err := p.initHistoryDBProvider(); err != nil {
		return nil, err
	}
	if err := p.initConfigHistoryManager(); err != nil {
		return nil, err
	}
	p.initCollElgNotifier()
	p.initStateListeners()
	if err := p.initStateDBProvider(); err != nil {
		return nil, err
	}
	p.initLedgerStatistics()
	if err := p.deletePartialLedgers(); err != nil {
		return nil, err
	}
	if err := p.initSnapshotDir(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Provider) initLedgerIDInventory() error {
	idStore, err := openIDStore(LedgerProviderPath(p.initializer.Config.RootFSPath))
	if err != nil {
		return err
	}
	p.idStore = idStore
	return nil
}

func (p *Provider) initBlockStoreProvider() error {
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	blkStoreProvider, err := blkstorage.NewProvider(
		blkstorage.NewConf(
			BlockStorePath(p.initializer.Config.RootFSPath),
			maxBlockFileSize,
		),
		indexConfig,
		p.initializer.MetricsProvider,
	)
	if err != nil {
		return err
	}
	p.blkStoreProvider = blkStoreProvider
	return nil
}

func (p *Provider) initPvtDataStoreProvider() error {
	privateDataConfig := &pvtdatastorage.PrivateDataConfig{
		PrivateDataConfig: p.initializer.Config.PrivateDataConfig,
		StorePath:         PvtDataStorePath(p.initializer.Config.RootFSPath),
	}
	ledgerIDs, err := p.idStore.getActiveAndInactiveLedgerIDs()
	if err != nil {
		return err
	}
	if err := pvtdatastorage.CheckAndConstructHashedIndex(privateDataConfig.StorePath, ledgerIDs); err != nil {
		return err
	}
	pvtdataStoreProvider, err := pvtdatastorage.NewProvider(privateDataConfig)
	if err != nil {
		return err
	}
	p.pvtdataStoreProvider = pvtdataStoreProvider
	return nil
}

func (p *Provider) initHistoryDBProvider() error {
	if !p.initializer.Config.HistoryDBConfig.Enabled {
		return nil
	}
	// Initialize the history database (index for history of values by key)
	historydbProvider, err := history.NewDBProvider(
		HistoryDBPath(p.initializer.Config.RootFSPath),
	)
	if err != nil {
		return err
	}
	p.historydbProvider = historydbProvider
	return nil
}

func (p *Provider) initConfigHistoryManager() error {
	var err error
	configHistoryMgr, err := confighistory.NewMgr(
		ConfigHistoryDBPath(p.initializer.Config.RootFSPath),
		p.initializer.DeployedChaincodeInfoProvider,
	)
	if err != nil {
		return err
	}
	p.configHistoryMgr = configHistoryMgr
	return nil
}

func (p *Provider) initCollElgNotifier() {
	collElgNotifier := &collElgNotifier{
		p.initializer.DeployedChaincodeInfoProvider,
		p.initializer.MembershipInfoProvider,
		make(map[string]collElgListener),
	}
	p.collElgNotifier = collElgNotifier
}

func (p *Provider) initStateListeners() {
	stateListeners := p.initializer.StateListeners
	stateListeners = append(stateListeners, p.collElgNotifier)
	stateListeners = append(stateListeners, p.configHistoryMgr)
	p.stateListeners = stateListeners
}

func (p *Provider) initStateDBProvider() error {
	var err error
	p.bookkeepingProvider, err = bookkeeping.NewProvider(
		BookkeeperDBPath(p.initializer.Config.RootFSPath),
	)
	if err != nil {
		return err
	}
	stateDBConfig := &privacyenabledstate.StateDBConfig{
		StateDBConfig: p.initializer.Config.StateDBConfig,
		LevelDBPath:   StateDBPath(p.initializer.Config.RootFSPath),
	}
	sysNamespaces := p.initializer.DeployedChaincodeInfoProvider.Namespaces()
	p.dbProvider, err = privacyenabledstate.NewDBProvider(
		p.bookkeepingProvider,
		p.initializer.MetricsProvider,
		p.initializer.HealthCheckRegistry,
		stateDBConfig,
		sysNamespaces,
	)
	return err
}

func (p *Provider) initLedgerStatistics() {
	p.stats = newStats(p.initializer.MetricsProvider)
}

func (p *Provider) initSnapshotDir() error {
	snapshotsRootDir := p.initializer.Config.SnapshotsConfig.RootDir
	if !filepath.IsAbs(snapshotsRootDir) {
		return errors.Errorf("invalid path: %s. The path for the snapshot dir is expected to be an absolute path", snapshotsRootDir)
	}

	inProgressSnapshotsPath := SnapshotsTempDirPath(snapshotsRootDir)
	completedSnapshotsPath := CompletedSnapshotsPath(snapshotsRootDir)

	if err := os.RemoveAll(inProgressSnapshotsPath); err != nil {
		return errors.Wrapf(err, "error while deleting the dir: %s", inProgressSnapshotsPath)
	}
	if err := os.MkdirAll(inProgressSnapshotsPath, 0o755); err != nil {
		return errors.Wrapf(err, "error while creating the dir: %s, ensure peer has write access to configured ledger.snapshots.rootDir directory", inProgressSnapshotsPath)
	}
	if err := os.MkdirAll(completedSnapshotsPath, 0o755); err != nil {
		return errors.Wrapf(err, "error while creating the dir: %s, ensure peer has write access to configured ledger.snapshots.rootDir directory", completedSnapshotsPath)
	}
	return fileutil.SyncDir(snapshotsRootDir)
}

// CreateFromGenesisBlock implements the corresponding method from interface ledger.PeerLedgerProvider
// This function creates a new ledger and commits the genesis block. If a failure happens during this
// process, the partially created ledger is deleted
func (p *Provider) CreateFromGenesisBlock(genesisBlock *common.Block) (ledger.PeerLedger, error) {
	ledgerID, err := protoutil.GetChannelIDFromBlock(genesisBlock)
	if err != nil {
		return nil, err
	}
	if err = p.idStore.createLedgerID(
		ledgerID,
		&msgs.LedgerMetadata{
			Status: msgs.Status_UNDER_CONSTRUCTION,
		},
	); err != nil {
		return nil, err
	}

	lgr, err := p.open(ledgerID, nil, false)
	if err != nil {
		return nil, p.deleteUnderConstructionLedger(lgr, ledgerID, err)
	}

	if err = lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: genesisBlock}, &ledger.CommitOptions{}); err != nil {
		return nil, p.deleteUnderConstructionLedger(lgr, ledgerID, err)
	}

	if err = p.idStore.updateLedgerStatus(ledgerID, msgs.Status_ACTIVE); err != nil {
		return nil, p.deleteUnderConstructionLedger(lgr, ledgerID, err)
	}
	return lgr, nil
}

func (p *Provider) deleteUnderConstructionLedger(ledger ledger.PeerLedger, ledgerID string, creationErr error) error {
	if creationErr == nil {
		return nil
	} else {
		logger.Errorf("ledger creation error = %+v", creationErr)
	}

	if ledger != nil {
		ledger.Close()
	}
	cleanupErr := p.runCleanup(ledgerID)
	if cleanupErr == nil {
		return creationErr
	}
	return errors.WithMessagef(cleanupErr, creationErr.Error())
}

// Open implements the corresponding method from interface ledger.PeerLedgerProvider
func (p *Provider) Open(ledgerID string) (ledger.PeerLedger, error) {
	logger.Debugf("Open() opening kvledger: %s", ledgerID)
	// Check the ID store to ensure that the chainId/ledgerId exists
	ledgerMetadata, err := p.idStore.getLedgerMetadata(ledgerID)
	if err != nil {
		return nil, err
	}
	if ledgerMetadata == nil {
		return nil, errors.Errorf("cannot open ledger [%s], ledger does not exist", ledgerID)
	}
	if ledgerMetadata.Status != msgs.Status_ACTIVE {
		return nil, errors.Errorf("cannot open ledger [%s], ledger status is [%s]", ledgerID, ledgerMetadata.Status)
	}

	bootSnapshotMetadata, err := snapshotMetadataFromProto(ledgerMetadata.BootSnapshotMetadata)
	if err != nil {
		return nil, err
	}
	return p.open(ledgerID, bootSnapshotMetadata, false)
}

func (p *Provider) open(ledgerID string, bootSnapshotMetadata *SnapshotMetadata, initializingFromSnapshot bool) (ledger.PeerLedger, error) {
	// Get the block store for a chain/ledger
	blockStore, err := p.blkStoreProvider.Open(ledgerID)
	if err != nil {
		return nil, err
	}
	pvtdataStore, err := p.pvtdataStoreProvider.OpenStore(ledgerID)
	if err != nil {
		return nil, err
	}

	p.collElgNotifier.registerListener(ledgerID, pvtdataStore)

	// Get the versioned database (state database) for a chain/ledger
	channelInfoProvider := &channelInfoProvider{ledgerID, blockStore, p.collElgNotifier.deployedChaincodeInfoProvider}
	db, err := p.dbProvider.GetDBHandle(ledgerID, channelInfoProvider)
	if err != nil {
		return nil, err
	}

	// Get the history database (index for history of values by key) for a chain/ledger
	var historyDB *history.DB
	if p.historydbProvider != nil {
		historyDB = p.historydbProvider.GetDBHandle(ledgerID)
	}

	initializer := &lgrInitializer{
		ledgerID:                 ledgerID,
		blockStore:               blockStore,
		pvtdataStore:             pvtdataStore,
		stateDB:                  db,
		historyDB:                historyDB,
		configHistoryMgr:         p.configHistoryMgr,
		stateListeners:           p.stateListeners,
		bookkeeperProvider:       p.bookkeepingProvider,
		ccInfoProvider:           p.initializer.DeployedChaincodeInfoProvider,
		ccLifecycleEventProvider: p.initializer.ChaincodeLifecycleEventProvider,
		stats:                    p.stats.ledgerStats(ledgerID),
		customTxProcessors:       p.initializer.CustomTxProcessors,
		hashProvider:             p.initializer.HashProvider,
		config:                   p.initializer.Config,
		bootSnapshotMetadata:     bootSnapshotMetadata,
		initializingFromSnapshot: initializingFromSnapshot,
	}

	l, err := newKVLedger(initializer)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Exists implements the corresponding method from interface ledger.PeerLedgerProvider
func (p *Provider) Exists(ledgerID string) (bool, error) {
	return p.idStore.ledgerIDExists(ledgerID)
}

// List implements the corresponding method from interface ledger.PeerLedgerProvider
func (p *Provider) List() ([]string, error) {
	return p.idStore.getActiveLedgerIDs()
}

// Close implements the corresponding method from interface ledger.PeerLedgerProvider
func (p *Provider) Close() {
	if p.idStore != nil {
		p.idStore.close()
	}
	if p.blkStoreProvider != nil {
		p.blkStoreProvider.Close()
	}
	if p.pvtdataStoreProvider != nil {
		p.pvtdataStoreProvider.Close()
	}
	if p.dbProvider != nil {
		p.dbProvider.Close()
	}
	if p.bookkeepingProvider != nil {
		p.bookkeepingProvider.Close()
	}
	if p.configHistoryMgr != nil {
		p.configHistoryMgr.Close()
	}
	if p.historydbProvider != nil {
		p.historydbProvider.Close()
	}
	if p.fileLock != nil {
		p.fileLock.Unlock()
	}
}

// deletePartialLedgers scans for and deletes any ledger with a status of UNDER_CONSTRUCTION or UNDER_DELETION.
// UNDER_CONSTRUCTION ledgers represent residual structures created as a side effect of a crash during ledger creation.
// UNDER_DELETION ledgers represent residual structures created as a side effect of a crash during a peer channel unjoin.
func (p *Provider) deletePartialLedgers() error {
	logger.Debug("Removing ledgers in state UNDER_CONSTRUCTION or UNDER_DELETION")
	itr := p.idStore.db.GetIterator(metadataKeyPrefix, metadataKeyStop)
	defer itr.Release()
	if err := itr.Error(); err != nil {
		return errors.WithMessage(err, "error obtaining iterator for incomplete ledger scans")
	}
	for {
		hasMore := itr.Next()
		err := itr.Error()
		if err != nil {
			return errors.WithMessage(err, "error while iterating over ledger list while scanning for incomplete ledgers")
		}
		if !hasMore {
			return nil
		}
		ledgerID := ledgerIDFromMetadataKey(itr.Key())
		metadata := &msgs.LedgerMetadata{}
		if err := proto.Unmarshal(itr.Value(), metadata); err != nil {
			return errors.Wrapf(err, "error while unmarshalling metadata bytes for ledger [%s]", ledgerID)
		}
		if metadata.Status == msgs.Status_UNDER_CONSTRUCTION || metadata.Status == msgs.Status_UNDER_DELETION {
			logger.Infow(
				"A partial ledger was identified at peer launch, indicating a peer stop/crash during creation or a failed channel unjoin.  The partial ledger wil be deleted.",
				"ledgerID", ledgerID,
				"Status", metadata.Status,
			)
			if err := p.runCleanup(ledgerID); err != nil {
				logger.Errorw(
					"Error while deleting a partially created ledger at start",
					"ledgerID", ledgerID,
					"Status", metadata.Status,
					"error", err,
				)
				return errors.WithMessagef(err, "error while deleting a partially constructed ledger with status [%s] at start for ledger = [%s]", metadata.Status, ledgerID)
			}
		}
	}
}

// runCleanup cleans up blockstorage, statedb, and historydb for what
// may have got created during in-complete ledger creation
func (p *Provider) runCleanup(ledgerID string) error {
	ledgerDataRemover := &ledgerDataRemover{
		blkStoreProvider:     p.blkStoreProvider,
		statedbProvider:      p.dbProvider,
		bookkeepingProvider:  p.bookkeepingProvider,
		configHistoryMgr:     p.configHistoryMgr,
		historydbProvider:    p.historydbProvider,
		pvtdataStoreProvider: p.pvtdataStoreProvider,
	}
	if err := ledgerDataRemover.Drop(ledgerID); err != nil {
		return errors.WithMessagef(err, "error while deleting data from ledger [%s]", ledgerID)
	}

	return p.idStore.deleteLedgerID(ledgerID)
}

func snapshotMetadataFromProto(p *msgs.BootSnapshotMetadata) (*SnapshotMetadata, error) {
	if p == nil {
		return nil, nil
	}

	m := &SnapshotMetadataJSONs{
		signableMetadata:   p.SingableMetadata,
		additionalMetadata: p.AdditionalMetadata,
	}

	return m.ToMetadata()
}

// ////////////////////////////////////////////////////////////////////
// Ledger id persistence related code
// /////////////////////////////////////////////////////////////////////
type idStore struct {
	db     *leveldbhelper.DB
	dbPath string
}

func openIDStore(path string) (s *idStore, e error) {
	db := leveldbhelper.CreateDB(&leveldbhelper.Conf{DBPath: path})
	db.Open()
	defer func() {
		if e != nil {
			db.Close()
		}
	}()

	emptyDB, err := db.IsEmpty()
	if err != nil {
		return nil, err
	}

	expectedFormatBytes := []byte(dataformat.CurrentFormat)
	if emptyDB {
		// add format key to a new db
		err := db.Put(formatKey, expectedFormatBytes, true)
		if err != nil {
			return nil, err
		}
		return &idStore{db, path}, nil
	}

	// verify the format is current for an existing db
	format, err := db.Get(formatKey)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(format, expectedFormatBytes) {
		logger.Errorf("The db at path [%s] contains data in unexpected format. expected data format = [%s] (%#v), data format = [%s] (%#v).",
			path, dataformat.CurrentFormat, expectedFormatBytes, format, format)
		return nil, &dataformat.ErrFormatMismatch{
			ExpectedFormat: dataformat.CurrentFormat,
			Format:         string(format),
			DBInfo:         fmt.Sprintf("leveldb for channel-IDs at [%s]", path),
		}
	}
	return &idStore{db, path}, nil
}

// checkUpgradeEligibility checks if the format is eligible to upgrade.
// It returns true if the format is eligible to upgrade to the current format.
// It returns false if either the format is the current format or the db is empty.
// Otherwise, an ErrFormatMismatch is returned.
func (s *idStore) checkUpgradeEligibility() (bool, error) {
	emptydb, err := s.db.IsEmpty()
	if err != nil {
		return false, err
	}
	if emptydb {
		logger.Warnf("Ledger database %s is empty, nothing to upgrade", s.dbPath)
		return false, nil
	}
	format, err := s.db.Get(formatKey)
	if err != nil {
		return false, err
	}
	if bytes.Equal(format, []byte(dataformat.CurrentFormat)) {
		logger.Debugf("Ledger database %s has current data format, nothing to upgrade", s.dbPath)
		return false, nil
	}
	if !bytes.Equal(format, []byte(dataformat.PreviousFormat)) {
		err = &dataformat.ErrFormatMismatch{
			ExpectedFormat: dataformat.PreviousFormat,
			Format:         string(format),
			DBInfo:         fmt.Sprintf("leveldb for channel-IDs at [%s]", s.dbPath),
		}
		return false, err
	}
	return true, nil
}

func (s *idStore) upgradeFormat() error {
	eligible, err := s.checkUpgradeEligibility()
	if err != nil {
		return err
	}
	if !eligible {
		return nil
	}

	logger.Infof("Upgrading ledgerProvider database to the new format %s", dataformat.CurrentFormat)

	batch := &leveldb.Batch{}
	batch.Put(formatKey, []byte(dataformat.CurrentFormat))

	// add new metadata key for each ledger (channel)
	metadata, err := protoutil.Marshal(&msgs.LedgerMetadata{Status: msgs.Status_ACTIVE})
	if err != nil {
		logger.Errorf("Error marshalling ledger metadata: %s", err)
		return errors.Wrapf(err, "error marshalling ledger metadata")
	}
	itr := s.db.GetIterator(genesisBlkKeyPrefix, genesisBlkKeyStop)
	defer itr.Release()
	for itr.Error() == nil && itr.Next() {
		id := ledgerIDFromGenesisBlockKey(itr.Key())
		batch.Put(metadataKey(id), metadata)
	}
	if err = itr.Error(); err != nil {
		logger.Errorf("Error while upgrading idStore format: %s", err)
		return errors.Wrapf(err, "error while upgrading idStore format")
	}

	return s.db.WriteBatch(batch, true)
}

func (s *idStore) createLedgerID(ledgerID string, metadata *msgs.LedgerMetadata) error {
	m, err := s.getLedgerMetadata(ledgerID)
	if err != nil {
		return err
	}
	if m != nil {
		return errors.Errorf("ledger [%s] already exists with state [%s]", ledgerID, m.GetStatus())
	}
	metadataBytes, err := protoutil.Marshal(metadata)
	if err != nil {
		return err
	}
	return s.db.Put(metadataKey(ledgerID), metadataBytes, true)
}

func (s *idStore) deleteLedgerID(ledgerID string) error {
	return s.db.Delete(metadataKey(ledgerID), true)
}

func (s *idStore) updateLedgerStatus(ledgerID string, newStatus msgs.Status) error {
	metadata, err := s.getLedgerMetadata(ledgerID)
	if err != nil {
		return err
	}
	if metadata == nil {
		logger.Errorf("LedgerID [%s] does not exist", ledgerID)
		return errors.Errorf("cannot update ledger status, ledger [%s] does not exist", ledgerID)
	}
	if metadata.Status == newStatus {
		logger.Infof("Ledger [%s] is already in [%s] status, nothing to do", ledgerID, newStatus)
		return nil
	}
	metadata.Status = newStatus
	metadataBytes, err := proto.Marshal(metadata)
	if err != nil {
		logger.Errorf("Error marshalling ledger metadata: %s", err)
		return errors.Wrapf(err, "error marshalling ledger metadata")
	}
	logger.Infof("Updating ledger [%s] status to [%s]", ledgerID, newStatus)
	key := metadataKey(ledgerID)
	return s.db.Put(key, metadataBytes, true)
}

func (s *idStore) getLedgerMetadata(ledgerID string) (*msgs.LedgerMetadata, error) {
	val, err := s.db.Get(metadataKey(ledgerID))
	if val == nil || err != nil {
		return nil, err
	}
	metadata := &msgs.LedgerMetadata{}
	if err := proto.Unmarshal(val, metadata); err != nil {
		logger.Errorf("Error unmarshalling ledger metadata: %s", err)
		return nil, errors.Wrapf(err, "error unmarshalling ledger metadata")
	}
	return metadata, nil
}

func (s *idStore) ledgerIDExists(ledgerID string) (bool, error) {
	key := metadataKey(ledgerID)
	val, err := s.db.Get(key)
	if err != nil {
		return false, err
	}
	return val != nil, nil
}

func (s *idStore) getActiveAndInactiveLedgerIDs() ([]string, error) {
	return s.getLedgerIDs(
		map[msgs.Status]struct{}{
			msgs.Status_ACTIVE:   {},
			msgs.Status_INACTIVE: {},
		},
	)
}

func (s *idStore) getActiveLedgerIDs() ([]string, error) {
	return s.getLedgerIDs(
		map[msgs.Status]struct{}{
			msgs.Status_ACTIVE: {},
		},
	)
}

func (s *idStore) getLedgerIDs(filterIn map[msgs.Status]struct{}) ([]string, error) {
	var ids []string
	itr := s.db.GetIterator(metadataKeyPrefix, metadataKeyStop)
	defer itr.Release()
	for itr.Error() == nil && itr.Next() {
		metadata := &msgs.LedgerMetadata{}
		if err := proto.Unmarshal(itr.Value(), metadata); err != nil {
			logger.Errorf("Error unmarshalling ledger metadata: %s", err)
			return nil, errors.Wrapf(err, "error unmarshalling ledger metadata")
		}
		if _, ok := filterIn[metadata.Status]; ok {
			id := ledgerIDFromMetadataKey(itr.Key())
			ids = append(ids, id)
		}
	}
	if err := itr.Error(); err != nil {
		logger.Errorf("Error getting ledger ids from idStore: %s", err)
		return nil, errors.Wrapf(err, "error getting ledger ids from idStore")
	}
	return ids, nil
}

func (s *idStore) close() {
	s.db.Close()
}

func ledgerIDFromGenesisBlockKey(key []byte) string {
	return string(key[len(genesisBlkKeyPrefix):])
}

func metadataKey(ledgerID string) []byte {
	return append(metadataKeyPrefix, []byte(ledgerID)...)
}

func ledgerIDFromMetadataKey(key []byte) string {
	return string(key[len(metadataKeyPrefix):])
}
