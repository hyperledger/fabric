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

package rwsetutil

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
)

func TestTxRWSetMarshalUnmarshal(t *testing.T) {
	txRwSet := &TxRwSet{}

	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "k0", EndKey: "k9", ItrExhausted: true}
	rqi1.SetRawReads([]*kvrwset.KVRead{
		&kvrwset.KVRead{Key: "k1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}},
		&kvrwset.KVRead{Key: "k2", Version: &kvrwset.Version{BlockNum: 1, TxNum: 2}},
	})

	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "k00", EndKey: "k90", ItrExhausted: true}
	rqi2.SetMerkelSummary(&kvrwset.QueryReadsMerkleSummary{MaxDegree: 5, MaxLevel: 4, MaxLevelHashes: [][]byte{[]byte("Hash-1"), []byte("Hash-2")}})

	txRwSet.NsRwSets = []*NsRwSet{
		&NsRwSet{"ns1", &kvrwset.KVRWSet{
			[]*kvrwset.KVRead{&kvrwset.KVRead{Key: "key1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
			[]*kvrwset.RangeQueryInfo{rqi1},
			[]*kvrwset.KVWrite{&kvrwset.KVWrite{Key: "key2", IsDelete: false, Value: []byte("value2")}},
		}},

		&NsRwSet{"ns2", &kvrwset.KVRWSet{
			[]*kvrwset.KVRead{&kvrwset.KVRead{Key: "key3", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
			[]*kvrwset.RangeQueryInfo{rqi2},
			[]*kvrwset.KVWrite{&kvrwset.KVWrite{Key: "key3", IsDelete: false, Value: []byte("value3")}},
		}},

		&NsRwSet{"ns3", &kvrwset.KVRWSet{
			[]*kvrwset.KVRead{&kvrwset.KVRead{Key: "key4", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
			nil,
			[]*kvrwset.KVWrite{&kvrwset.KVWrite{Key: "key4", IsDelete: false, Value: []byte("value4")}},
		}},
	}

	protoBytes, err := txRwSet.ToProtoBytes()
	testutil.AssertNoError(t, err, "")
	txRwSet1 := &TxRwSet{}
	testutil.AssertNoError(t, txRwSet1.FromProtoBytes(protoBytes), "")
	t.Logf("txRwSet=%s, txRwSet1=%s", spew.Sdump(txRwSet), spew.Sdump(txRwSet1))
	testutil.AssertEquals(t, txRwSet1, txRwSet)
}

func TestVersionConversion(t *testing.T) {
	protoVer := &kvrwset.Version{BlockNum: 5, TxNum: 2}
	internalVer := version.NewHeight(5, 2)
	// convert proto to internal
	testutil.AssertNil(t, NewVersion(nil))
	testutil.AssertEquals(t, NewVersion(protoVer), internalVer)

	// convert internal to proto
	testutil.AssertNil(t, newProtoVersion(nil))
	testutil.AssertEquals(t, newProtoVersion(internalVer), protoVer)
}
