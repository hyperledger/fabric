/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package txmgr

import (
	"strconv"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/stretchr/testify/require"
)

func BenchmarkTxmgrTest(b *testing.B) {
	b.ReportAllocs()
	ledgerid := "TestTxSimulatorBenchmark"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns", "coll"}: 1,
		},
	)
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(b, ledgerid, btlPolicy)
	defer testEnv.cleanup()
	txMgr := testEnv.getTxMgr()
	populateCollConfigForTest(txMgr,
		[]collConfigkey{
			{"ns", "coll"},
		},
		version.NewHeight(1, 1),
	)
	T := &testing.T{}
	bg, _ := testutil.NewBlockGenerator(T, ledgerid, false)
	n := 0
	b.ResetTimer()
	for n < b.N {
		blkAndPvtdata := prepareNextBlockForTest(T, txMgr, bg, "txid-"+strconv.Itoa(n),
			map[string]string{"pubkey" + strconv.Itoa(n): "pub-value" + strconv.Itoa(n)},
			map[string]string{"pvtkey" + strconv.Itoa(n): "pvt-value" + strconv.Itoa(n)},
			true)
		_, _, err := txMgr.ValidateAndPrepare(blkAndPvtdata, true)
		require.NoError(b, err)
		require.NoError(b, txMgr.Commit())
		n++
	}
	b.StopTimer()
}
