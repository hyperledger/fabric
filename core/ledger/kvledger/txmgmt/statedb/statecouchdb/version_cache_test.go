/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/stretchr/testify/require"
)

func TestVersionCache(t *testing.T) {
	verCache := newVersionCache()
	ver1 := version.NewHeight(1, 1)
	ver2 := version.NewHeight(2, 2)
	verCache.setVerAndRev("ns1", "key1", version.NewHeight(1, 1), "rev1")
	verCache.setVerAndRev("ns2", "key2", version.NewHeight(2, 2), "rev2")

	ver, found := verCache.getVersion("ns1", "key1")
	require.True(t, found)
	require.Equal(t, ver1, ver)

	ver, found = verCache.getVersion("ns2", "key2")
	require.True(t, found)
	require.Equal(t, ver2, ver)

	ver, found = verCache.getVersion("ns1", "key3")
	require.False(t, found)
	require.Nil(t, ver)

	ver, found = verCache.getVersion("ns3", "key4")
	require.False(t, found)
	require.Nil(t, ver)
}
