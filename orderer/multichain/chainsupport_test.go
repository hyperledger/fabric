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

package multichain

import (
	"reflect"
	"testing"

	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

type mockLedgerReadWriter struct {
	data     []*cb.Envelope
	metadata [][]byte
}

func (mlw *mockLedgerReadWriter) Append(data []*cb.Envelope, metadata [][]byte) *cb.Block {
	mlw.data = data
	mlw.metadata = metadata
	return nil
}

func (mlw *mockLedgerReadWriter) Iterator(startType *ab.SeekPosition) (rawledger.Iterator, uint64) {
	panic("Unimplemented")
}

func (mlw *mockLedgerReadWriter) Height() uint64 {
	panic("Unimplemented")
}

type mockCommitter struct {
	committed int
}

func (mc *mockCommitter) Isolated() bool {
	panic("Unimplemented")
}

func (mc *mockCommitter) Commit() {
	mc.committed++
}

func TestCommitConfig(t *testing.T) {
	ml := &mockLedgerReadWriter{}
	cs := &chainSupport{ledger: ml}
	txs := []*cb.Envelope{makeNormalTx("foo", 0), makeNormalTx("bar", 1)}
	md := [][]byte{[]byte("foometa"), []byte("barmeta")}
	committers := []filter.Committer{&mockCommitter{}, &mockCommitter{}}
	cs.WriteBlock(txs, md, committers)

	if !reflect.DeepEqual(ml.data, txs) {
		t.Errorf("Should have written input data to ledger but did not")
	}

	if !reflect.DeepEqual(ml.metadata, md) {
		t.Errorf("Should have written input metadata to ledger but did not")
	}

	for _, c := range committers {
		if c.(*mockCommitter).committed != 1 {
			t.Errorf("Expected exactly 1 commits but got %d", c.(*mockCommitter).committed)
		}
	}
}
