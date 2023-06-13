/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider_test

import (
	"fmt"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider/fake"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sync/atomic"
	"testing"
	"time"
)

func TestBftHeaderReceiver_NoBlocks(t *testing.T) {
	fakeBlockVerifier := &fake.BlockVerifier{}
	fakeBlockVerifier.VerifyBlockAttestationReturns(fmt.Errorf("fake-verify-error"))

	streamClientMock := &fake.DeliverClient{}
	streamClientMock.RecvReturns(nil, errors.New("oops"))
	streamClientMock.CloseSendReturns(nil)

	hr := blocksprovider.NewBFTHeaderReceiver(
		"testchannel", "10.10.10.11:666", streamClientMock, fakeBlockVerifier, 10*time.Millisecond, 10*time.Second,
		flogging.MustGetLogger("test.blocksprovider"))
	assert.NotNil(t, hr)

	_, _, err := hr.LastBlockNum()
	assert.EqualError(t, err, "Not found")

	hr.DeliverHeaders()
	_, _, err = hr.LastBlockNum()
	assert.EqualError(t, err, "Not found")
	fakeBlockVerifier.Invocations()
	assert.Equal(t, fakeBlockVerifier.VerifyBlockAttestationCallCount(), 0)
}

func TestBftHeaderReceiver_WithBlocks(t *testing.T) {
	fakeBlockVerifier := &fake.BlockVerifier{}
	streamClientMock := &fake.DeliverClient{}
	hr := blocksprovider.NewBFTHeaderReceiver("testchannel", "10.10.10.11:666", streamClientMock, fakeBlockVerifier, 10*time.Millisecond, 10*time.Second,
		flogging.MustGetLogger("test.blocksprovider"))

	seq := uint64(0)
	goodSig := uint32(1)
	streamClientMock.RecvCalls(
		func() (*orderer.DeliverResponse, error) {
			time.Sleep(time.Millisecond)
			seqNew := atomic.AddUint64(&seq, 1)
			return prepareBlock(seqNew, orderer.SeekInfo_HEADER_WITH_SIG, atomic.LoadUint32(&goodSig)), nil
		},
	)
	streamClientMock.CloseSendReturns(nil)

	fakeBlockVerifier.VerifyBlockAttestationCalls(
		func(_ string, signedBlock *common.Block) error {
			sigArray := signedBlock.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES]
			sig := string(sigArray)
			if sig == "good" {
				return nil
			}
			return errors.New("test: bad signature")
		},
	)

	start := time.Now()
	go hr.DeliverHeaders()

	assert.True(t, waitForAtomicGreaterThan(&seq, 1))
	bNum, bTime, err := hr.LastBlockNum()
	assert.NoError(t, err)
	assert.True(t, uint64(0) < bNum, "expect bNum = %d > 0", bNum)
	assert.True(t, bTime.After(start))
	assert.Equal(t, fakeBlockVerifier.VerifyBlockAttestationCallCount(), 1)

	bNumOld := bNum
	assert.True(t, waitForAtomicGreaterThan(&seq, bNumOld+2))
	bNum, bTime, err = hr.LastBlockNum()
	assert.NoError(t, err)
	assert.True(t, bNumOld < bNum, "expect bNum = %d > %d", bNum, bNumOld)
	assert.True(t, bTime.After(start))
	assert.Equal(t, fakeBlockVerifier.VerifyBlockAttestationCallCount(), 2)

	//Invalid blocks
	bNumOld = bNum
	atomic.StoreUint32(&goodSig, 0)
	assert.True(t, waitForAtomicGreaterThan(&seq, bNumOld+3))
	bNum, bTime, err = hr.LastBlockNum()
	assert.EqualError(t, err, "Last block verification failed: test: bad signature")
	assert.True(t, bNumOld < bNum, "expect bNum = %d > %d", bNum, bNumOld)
	assert.True(t, bTime.After(start))
	assert.Equal(t, fakeBlockVerifier.VerifyBlockAttestationCallCount(), 3)

	hr.CloseSend()
	assert.True(t, hr.IsStopped())
}

func TestBftHeaderReceiver_VerifyOnce(t *testing.T) {
	fakeBlockVerifier := &fake.BlockVerifier{}
	streamClientMock := &fake.DeliverClient{}
	hr := blocksprovider.NewBFTHeaderReceiver("testchannel", "10.10.10.11:666", streamClientMock, fakeBlockVerifier, 10*time.Millisecond, 10*time.Second,
		flogging.MustGetLogger("test.blocksprovider"))

	seq := uint64(0)
	done := uint32(0)
	streamClientMock.RecvCalls(
		func() (*orderer.DeliverResponse, error) {
			for atomic.LoadUint64(&seq) > 0 && atomic.LoadUint32(&done) == 0 {
				time.Sleep(time.Millisecond)
			}
			seqNew := atomic.AddUint64(&seq, 1)
			return prepareBlock(seqNew, orderer.SeekInfo_HEADER_WITH_SIG, 1), nil
		})
	streamClientMock.CloseSendReturns(nil)

	fakeBlockVerifier.VerifyBlockAttestationCalls(
		func(_ string, signedBlock *common.Block) error {
			sigArray := signedBlock.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES]
			sig := string(sigArray)
			if sig == "good" {
				return nil
			}
			return errors.New("test: bad signature")
		},
	)

	start := time.Now()
	go hr.DeliverHeaders()

	assert.True(t, waitForAtomicGreaterThan(&seq, 0))

	for i := 0; i < 10; {
		bNum, bTime, err := hr.LastBlockNum()
		if bNum > 0 {
			assert.NoError(t, err)
			assert.Equal(t, uint64(1), bNum)
			assert.True(t, bTime.After(start))
			i++
		} else {
			assert.EqualError(t, err, "Not found")
		}
	}
	assert.Equal(t, fakeBlockVerifier.VerifyBlockAttestationCallCount(), 1)

	hr.CloseSend()
	atomic.StoreUint32(&done, 1)
	assert.True(t, hr.IsStopped())
}

func prepareBlock(seq uint64, contentType orderer.SeekInfo_SeekContentType, goodSignature uint32) *orderer.DeliverResponse {
	const numTx = 10
	block := protoutil.NewBlock(seq, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	data := &common.BlockData{
		Data: make([][]byte, numTx),
	}
	for i := 0; i < numTx; i++ {
		data.Data[i] = []byte{byte(i), byte(seq)}
	}
	block.Header.DataHash = protoutil.BlockDataHash(data)
	if contentType == orderer.SeekInfo_BLOCK {
		block.Data = data
	}

	if goodSignature > 0 {
		block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = []byte("good")
	} else {
		block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = []byte("bad")
	}

	return &orderer.DeliverResponse{Type: &orderer.DeliverResponse_Block{Block: block}}
}

func waitForAtomicGreaterThan(addr *uint64, threshold uint64, timeoutOpt ...time.Duration) bool {
	to := 5 * time.Second
	if len(timeoutOpt) > 0 {
		to = timeoutOpt[0]
	}

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(to)

	for {
		select {
		case <-ticker.C:
		case <-timeout:
			return false
		}

		if atomic.LoadUint64(addr) > threshold {
			return true
		}
	}
}
