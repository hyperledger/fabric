/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package confighistorytest

import (
	"fmt"
	"math"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/stretchr/testify/require"
)

func TestConfigHistory(t *testing.T) {
	testDir := t.TempDir()

	mgr, err := NewMgr(testDir)
	require.NoError(t, err)
	defer mgr.Close()

	sampleConfigHistoryNS1 := map[uint64][]*peer.StaticCollectionConfig{
		300: {{Name: "coll1"}, {Name: "coll2"}, {Name: "coll3"}},
		20:  {{Name: "coll1"}, {Name: "coll2"}},
	}

	sampleConfigHistoryNS2 := map[uint64][]*peer.StaticCollectionConfig{
		400: {{Name: "coll4"}},
	}

	require.NoError(t, mgr.Setup("ledger-1", "ns1", sampleConfigHistoryNS1))
	require.NoError(t, mgr.Setup("ledger-1", "ns2", sampleConfigHistoryNS2))

	r := mgr.GetRetriever("ledger-1")

	testcases := []struct {
		inputNS     string
		inputBlkNum uint64

		outputConfig *peer.CollectionConfigPackage
		outputBlkNum uint64
	}{
		{
			inputNS:      "ns1",
			inputBlkNum:  math.MaxUint64,
			outputConfig: BuildCollConfigPkg(sampleConfigHistoryNS1[300]),
			outputBlkNum: 300,
		},

		{
			inputNS:      "ns1",
			inputBlkNum:  300,
			outputConfig: BuildCollConfigPkg(sampleConfigHistoryNS1[20]),
			outputBlkNum: 20,
		},

		{
			inputNS:      "ns1",
			inputBlkNum:  20,
			outputConfig: nil,
			outputBlkNum: 0,
		},

		{
			inputNS:      "ns2",
			inputBlkNum:  math.MaxUint64,
			outputConfig: BuildCollConfigPkg(sampleConfigHistoryNS2[400]),
			outputBlkNum: 400,
		},

		{
			inputNS:      "ns2",
			inputBlkNum:  200,
			outputConfig: nil,
			outputBlkNum: 0,
		},
	}

	for i, c := range testcases {
		t.Run(fmt.Sprintf("testcase-%d", i), func(t *testing.T) {
			collectionConfigInfo, err := r.MostRecentCollectionConfigBelow(c.inputBlkNum, c.inputNS)
			require.NoError(t, err)

			if c.outputConfig == nil {
				require.Nil(t, c.outputConfig)
				require.Equal(t, uint64(0), c.outputBlkNum)
				return
			}

			require.Equal(t, c.outputBlkNum, collectionConfigInfo.CommittingBlockNum)
			require.True(t,
				proto.Equal(
					collectionConfigInfo.CollectionConfig,
					c.outputConfig,
				),
			)
		})
	}
}
