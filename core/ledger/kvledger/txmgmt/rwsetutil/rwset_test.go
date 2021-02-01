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
	"io/ioutil"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/stretchr/testify/require"
)

const rwsetV1ProtoBytesFile = "testdata/rwsetV1ProtoBytes"

// TestRWSetV1BackwardCompatible passes if the 'RWSet' messgae declared in the latest version
// is able to unmarshal the protobytes that are produced by the 'RWSet' proto message declared in
// v1.0. This is to make sure that any incompatible changes does not go uncaught.
func TestRWSetV1BackwardCompatible(t *testing.T) {
	protoBytes, err := ioutil.ReadFile(rwsetV1ProtoBytesFile)
	require.NoError(t, err)
	rwset1 := &rwset.TxReadWriteSet{}
	require.NoError(t, proto.Unmarshal(protoBytes, rwset1))
	rwset2 := constructSampleRWSet()
	t.Logf("rwset1=%s, rwset2=%s", spew.Sdump(rwset1), spew.Sdump(rwset2))
	require.Equal(t, rwset2, rwset1)
}

// PrepareBinaryFileSampleRWSetV1 constructs a proto message for kvrwset and marshals its bytes to file 'rwsetV1ProtoBytes'.
// this code should be run on fabric version 1.0 so as to produce a sample file of proto message declared in V1
// In order to invoke this function on V1 code, copy this over on to V1 code, make the first letter as 'T', and finally invoke this function
// using golang test framwork
func PrepareBinaryFileSampleRWSetV1(t *testing.T) {
	b, err := proto.Marshal(constructSampleRWSet())
	require.NoError(t, err)
	require.NoError(t, ioutil.WriteFile(rwsetV1ProtoBytesFile, b, 0o644))
}

func constructSampleRWSet() *rwset.TxReadWriteSet {
	rwset1 := &rwset.TxReadWriteSet{}
	rwset1.DataModel = rwset.TxReadWriteSet_KV
	rwset1.NsRwset = []*rwset.NsReadWriteSet{
		{Namespace: "ns-1", Rwset: []byte("ns-1-rwset")},
		{Namespace: "ns-2", Rwset: []byte("ns-2-rwset")},
	}
	return rwset1
}
