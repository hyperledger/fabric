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

package txmgmt

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledgernext/testutil"
)

func TestTxRWSetMarshalUnmarshal(t *testing.T) {
	txRW := &TxReadWriteSet{}
	nsRW1 := &NsReadWriteSet{"ns1",
		[]*KVRead{&KVRead{"key1", uint64(1)}},
		[]*KVWrite{&KVWrite{"key2", false, []byte("value2")}}}

	nsRW2 := &NsReadWriteSet{"ns2",
		[]*KVRead{&KVRead{"key3", uint64(1)}},
		[]*KVWrite{&KVWrite{"key4", true, nil}}}

	nsRW3 := &NsReadWriteSet{"ns3",
		[]*KVRead{&KVRead{"key5", uint64(1)}},
		[]*KVWrite{&KVWrite{"key6", false, []byte("value6")}, &KVWrite{"key7", false, []byte("value7")}}}

	txRW.NsRWs = append(txRW.NsRWs, nsRW1, nsRW2, nsRW3)

	b, err := txRW.Marshal()
	testutil.AssertNoError(t, err, "Error while marshalling changeset")

	deserializedRWSet := &TxReadWriteSet{}
	err = deserializedRWSet.Unmarshal(b)
	testutil.AssertNoError(t, err, "Error while unmarshalling changeset")
	t.Logf("Unmarshalled changeset = %#+v", deserializedRWSet.NsRWs[0].Writes[0].IsDelete)
	testutil.AssertEquals(t, deserializedRWSet, txRW)

}
