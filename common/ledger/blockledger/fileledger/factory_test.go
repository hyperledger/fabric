/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fileledger

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
	"github.com/stretchr/testify/assert"
)

type mockBlockStoreProvider struct {
	blockstore blkstorage.BlockStore
	exists     bool
	list       []string
	error      error
}

func (mbsp *mockBlockStoreProvider) CreateBlockStore(ledgerid string) (blkstorage.BlockStore, error) {
	return mbsp.blockstore, mbsp.error
}

func (mbsp *mockBlockStoreProvider) OpenBlockStore(ledgerid string) (blkstorage.BlockStore, error) {
	return mbsp.blockstore, mbsp.error
}

func (mbsp *mockBlockStoreProvider) Exists(ledgerid string) (bool, error) {
	return mbsp.exists, mbsp.error
}

func (mbsp *mockBlockStoreProvider) List() ([]string, error) {
	return mbsp.list, mbsp.error
}

func (mbsp *mockBlockStoreProvider) Close() {
}

func TestBlockstoreProviderError(t *testing.T) {
	flf := &fileLedgerFactory{
		blkstorageProvider: &mockBlockStoreProvider{error: fmt.Errorf("blockstorage provider error")},
		ledgers:            make(map[string]blockledger.ReadWriter),
	}
	assert.Panics(
		t,
		func() { flf.ChainIDs() },
		"Expected ChainIDs to panic if storage provider cannot list chain IDs")

	_, err := flf.GetOrCreate("foo")
	assert.Error(t, err, "Expected GetOrCreate to return error if blockstorage provider cannot open")
	assert.Empty(t, flf.ledgers, "Expected no new ledger is created")
}

func TestMultiReinitialization(t *testing.T) {
	metricsProvider := &disabled.Provider{}

	dir, err := ioutil.TempDir("", "hyperledger_fabric")
	assert.NoError(t, err, "Error creating temp dir: %s", err)

	flf := New(dir, metricsProvider)
	_, err = flf.GetOrCreate(genesisconfig.TestChainID)
	assert.NoError(t, err, "Error GetOrCreate chain")
	assert.Equal(t, 1, len(flf.ChainIDs()), "Expected 1 chain")
	flf.Close()

	flf = New(dir, metricsProvider)
	_, err = flf.GetOrCreate("foo")
	assert.NoError(t, err, "Error creating chain")
	assert.Equal(t, 2, len(flf.ChainIDs()), "Expected chain to be recovered")
	flf.Close()

	flf = New(dir, metricsProvider)
	_, err = flf.GetOrCreate("bar")
	assert.NoError(t, err, "Error creating chain")
	assert.Equal(t, 3, len(flf.ChainIDs()), "Expected chain to be recovered")
	flf.Close()
}
