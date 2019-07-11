/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package fileledger

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
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
