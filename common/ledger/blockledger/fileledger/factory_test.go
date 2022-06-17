/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fileledger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger/mock"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/orderer/common/filerepo"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mock/file_ledger_block_store.go --fake-name FileLedgerBlockStore . fileLedgerBlockStore

type fileLedgerBlockStore interface {
	FileLedgerBlockStore
}

func TestBlockStoreProviderErrors(t *testing.T) {
	setup := func(fileRepo *filerepo.Repo) (*fileLedgerFactory, *mock.BlockStoreProvider) {
		m := &mock.BlockStoreProvider{}

		f := &fileLedgerFactory{
			blkstorageProvider: m,
			ledgers:            map[string]*FileLedger{},
			removeFileRepo:     fileRepo,
		}
		return f, m
	}

	t.Run("list", func(t *testing.T) {
		f, mockBlockStoreProvider := setup(nil)
		mockBlockStoreProvider.ListReturns(nil, errors.New("boogie"))
		require.PanicsWithValue(
			t,
			"boogie",
			func() { f.ChannelIDs() },
			"Expected ChannelIDs to panic if storage provider cannot list channel IDs",
		)
	})

	t.Run("open", func(t *testing.T) {
		f, mockBlockStoreProvider := setup(nil)
		mockBlockStoreProvider.OpenReturns(nil, errors.New("woogie"))
		_, err := f.GetOrCreate("foo")
		require.EqualError(t, err, "woogie")
		require.Empty(t, f.ledgers, "Expected no new ledger is created")
	})

	t.Run("remove", func(t *testing.T) {
		dir := t.TempDir()
		fileRepo, err := filerepo.New(filepath.Join(dir, "pendingops"), "remove")
		require.NoError(t, err, "Error creating temp file repo: %s", err)

		t.Run("ledger doesn't exist", func(t *testing.T) {
			f, mockBlockStoreProvider := setup(fileRepo)
			err := f.Remove("foo")
			require.NoError(t, err)
			require.Equal(t, 1, mockBlockStoreProvider.DropCallCount())
			channelID := mockBlockStoreProvider.DropArgsForCall(0)
			require.Equal(t, "foo", channelID)
		})

		t.Run("dropping the blockstore fails", func(t *testing.T) {
			f, mockBlockStoreProvider := setup(fileRepo)
			mockBlockStore := &mock.FileLedgerBlockStore{}
			f.ledgers["bar"] = &FileLedger{blockStore: mockBlockStore}
			mockBlockStoreProvider.DropReturns(errors.New("oogie"))

			err := f.Remove("bar")
			require.EqualError(t, err, "oogie")
			require.Equal(t, 1, mockBlockStore.ShutdownCallCount())
			require.Equal(t, 1, mockBlockStoreProvider.DropCallCount())
			channelID := mockBlockStoreProvider.DropArgsForCall(0)
			require.Equal(t, "bar", channelID)
		})
	})
}

func TestMultiReinitialization(t *testing.T) {
	metricsProvider := &disabled.Provider{}

	dir := t.TempDir()

	f, err := New(dir, metricsProvider)
	require.NoError(t, err)
	_, err = f.GetOrCreate("testchannelid")
	require.NoError(t, err, "Error GetOrCreate channel")
	require.Equal(t, 1, len(f.ChannelIDs()), "Expected 1 channel")
	f.Close()

	f, err = New(dir, metricsProvider)
	require.NoError(t, err)
	_, err = f.GetOrCreate("foo")
	require.NoError(t, err, "Error creating channel")
	require.Equal(t, 2, len(f.ChannelIDs()), "Expected channel to be recovered")
	f.Close()

	f, err = New(dir, metricsProvider)
	require.NoError(t, err)
	_, err = f.GetOrCreate("bar")
	require.NoError(t, err, "Error creating channel")
	require.Equal(t, 3, len(f.ChannelIDs()), "Expected channel to be recovered")
	f.Close()

	bar2FileRepoDir := filepath.Join(dir, "pendingops", "remove", "bar2.remove")
	_, err = os.Create(bar2FileRepoDir)
	require.NoError(t, err, "Error creating temp file: %s", err)

	bar2ChainsDir := filepath.Join(dir, "chains", "bar2")
	err = os.MkdirAll(bar2ChainsDir, 0o700)
	require.NoError(t, err, "Error creating temp dir: %s", err)
	_, err = os.Create(filepath.Join(bar2ChainsDir, "blockfile_000000"))
	require.NoError(t, err, "Error creating temp file: %s", err)

	f, err = New(dir, metricsProvider)
	require.NoError(t, err)

	err = f.Remove("bar")
	require.NoError(t, err, "Error removing channel")
	require.Equal(t, 2, len(f.ChannelIDs()))
	err = f.Remove("this-isnt-an-existing-channel")
	require.NoError(t, err, "Error removing channel")
	require.Equal(t, 2, len(f.ChannelIDs()))

	_, err = os.Stat(bar2ChainsDir)
	require.EqualError(t, err, fmt.Sprintf("stat %s: no such file or directory", bar2ChainsDir))

	_, err = os.Stat(bar2FileRepoDir)
	require.EqualError(t, err, fmt.Sprintf("stat %s: no such file or directory", bar2FileRepoDir))
	f.Close()
}

