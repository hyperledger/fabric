/*
Copyright the Hyperledger Fabric contributors. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/stretchr/testify/require"
)

func TestConsenterRemoveChannelData(t *testing.T) {
	newConsenter := func(t *testing.T) (*smartbft.Consenter, string) {
		walBaseDir := t.TempDir()
		for _, channel := range []string{"mychannel", "otherchannel"} {
			require.NoError(t, os.MkdirAll(filepath.Join(walBaseDir, channel), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(walBaseDir, channel, "data"), []byte("data"), 0o644))
		}
		return &smartbft.Consenter{WALBaseDir: walBaseDir}, walBaseDir
	}

	t.Run("removes the WAL directory of that channel only", func(t *testing.T) {
		c, walBaseDir := newConsenter(t)
		require.NoError(t, c.RemoveChannelData("mychannel"))
		require.NoDirExists(t, filepath.Join(walBaseDir, "mychannel"))
		require.FileExists(t, filepath.Join(walBaseDir, "otherchannel", "data"))
	})

	t.Run("succeeds when the channel has no data", func(t *testing.T) {
		c, _ := newConsenter(t)
		require.NoError(t, c.RemoveChannelData("unknownchannel"))
	})

	t.Run("rejects an empty channel ID", func(t *testing.T) {
		c, walBaseDir := newConsenter(t)
		require.EqualError(t, c.RemoveChannelData(""), "empty channel ID")
		require.FileExists(t, filepath.Join(walBaseDir, "mychannel", "data"))
		require.FileExists(t, filepath.Join(walBaseDir, "otherchannel", "data"))
	})

	t.Run("does nothing without a WAL directory", func(t *testing.T) {
		workDir := t.TempDir()
		t.Chdir(workDir)
		require.NoError(t, os.Mkdir("mychannel", 0o755))

		require.NoError(t, (&smartbft.Consenter{}).RemoveChannelData("mychannel"))
		require.DirExists(t, filepath.Join(workDir, "mychannel"))
	})
}
