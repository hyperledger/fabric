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

package multichannel

import (
	"testing"

	"github.com/golang/protobuf/proto"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	"github.com/hyperledger/fabric/common/mocks/crypto"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
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

func (mlw *mockLedgerReadWriter) Iterator(startType *ab.SeekPosition) (ledger.Iterator, uint64) {
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
	cs := &chainSupport{
		ledgerResources: &ledgerResources{configResources: &configResources{Manager: cm}, ledger: ml},
		filters:         filter.NewRuleSet([]filter.Rule{filter.AcceptRule}),
		signer:          mockCrypto(),
	}
	assert.Equal(t, uint64(0), cs.Height(), "Should has height of 0")

	txs := []*cb.Envelope{makeNormalTx("foo", 0), makeNormalTx("bar", 1)}
	block := cs.CreateNextBlock(txs)
	cs.WriteConfigBlock(block, nil)
	assert.Equal(t, uint64(1), cs.Height(), "Should has height of 1")

	blockTXs := make([]*cb.Envelope, len(ml.data))
	for i := range ml.data {
		blockTXs[i] = utils.UnmarshalEnvelopeOrPanic(ml.data[i])
	}

	assert.Equal(t, txs, blockTXs, "Should have written input data to ledger but did not")
}

func TestWriteBlockSignatures(t *testing.T) {
	ml := &mockLedgerReadWriter{}
	cm := &mockconfigtx.Manager{}
	cs := &chainSupport{ledgerResources: &ledgerResources{configResources: &configResources{Manager: cm}, ledger: ml}, signer: mockCrypto()}

	actual := utils.GetMetadataFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(0, nil), nil), cb.BlockMetadataIndex_SIGNATURES)
	assert.NotNil(t, actual, "Block should have block signature")
}

func TestWriteBlockOrdererMetadata(t *testing.T) {
	ml := &mockLedgerReadWriter{}
	cm := &mockconfigtx.Manager{}
	cs := &chainSupport{ledgerResources: &ledgerResources{configResources: &configResources{Manager: cm}, ledger: ml}, signer: mockCrypto()}

	value := []byte("foo")
	expected := &cb.Metadata{Value: value}
	actual := utils.GetMetadataFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(0, nil), value), cb.BlockMetadataIndex_ORDERER)
	assert.NotNil(t, actual, "Block should have orderer metadata written")
	assert.True(t, proto.Equal(expected, actual), "Orderer metadata not written to block correctly")
}

func TestSignature(t *testing.T) {
	ml := &mockLedgerReadWriter{}
	cm := &mockconfigtx.Manager{}
	cs := &chainSupport{ledgerResources: &ledgerResources{configResources: &configResources{Manager: cm}, ledger: ml}, signer: mockCrypto()}

	message := []byte("Darth Vader")
	signed, _ := cs.Sign(message)
	assert.Equal(t, message, signed, "Should sign the message")

	signatureHeader, _ := cs.NewSignatureHeader()
	assert.Equal(t, crypto.FakeLocalSigner.Identity, signatureHeader.Creator)
	assert.Equal(t, crypto.FakeLocalSigner.Nonce, signatureHeader.Nonce)
}

func TestWriteLastConfig(t *testing.T) {
	ml := &mockLedgerReadWriter{}
	cm := &mockconfigtx.Manager{}
	cs := &chainSupport{ledgerResources: &ledgerResources{configResources: &configResources{Manager: cm}, ledger: ml}, signer: mockCrypto()}

	expected := uint64(0)
	lc := utils.GetLastConfigIndexFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(0, nil), nil))
	assert.Equal(t, expected, lc, "First block should have config block index of %d, but got %d", expected, lc)
	lc = utils.GetLastConfigIndexFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(1, nil), nil))
	assert.Equal(t, expected, lc, "Second block should have config block index of %d, but got %d", expected, lc)

	cm.SequenceVal = 1
	expected = uint64(2)
	lc = utils.GetLastConfigIndexFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(2, nil), nil))
	assert.Equal(t, expected, lc, "Second block should have config block index of %d, but got %d", expected, lc)

	lc = utils.GetLastConfigIndexFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(3, nil), nil))
	assert.Equal(t, expected, lc, "Second block should have config block index of %d, but got %d", expected, lc)

	t.Run("ResetChainSupport", func(t *testing.T) {
		cm.SequenceVal = 2
		expected = uint64(4)

		cs = &chainSupport{ledgerResources: &ledgerResources{configResources: &configResources{Manager: cm}, ledger: ml}, signer: mockCrypto()}
		lc := utils.GetLastConfigIndexFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(4, nil), nil))
		assert.Equal(t, expected, lc, "Second block should have config block index of %d, but got %d", expected, lc)

		lc = utils.GetLastConfigIndexFromBlockOrPanic(cs.WriteBlock(cb.NewBlock(5, nil), nil))
		assert.Equal(t, expected, lc, "Second block should have config block index of %d, but got %d")
	})
}