func TestNewErrors(t *testing.T) {
	metricsProvider := &disabled.Provider{}

	t.Run("creation of filerepo fails", func(t *testing.T) {
		dir := t.TempDir()

		fileRepoDir := filepath.Join(dir, "pendingops", "remove")
		err := os.MkdirAll(fileRepoDir, 0o700)
		require.NoError(t, err, "Error creating temp dir: %s", err)
		removeFile := filepath.Join(fileRepoDir, "rojo.remove")
		_, err = os.Create(removeFile)
		require.NoError(t, err, "Error creating temp file: %s", err)

		err = os.Chmod(removeFile, 0o444)
		require.NoError(t, err, "Error changing permissions of temp file: %s", err)

		err = os.Chmod(filepath.Join(dir, "pendingops", "remove"), 0o444)
		require.NoError(t, err, "Error changing permissions of temp file: %s", err)
		t.Cleanup(func() {
			require.NoError(t, os.Chmod(filepath.Join(dir, "pendingops", "remove"), 0o777))
		})

		_, err = New(dir, metricsProvider)
		require.EqualError(t, err, fmt.Sprintf("error checking if dir [%s] is empty: lstat %s: permission denied", fileRepoDir, removeFile))
	})

	t.Run("removal fails", func(t *testing.T) {
		dir := t.TempDir()

		fileRepoDir := filepath.Join(dir, "pendingops", "remove")
		err := os.MkdirAll(fileRepoDir, 0o777)
		require.NoError(t, err, "Error creating temp dir: %s", err)
		removeFile := filepath.Join(fileRepoDir, "rojo.remove")
		_, err = os.Create(removeFile)
		require.NoError(t, err, "Error creating temp file: %s", err)
		err = os.Chmod(removeFile, 0o444)
		require.NoError(t, err, "Error changing permissions of temp file: %s", err)
		err = os.Chmod(filepath.Join(dir, "pendingops", "remove"), 0o544)
		require.NoError(t, err, "Error changing permissions of temp file: %s", err)
		t.Cleanup(func() {
			require.NoError(t, os.Chmod(filepath.Join(dir, "pendingops", "remove"), 0o777))
		})

		_, err = New(dir, metricsProvider)
		require.EqualError(t, err, fmt.Sprintf("unlinkat %s: permission denied", removeFile))
	})
}

func TestRemove(t *testing.T) {
	mockBlockStore := &mock.BlockStoreProvider{}
	dir := t.TempDir()

	fileRepo, err := filerepo.New(filepath.Join(dir, "pendingops"), "remove")
	require.NoError(t, err, "Error creating temp file repo: %s", err)
	f := &fileLedgerFactory{
		blkstorageProvider: mockBlockStore,
		ledgers:            map[string]*FileLedger{},
		removeFileRepo:     fileRepo,
	}
	defer f.Close()

	t.Run("success", func(t *testing.T) {
		dest := filepath.Join(dir, "pendingops", "remove", "foo.remove")
		mockBlockStore.DropCalls(func(string) error {
			_, err = os.Stat(dest)
			require.NoError(t, err, "Expected foo.remove to exist")
			return nil
		})
		err = f.Remove("foo")
		require.NoError(t, err, "Error removing channel")
		require.Equal(t, 1, mockBlockStore.DropCallCount(), "Expected 1 Drop() calls")

		_, err = os.Stat(dest)
		require.EqualError(t, err, fmt.Sprintf("stat %s: no such file or directory", dest))
	})

	t.Run("drop fails", func(t *testing.T) {
		mockBlockStore.DropReturns(errors.New("oogie"))
		err = f.Remove("foo")
		require.EqualError(t, err, "oogie")

		dest := filepath.Join(dir, "pendingops", "remove", "foo.remove")
		_, err = os.Stat(dest)
		require.NoError(t, err, "Expected foo.remove to exist")
	})

	t.Run("saving to pending ops fails", func(t *testing.T) {
		os.RemoveAll(dir)
		mockBlockStore.DropReturns(nil)
		err = f.Remove("foo")
		require.EqualError(t, err, fmt.Sprintf("error while creating file:%s/pendingops/remove/foo.remove~: open %s/pendingops/remove/foo.remove~: no such file or directory", dir, dir))
	})
}
