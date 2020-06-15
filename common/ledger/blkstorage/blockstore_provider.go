/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/dataformat"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("blkstorage")

// IndexableAttr represents an indexable attribute
type IndexableAttr string

// constants for indexable attributes
const (
	IndexableAttrBlockNum        = IndexableAttr("BlockNum")
	IndexableAttrBlockHash       = IndexableAttr("BlockHash")
	IndexableAttrTxID            = IndexableAttr("TxID")
	IndexableAttrBlockNumTranNum = IndexableAttr("BlockNumTranNum")
)

// IndexConfig - a configuration that includes a list of attributes that should be indexed
type IndexConfig struct {
	AttrsToIndex []IndexableAttr
}

// Contains returns true iff the supplied parameter is present in the IndexConfig.AttrsToIndex
func (c *IndexConfig) Contains(indexableAttr IndexableAttr) bool {
	for _, a := range c.AttrsToIndex {
		if a == indexableAttr {
			return true
		}
	}
	return false
}

var (
	// ErrNotFoundInIndex is used to indicate missing entry in the index
	ErrNotFoundInIndex = ledger.NotFoundInIndexErr("")

	// ErrAttrNotIndexed is used to indicate that an attribute is not indexed
	ErrAttrNotIndexed = errors.New("attribute not indexed")
)

// BlockStoreProvider provides handle to block storage - this is not thread-safe
type BlockStoreProvider struct {
	conf            *Conf
	indexConfig     *IndexConfig
	leveldbProvider *leveldbhelper.Provider
	stats           *stats
	mutex           sync.RWMutex
}

// NewProvider constructs a filesystem based block store provider
func NewProvider(conf *Conf, indexConfig *IndexConfig, metricsProvider metrics.Provider) (*BlockStoreProvider, error) {
	dbConf := &leveldbhelper.Conf{
		DBPath:         conf.getIndexDir(),
		ExpectedFormat: dataFormatVersion(indexConfig),
	}

	p, err := leveldbhelper.NewProvider(dbConf)
	if err != nil {
		return nil, err
	}

	dirPath := conf.getChainsDir()
	if _, err := os.Stat(dirPath); err != nil {
		if !os.IsNotExist(err) { // NotExist is the only permitted error type
			return nil, errors.Wrapf(err, "failed to read ledger directory %s", dirPath)
		}

		logger.Info("Creating new file ledger directory at", dirPath)
		if err = os.MkdirAll(dirPath, 0755); err != nil {
			return nil, errors.Wrapf(err, "failed to create ledger directory: %s", dirPath)
		}
	}

	stats := newStats(metricsProvider)
	provider := &BlockStoreProvider{
		conf:            conf,
		indexConfig:     indexConfig,
		leveldbProvider: p,
		stats:           stats,
	}

	go provider.completePendingRemoves()
	return provider, nil
}

// Open opens a block store for given ledgerid.
// If a blockstore is not existing, this method creates one
// This method should be invoked only once for a particular ledgerid
func (p *BlockStoreProvider) Open(ledgerid string) (*BlockStore, error) {
	indexStoreHandle := p.leveldbProvider.GetDBHandle(ledgerid)
	return newBlockStore(ledgerid, p.conf, p.indexConfig, indexStoreHandle, p.stats), nil
}

// Exists tells whether the BlockStore with given id exists
func (p *BlockStoreProvider) Exists(ledgerid string) (bool, error) {
	exists, _, err := util.FileExists(p.conf.getLedgerBlockDir(ledgerid))
	if !exists || err != nil {
		return false, err
	}
	toBeRemoved, _, err := util.FileExists(p.conf.getToBeRemovedFilePath(ledgerid))
	if err != nil {
		return false, err
	}
	return !toBeRemoved, nil
}

// List lists the ids of the existing ledgers
// A channel is filtered out if it has a temporary __toBeRemoved_ file.
func (p *BlockStoreProvider) List() ([]string, error) {
	subdirs, err := util.ListSubdirs(p.conf.getChainsDir())
	if err != nil {
		return nil, err
	}
	channelNames := []string{}
	for _, subdir := range subdirs {
		toBeRemoved, _, err := util.FileExists(p.conf.getToBeRemovedFilePath(subdir))
		if err != nil {
			return nil, err
		}
		if !toBeRemoved {
			channelNames = append(channelNames, subdir)
		}
	}
	return channelNames, nil
}

// Remove block index and blocks for the given ledgerid (channelID).
// It creates a temporary file to indicate the channel is to be removed and deletes the ledger data in a separate goroutine.
// If the channel does not exist (or the channel is already marked to be removed), it is not an error.
func (p *BlockStoreProvider) Remove(ledgerid string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	exists, err := p.Exists(ledgerid)
	if !exists || err != nil {
		return err
	}

	f, err := os.Create(p.conf.getToBeRemovedFilePath(ledgerid))
	if err != nil {
		return err
	}
	f.Close()

	go p.removeLedgerData(ledgerid)

	return nil
}

// completePendingRemoves checks __toBeRemoved_xxx files and removes the corresponding channel ledger data
// if any temporary file(s) is found. This function should only be called upon ledger init.
func (p *BlockStoreProvider) completePendingRemoves() {
	files, err := ioutil.ReadDir(p.conf.blockStorageDir)
	if err != nil {
		logger.Errorf("Error reading dir %s, error: %s", p.conf.blockStorageDir, err)
		return
	}
	for _, f := range files {
		fileName := f.Name()
		if !f.IsDir() && strings.HasPrefix(fileName, toBeRemovedFilePrefix) {
			p.removeLedgerData(fileName[len(toBeRemovedFilePrefix):])
		}
	}
}

func (p *BlockStoreProvider) removeLedgerData(ledgerid string) error {
	logger.Infof("Removing block data for channel %s", ledgerid)
	if err := p.leveldbProvider.Remove(ledgerid); err != nil {
		logger.Errorf("Failed to remove block index for channel %s, error: %s", ledgerid, err)
		return err
	}
	if err := os.RemoveAll(p.conf.getLedgerBlockDir(ledgerid)); err != nil {
		logger.Errorf("Failed to remove blocks for channel %s, error: %s", ledgerid, err)
		return err
	}
	tempFile := p.conf.getToBeRemovedFilePath(ledgerid)
	if err := os.Remove(tempFile); err != nil {
		logger.Errorf("Failed to remove temporary file %s for channel %s", tempFile, ledgerid)
		return err
	}
	logger.Infof("Successfully removed block data for channel %s", ledgerid)
	return nil
}

// Close closes the BlockStoreProvider
func (p *BlockStoreProvider) Close() {
	p.leveldbProvider.Close()
}

func dataFormatVersion(indexConfig *IndexConfig) string {
	// in version 2.0 we merged three indexable into one `IndexableAttrTxID`
	if indexConfig.Contains(IndexableAttrTxID) {
		return dataformat.CurrentFormat
	}
	return dataformat.PreviousFormat
}
