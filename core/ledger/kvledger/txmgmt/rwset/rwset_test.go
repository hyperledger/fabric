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
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

func TestNilTxRWSet(t *testing.T) {
	txRW := &TxReadWriteSet{}
	nsRW1 := &NsReadWriteSet{"ns1",
		[]*KVRead{&KVRead{"key1", nil}},
		[]*KVWrite{&KVWrite{"key1", false, []byte("value1")}},
		nil}
	txRW.NsRWs = append(txRW.NsRWs, nsRW1)
	b, err := txRW.Marshal()
	testutil.AssertNoError(t, err, "Error while marshalling changeset")

	deserializedRWSet := &TxReadWriteSet{}
	err = deserializedRWSet.Unmarshal(b)
	testutil.AssertNoError(t, err, "Error while unmarshalling changeset")
	t.Logf("Unmarshalled changeset = %#+v", deserializedRWSet.NsRWs[0].Writes[0].IsDelete)
	testutil.AssertEquals(t, deserializedRWSet, txRW)
}

func TestTxRWSetMarshalUnmarshal(t *testing.T) {
	txRW := &TxReadWriteSet{}
	nsRW1 := &NsReadWriteSet{"ns1",
		[]*KVRead{&KVRead{"key1", version.NewHeight(1, 1)}},
		[]*KVWrite{&KVWrite{"key2", false, []byte("value2")}},
		nil}

	nsRW2 := &NsReadWriteSet{"ns2",
		[]*KVRead{&KVRead{"key3", version.NewHeight(1, 2)}},
		[]*KVWrite{&KVWrite{"key4", true, nil}},
		nil}

	nsRW3 := &NsReadWriteSet{"ns3",
		[]*KVRead{&KVRead{"key5", version.NewHeight(1, 3)}},
		[]*KVWrite{&KVWrite{"key6", false, []byte("value6")}, &KVWrite{"key7", false, []byte("value7")}},
		nil}

	nsRW4 := &NsReadWriteSet{"ns4",
		[]*KVRead{&KVRead{"key8", version.NewHeight(1, 3)}},
		[]*KVWrite{&KVWrite{"key9", false, []byte("value9")}, &KVWrite{"key10", false, []byte("value10")}},
		[]*RangeQueryInfo{&RangeQueryInfo{"startKey1", "endKey1", true, nil, testutil.ConstructRandomBytes(t, 10)}}}

	buf := proto.NewBuffer(nil)
	rqInfo := &RangeQueryInfo{"startKey2", "endKey2", false, []*KVRead{&KVRead{"key11", version.NewHeight(1, 3)}}, nil}
	rqInfo.Marshal(buf)

	rqInfo1 := &RangeQueryInfo{}
	rqInfo1.Unmarshal(buf)
	fmt.Printf("rqInfo=%#v\n", rqInfo)
	fmt.Printf("rqInfo1=%#v\n", rqInfo1)

	nsRW5 := &NsReadWriteSet{"ns5",
		nil,
		nil,
		[]*RangeQueryInfo{&RangeQueryInfo{"startKey2", "endKey2", false, []*KVRead{&KVRead{"key11", version.NewHeight(1, 3)}}, nil}}}

	nsRW6 := &NsReadWriteSet{"ns6",
		nil,
		nil,
		[]*RangeQueryInfo{
			&RangeQueryInfo{"startKey2", "endKey2", false, []*KVRead{&KVRead{"key11", version.NewHeight(1, 3)}}, nil},
			&RangeQueryInfo{"startKey3", "endKey3", true, []*KVRead{&KVRead{"key12", version.NewHeight(2, 4)}}, nil}}}

	txRW.NsRWs = append(txRW.NsRWs, nsRW1, nsRW2, nsRW3, nsRW4, nsRW5, nsRW6)
	t.Logf("Testing txRWSet = %s", txRW)
	b, err := txRW.Marshal()
	testutil.AssertNoError(t, err, "Error while marshalling changeset")

	deserializedRWSet := &TxReadWriteSet{}
	err = deserializedRWSet.Unmarshal(b)
	testutil.AssertNoError(t, err, "Error while unmarshalling changeset")
	testutil.AssertEquals(t, deserializedRWSet, txRW)
}
