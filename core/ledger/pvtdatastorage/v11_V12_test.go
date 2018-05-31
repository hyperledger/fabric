/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestV11v12 test that we are able to read the mixed format data (for v11 and v12)
// from pvtdata store. This test used a pvt data store that is produced in one of the
// upgrade tests. The store contains total 15 blocks. Block number one to nine has not
// pvt data because, that time peer code was v1.0 and hence no pvt data. Block 10 contains
// a pvtdata from peer v1.1. Block 11 - 13 has not pvt data. Block 14 has pvt data from peer v1.2
func TestV11v12(t *testing.T) {
	testWorkingDir := "test-working-dir"
	testutil.CopyDir("testdata/v11_v12/ledgersData", testWorkingDir)
	defer os.RemoveAll(testWorkingDir)

	viper.Set("peer.fileSystemPath", testWorkingDir)
	defer viper.Reset()

	ledgerid := "ch1"
	cs := btltestutil.NewMockCollectionStore()
	cs.SetBTL("marbles_private", "collectionMarbles", 0)
	cs.SetBTL("marbles_private", "collectionMarblePrivateDetails", 0)
	btlPolicy := pvtdatapolicy.ConstructBTLPolicy(cs)

	p := NewProvider()
	defer p.Close()
	s, err := p.OpenStore(ledgerid)
	assert.NoError(t, err)
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
	_, ok := err.(*ErrOutOfRange)
	assert.True(t, ok)
}

func checkDataNotExists(t *testing.T, s Store, blkNum int) {
	data, err := s.GetPvtDataByBlockNum(uint64(blkNum), nil)
	assert.NoError(t, err)
	assert.Nil(t, data)
}

func checkDataExists(t *testing.T, s Store, blkNum int) {
	data, err := s.GetPvtDataByBlockNum(uint64(blkNum), nil)
	assert.NoError(t, err)
	assert.NotNil(t, data)
	t.Logf("pvtdata = %s\n", spew.Sdump(data))
}
