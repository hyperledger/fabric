/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fileledger

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/orderer/common/filerepo"
)

//go:generate counterfeiter -o mock/block_store_provider.go --fake-name BlockStoreProvider . blockStoreProvider
type blockStoreProvider interface {
	Open(ledgerid string) (*blkstorage.BlockStore, error)
	Drop(ledgerid string) error
	List() ([]string, error)
	Close()
}

type fileLedgerFactory struct {
	blkstorageProvider blockStoreProvider
	ledgers            map[string]*FileLedger
	mutex              sync.Mutex
	removeFileRepo     *filerepo.Repo
}

// GetOrCreate gets an existing ledger (if it exists) or creates it
// if it does not.
func (f *fileLedgerFactory) GetOrCreate(channelID string) (blockledger.ReadWriter, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// check cache
	ledger, ok := f.ledgers[channelID]
	if ok {
		return ledger, nil
	}
	// open fresh
	blockStore, err := f.blkstorageProvider.Open(channelID)
	if err != nil {
		return nil, err
	}
	ledger = NewFileLedger(blockStore)
	f.ledgers[channelID] = ledger
	return ledger, nil
}

// Remove removes an existing ledger and its indexes. This operation
// is blocking.
func (f *fileLedgerFactory) Remove(channelID string) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if err := f.removeFileRepo.Save(channelID, []byte{}); err != nil && err != os.ErrExist {
		return err
	}

	// check cache for open blockstore and, if one exists,
	// shut it down in order to avoid resource contention
	ledger, ok := f.ledgers[channelID]
	if ok {
		ledger.blockStore.Shutdown()
	}

	err := f.blkstorageProvider.Drop(channelID)
	if err != nil {
		return err
	}

	delete(f.ledgers, channelID)

	if err := f.removeFileRepo.Remove(channelID); err != nil {
		return err
	}

	return nil
}

// ChannelIDs returns the channel IDs the factory is aware of.
func (f *fileLedgerFactory) ChannelIDs() []string {
	channelIDs, err := f.blkstorageProvider.List()
	if err != nil {
		logger.Panic(err)
	}
	return channelIDs
}

// Close releases all resources acquired by the factory.
func (f *fileLedgerFactory) Close() {
	f.blkstorageProvider.Close()
}

// New creates a new ledger factory
func New(directory string, metricsProvider metrics.Provider) (blockledger.Factory, error) {
	p, err := blkstorage.NewProvider(
		blkstorage.NewConf(directory, -1),
		&blkstorage.IndexConfig{
			AttrsToIndex: []blkstorage.IndexableAttr{blkstorage.IndexableAttrBlockNum},
		},
		metricsProvider,
	)
	if err != nil {
		return nil, err
	}

	fileRepo, err := filerepo.New(filepath.Join(directory, "pendingops"), "remove")
	if err != nil {
		return nil, err
	}

	factory := &fileLedgerFactory{
		blkstorageProvider: p,
		ledgers:            map[string]*FileLedger{},
		removeFileRepo:     fileRepo,
	}

	files, err := factory.removeFileRepo.List()
	if err != nil {
		return nil, err
	}
	for _, fileName := range files {
		channelID := factory.removeFileRepo.FileToBaseName(fileName)
		err = factory.Remove(channelID)
		if err != nil {
			logger.Errorf("Failed to remove channel %s: %s", channelID, err.Error())
			return nil, err
		}
		logger.Infof("Removed channel: %s", channelID)
	}

	return factory, nil
}
