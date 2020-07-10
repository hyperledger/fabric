/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fileledger

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/stretchr/testify/require"
)

type mockBlockStoreProvider struct {
	blockstore *blkstorage.BlockStore
	list       []string
	error      error
}

func (mbsp *mockBlockStoreProvider) Open(ledgerid string) (*blkstorage.BlockStore, error) {
	return mbsp.blockstore, mbsp.error
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
	require.Panics(
		t,
		func() { flf.ChannelIDs() },
		"Expected ChannelIDs to panic if storage provider cannot list channel IDs")

	_, err := flf.GetOrCreate("foo")
	require.Error(t, err, "Expected GetOrCreate to return error if blockstorage provider cannot open")
	require.Empty(t, flf.ledgers, "Expected no new ledger is created")
}

func TestMultiReinitialization(t *testing.T) {
	metricsProvider := &disabled.Provider{}

	dir, err := ioutil.TempDir("", "hyperledger_fabric")
	require.NoError(t, err, "Error creating temp dir: %s", err)
	defer os.RemoveAll(dir)

	flf, err := New(dir, metricsProvider)
	require.NoError(t, err)
	_, err = flf.GetOrCreate("testchannelid")
	require.NoError(t, err, "Error GetOrCreate channel")
	require.Equal(t, 1, len(flf.ChannelIDs()), "Expected 1 channel")
	flf.Close()

	flf, err = New(dir, metricsProvider)
	require.NoError(t, err)
	_, err = flf.GetOrCreate("foo")
	require.NoError(t, err, "Error creating channel")
	require.Equal(t, 2, len(flf.ChannelIDs()), "Expected channel to be recovered")
	flf.Close()

	flf, err = New(dir, metricsProvider)
	require.NoError(t, err)
	_, err = flf.GetOrCreate("bar")
	require.NoError(t, err, "Error creating channel")
	require.Equal(t, 3, len(flf.ChannelIDs()), "Expected channel to be recovered")
	flf.Close()
}
