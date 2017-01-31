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

	"github.com/golang/protobuf/proto"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	"github.com/hyperledger/fabric/orderer/common/filter"
	ordererledger "github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

type mockLedgerReadWriter struct {
	data     [][]byte
	metadata [][]byte
	height   uint64
}

func (mlw *mockLedgerReadWriter) Append(block *cb.Block) error {
	mlw.data = block.Data.Data
	mlw.metadata = block.Metadata.Metadata
	mlw.height++
	return nil
}

func (mlw *mockLedgerReadWriter) Iterator(startType *ab.SeekPosition) (ordererledger.Iterator, uint64) {
	panic("Unimplemented")
}

func (mlw *mockLedgerReadWriter) Height() uint64 {
	return mlw.height
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
	cm := &mockconfigtx.Manager{}
	cs := &chainSupport{ledgerResources: &ledgerResources{configResources: &configResources{Manager: cm}, ledger: ml}, signer: &xxxCryptoHelper{}}
	txs := []*cb.Envelope{makeNormalTx("foo", 0), makeNormalTx("bar", 1)}
	committers := []filter.Committer{&mockCommitter{}, &mockCommitter{}}
	block := cs.CreateNextBlock(txs)
	cs.WriteBlock(block, committers, nil)

	blockTXs := make([]*cb.Envelope, len(ml.data))
	for i := range ml.data {
		blockTXs[i] = utils.UnmarshalEnvelopeOrPanic(ml.data[i])
	}

	if !reflect.DeepEqual(blockTXs, txs) {
		t.Errorf("Should have written input data to ledger but did not")
	}

	for _, c := range committers {
		if c.(*mockCommitter).committed != 1 {
			t.Errorf("Expected exactly 1 commits but got %d", c.(*mockCommitter).committed)
		}
	}
}

func TestWriteBlockSignatures(t *testing.T) {
	ml := &mockLedgerReadWriter{}
	cm := &mockconfigtx.Manager{}
	cs := &chainSupport{ledgerResources: &ledgerResources{configResources: &configResources{Manager: cm}, ledger: ml}, signer: &xxxCryptoHelper{}}

	if utils.GetMetadataFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(0, nil), nil, nil), cb.BlockMetadataIndex_SIGNATURES) == nil {
		t.Fatalf("Block should have block signature")
	}
}

func TestWriteBlockOrdererMetadata(t *testing.T) {
	ml := &mockLedgerReadWriter{}
	cm := &mockconfigtx.Manager{}
	cs := &chainSupport{ledgerResources: &ledgerResources{configResources: &configResources{Manager: cm}, ledger: ml}, signer: &xxxCryptoHelper{}}

	value := []byte("foo")
	expected := &cb.Metadata{Value: value}
	actual := utils.GetMetadataFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(0, nil), nil, value), cb.BlockMetadataIndex_ORDERER)

	if actual == nil {
		t.Fatalf("Block should have orderer metadata written")
	}
	if !proto.Equal(expected, actual) {
		t.Fatalf("Orderer metadata not written to block correctly")
	}

}

func TestWriteLastConfiguration(t *testing.T) {
	ml := &mockLedgerReadWriter{}
	cm := &mockconfigtx.Manager{}
	cs := &chainSupport{ledgerResources: &ledgerResources{configResources: &configResources{Manager: cm}, ledger: ml}, signer: &xxxCryptoHelper{}}

	expected := uint64(0)

	if lc := utils.GetLastConfigurationIndexFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(0, nil), nil, nil)); lc != expected {
		t.Fatalf("First block should have config block index of %d, but got %d", expected, lc)
	}
	if lc := utils.GetLastConfigurationIndexFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(1, nil), nil, nil)); lc != expected {
		t.Fatalf("Second block should have config block index of %d, but got %d", expected, lc)
	}

	cm.SequenceVal = 1
	expected = uint64(2)

	if lc := utils.GetLastConfigurationIndexFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(2, nil), nil, nil)); lc != expected {
		t.Fatalf("Second block should have config block index of %d, but got %d", expected, lc)
	}

	if lc := utils.GetLastConfigurationIndexFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(3, nil), nil, nil)); lc != expected {
		t.Fatalf("Second block should have config block index of %d, but got %d", expected, lc)
	}

}
