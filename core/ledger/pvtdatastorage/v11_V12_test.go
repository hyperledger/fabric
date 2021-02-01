/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/stretchr/testify/require"
)

// TestV11v12 test that we are able to read the mixed format data (for v11 and v12)
// from pvtdata store. This test used a pvt data store that is produced in one of the
// upgrade tests. The store contains total 15 blocks. Block number one to nine has not
// pvt data because, that time peer code was v1.0 and hence no pvt data. Block 10 contains
// a pvtdata from peer v1.1. Block 11 - 13 has not pvt data. Block 14 has pvt data from peer v1.2
func TestV11v12(t *testing.T) {
	testWorkingDir, err := ioutil.TempDir("", "pdstore")
	if err != nil {
		t.Fatalf("Failed to create private data storage directory: %s", err)
	}
	defer os.RemoveAll(testWorkingDir)
	require.NoError(t, testutil.CopyDir("testdata/v11_v12/ledgersData/pvtdataStore", testWorkingDir, false))

	ledgerid := "ch1"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"marbles_private", "collectionMarbles"}:              0,
			{"marbles_private", "collectionMarblePrivateDetails"}: 0,
		},
	)
	conf := &PrivateDataConfig{
		PrivateDataConfig: &ledger.PrivateDataConfig{
			BatchesInterval: 1000,
			MaxBatchSize:    5000,
			PurgeInterval:   100,
		},
		StorePath: filepath.Join(testWorkingDir, "pvtdataStore"),
	}
	p, err := NewProvider(conf)
	require.NoError(t, err)
	defer p.Close()
	s, err := p.OpenStore(ledgerid)
	require.NoError(t, err)
	s.Init(btlPolicy)

	for blk := 0; blk < 10; blk++ {
		checkDataNotExists(t, s, blk)
	}
	checkDataExists(t, s, 10)
	for blk := 11; blk < 14; blk++ {
		checkDataNotExists(t, s, blk)
	}
	checkDataExists(t, s, 14)

	_, err = s.GetPvtDataByBlockNum(uint64(15), nil)
	require.EqualError(t, err, "last committed block number [14] smaller than the requested block number [15]")
}

func checkDataNotExists(t *testing.T, s *Store, blkNum int) {
	data, err := s.GetPvtDataByBlockNum(uint64(blkNum), nil)
	require.NoError(t, err)
	require.Nil(t, data)
}

func checkDataExists(t *testing.T, s *Store, blkNum int) {
	data, err := s.GetPvtDataByBlockNum(uint64(blkNum), nil)
	require.NoError(t, err)
	require.NotNil(t, data)
	t.Logf("pvtdata = %s\n", spew.Sdump(data))
}
