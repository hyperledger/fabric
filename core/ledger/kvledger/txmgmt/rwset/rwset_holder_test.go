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

package rwset

import (
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

func TestRWSetHolder(t *testing.T) {
	rwSet := NewRWSet()

	rwSet.AddToReadSet("ns1", "key2", version.NewHeight(1, 2))
	rwSet.AddToReadSet("ns1", "key1", version.NewHeight(1, 1))
	rwSet.AddToWriteSet("ns1", "key2", []byte("value2"))

	rqi1 := &RangeQueryInfo{"bKey", "", false, nil, nil}
	rqi1.EndKey = "eKey"
	rqi1.results = []*KVRead{NewKVRead("bKey1", version.NewHeight(2, 3)), NewKVRead("bKey2", version.NewHeight(2, 4))}
	rqi1.ItrExhausted = true
	rwSet.AddToRangeQuerySet("ns1", rqi1)

	rqi2 := &RangeQueryInfo{"bKey", "", false, nil, nil}
	rqi2.EndKey = "eKey"
	rqi2.results = []*KVRead{NewKVRead("bKey1", version.NewHeight(2, 3)), NewKVRead("bKey2", version.NewHeight(2, 4))}
	rqi2.ItrExhausted = true
	rwSet.AddToRangeQuerySet("ns1", rqi2)

	rqi3 := &RangeQueryInfo{"bKey", "", true, nil, nil}
	rwSet.AddToRangeQuerySet("ns1", rqi3)
	rqi3.EndKey = "eKey1"
	rqi3.results = []*KVRead{NewKVRead("bKey1", version.NewHeight(2, 3)), NewKVRead("bKey2", version.NewHeight(2, 4))}

	rwSet.AddToReadSet("ns2", "key2", version.NewHeight(1, 2))
	rwSet.AddToWriteSet("ns2", "key3", []byte("value3"))

	txRWSet := rwSet.GetTxReadWriteSet()

	ns1RWSet := &NsReadWriteSet{"ns1",
		[]*KVRead{&KVRead{"key1", version.NewHeight(1, 1)}, &KVRead{"key2", version.NewHeight(1, 2)}},
		[]*KVWrite{&KVWrite{"key2", false, []byte("value2")}},
		[]*RangeQueryInfo{rqi1, rqi3}}

	ns2RWSet := &NsReadWriteSet{"ns2",
		[]*KVRead{&KVRead{"key2", version.NewHeight(1, 2)}},
		[]*KVWrite{&KVWrite{"key3", false, []byte("value3")}},
		[]*RangeQueryInfo{}}

	expectedTxRWSet := &TxReadWriteSet{[]*NsReadWriteSet{ns1RWSet, ns2RWSet}}
	t.Logf("Actual=%s\n Expected=%s", txRWSet, expectedTxRWSet)
	testutil.AssertEquals(t, txRWSet, expectedTxRWSet)
}
