/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider_test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider/fake"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBftHeaderReceiver_NoBlocks_RecvError(t *testing.T) {
	fakeBlockVerifier := &fake.BlockVerifier{}
	fakeBlockVerifier.VerifyBlockAttestationReturns(fmt.Errorf("fake-verify-error"))

	streamClientMock := &fake.DeliverClient{}
	streamClientMock.RecvReturns(nil, errors.New("oops"))
	streamClientMock.CloseSendReturns(nil)

	hr := blocksprovider.NewBFTHeaderReceiver("testchannel", "10.10.10.11:666", streamClientMock, fakeBlockVerifier, nil, flogging.MustGetLogger("test.BFTHeaderReceiver"))
	assert.NotNil(t, hr)
	assert.False(t, hr.IsStarted())
	assert.False(t, hr.IsStopped())
	_, _, err := hr.LastBlockNum()
	assert.EqualError(t, err, "not found")

	hr.DeliverHeaders() // it will get a Recv() error and exit

	assert.Eventually(t, hr.IsStarted, time.Second, time.Millisecond)
	assert.Eventually(t, hr.IsStopped, time.Second, time.Millisecond)
	_, _, err = hr.LastBlockNum()
	assert.EqualError(t, err, "not found")
	assert.Equal(t, fakeBlockVerifier.VerifyBlockAttestationCallCount(), 0)
	assert.Equal(t, 1, streamClientMock.RecvCallCount())
}

func TestBftHeaderReceiver_BadStatus(t *testing.T) {
	fakeBlockVerifier := &fake.BlockVerifier{}
	fakeBlockVerifier.VerifyBlockAttestationReturns(fmt.Errorf("fake-verify-error"))

	streamClientMock := &fake.DeliverClient{}
	streamClientMock.RecvReturnsOnCall(0, &orderer.DeliverResponse{Type: &orderer.DeliverResponse_Status{Status: common.Status_SUCCESS}}, nil)
	streamClientMock.RecvReturnsOnCall(1, &orderer.DeliverResponse{Type: &orderer.DeliverResponse_Status{Status: common.Status_BAD_REQUEST}}, nil)
	streamClientMock.RecvReturnsOnCall(2, &orderer.DeliverResponse{Type: &orderer.DeliverResponse_Status{Status: common.Status_SERVICE_UNAVAILABLE}}, nil)
	streamClientMock.CloseSendReturns(nil)

	for i := 0; i < 3; i++ {
		hr := blocksprovider.NewBFTHeaderReceiver("testchannel", "10.10.10.11:666", streamClientMock, fakeBlockVerifier, nil, flogging.MustGetLogger("test.BFTHeaderReceiver"))
		assert.NotNil(t, hr)

		hr.DeliverHeaders() // it will get a bad status and exit
		assert.Eventually(t, hr.IsStarted, time.Second, time.Millisecond)
		assert.Eventually(t, hr.IsStopped, time.Second, time.Millisecond)
		_, _, err := hr.LastBlockNum()
		assert.EqualError(t, err, "not found")
		assert.Equal(t, fakeBlockVerifier.VerifyBlockAttestationCallCount(), 0)
	}
}

func TestBftHeaderReceiver_NilResponse(t *testing.T) {
	fakeBlockVerifier := &fake.BlockVerifier{}
	fakeBlockVerifier.VerifyBlockAttestationReturns(fmt.Errorf("fake-verify-error"))

	streamClientMock := &fake.DeliverClient{}
	streamClientMock.RecvReturns(nil, nil)
	streamClientMock.CloseSendReturns(nil)

	hr := blocksprovider.NewBFTHeaderReceiver("testchannel", "10.10.10.11:666", streamClientMock, fakeBlockVerifier, nil, flogging.MustGetLogger("test.BFTHeaderReceiver"))
	assert.NotNil(t, hr)

	hr.DeliverHeaders() // it will get a bad status and exit
	assert.Eventually(t, hr.IsStarted, time.Second, time.Millisecond)
	assert.Eventually(t, hr.IsStopped, time.Second, time.Millisecond)
	_, _, err := hr.LastBlockNum()
	assert.EqualError(t, err, "not found")
	assert.Equal(t, fakeBlockVerifier.VerifyBlockAttestationCallCount(), 0)
}

func TestBftHeaderReceiver_WithBlocks_Renew(t *testing.T) {
	flogging.ActivateSpec("debug")
	fakeBlockVerifier := &fake.BlockVerifier{}
	streamClientMock := &fake.DeliverClient{}
	hr := blocksprovider.NewBFTHeaderReceiver("testchannel", "10.10.10.11:666", streamClientMock, fakeBlockVerifier, nil, flogging.MustGetLogger("test.BFTHeaderReceiver"))

	seqCh := make(chan uint64)
	streamClientMock.RecvCalls(
		func() (*orderer.DeliverResponse, error) {
			time.Sleep(time.Millisecond)

			seqNew, ok := <-seqCh
			if ok {
				return prepareBlock(seqNew, orderer.SeekInfo_HEADER_WITH_SIG, uint32(1)), nil
			} else {
				return nil, errors.New("test closed")
			}
		},
	)
	streamClientMock.CloseSendReturns(nil)
	fakeBlockVerifier.VerifyBlockAttestationCalls(naiveBlockVerifier)

	go hr.DeliverHeaders()

	var bNum uint64
	var bTime time.Time
	var err error

	seqCh <- uint64(1)
	require.Eventually(t, func() bool {
		bNum, bTime, err = hr.LastBlockNum()
		return err == nil && bNum == uint64(1) && !bTime.IsZero()
	}, time.Second, time.Millisecond)

	bTimeOld := bTime
	seqCh <- uint64(2)
	require.Eventually(t, func() bool {
		bNum, bTime, err = hr.LastBlockNum()
		return err == nil && bNum == uint64(2) && bTime.After(bTimeOld)
	}, time.Second, time.Millisecond)

	err = hr.Stop()
	assert.NoError(t, err)

	bTimeOld = bTime
	bNum, bTime, err = hr.LastBlockNum()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), bNum)
	assert.Equal(t, bTime, bTimeOld)

	assert.Equal(t, fakeBlockVerifier.VerifyBlockAttestationCallCount(), 2)

	//=== Create a new BFTHeaderReceiver with the last good header of the previous receiver
	fakeBlockVerifier = &fake.BlockVerifier{}
	streamClientMock = &fake.DeliverClient{}
	hr2 := blocksprovider.NewBFTHeaderReceiver("testchannel", "10.10.10.11:666", streamClientMock, fakeBlockVerifier, hr, flogging.MustGetLogger("test.BFTHeaderReceiver.2"))
	assert.False(t, hr2.IsStarted())
	assert.False(t, hr2.IsStopped())
	bNum, bTime, err = hr2.LastBlockNum()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), bNum)
	assert.Equal(t, bTime, bTimeOld)
}

func TestBftHeaderReceiver_WithBlocks_StopOnVerificationFailure(t *testing.T) {
	flogging.ActivateSpec("debug")
	fakeBlockVerifier := &fake.BlockVerifier{}
	streamClientMock := &fake.DeliverClient{}
	hr := blocksprovider.NewBFTHeaderReceiver("testchannel", "10.10.10.11:666", streamClientMock, fakeBlockVerifier, nil, flogging.MustGetLogger("test.BFTHeaderReceiver"))

	seqCh := make(chan uint64)
	goodSig := uint32(1)
	streamClientMock.RecvCalls(
		func() (*orderer.DeliverResponse, error) {
			time.Sleep(time.Millisecond)

			seqNew, ok := <-seqCh
			if ok {
				return prepareBlock(seqNew, orderer.SeekInfo_HEADER_WITH_SIG, atomic.LoadUint32(&goodSig)), nil
			} else {
				return nil, errors.New("test closed")
			}
		},
	)
	streamClientMock.CloseSendReturns(nil)
	fakeBlockVerifier.VerifyBlockAttestationCalls(naiveBlockVerifier)

	go hr.DeliverHeaders()

	var bNum uint64
	var bTime time.Time
	var err error

	seqCh <- uint64(1)
	require.Eventually(t, func() bool {
		bNum, bTime, err = hr.LastBlockNum()
		return err == nil && bNum == uint64(1) && !bTime.IsZero()
	}, time.Second, time.Millisecond)

	bTimeOld := bTime
	seqCh <- uint64(2)
	require.Eventually(t, func() bool {
		bNum, bTime, err = hr.LastBlockNum()
		return err == nil && bNum == uint64(2) && bTime.After(bTimeOld)
	}, time.Second, time.Millisecond)

	// Invalid block sig causes the receiver to close
	atomic.StoreUint32(&goodSig, 0)
	seqCh <- uint64(3)
	require.Eventually(t, hr.IsStopped, time.Second, time.Millisecond)

	// After the receiver closes, it returns the last good header
	bTimeOld = bTime
	bNum, bTime, err = hr.LastBlockNum()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), bNum)
	assert.Equal(t, bTime, bTimeOld)

	assert.Equal(t, fakeBlockVerifier.VerifyBlockAttestationCallCount(), 3)
}

func TestBftHeaderReceiver_VerifyOnce(t *testing.T) {
	flogging.ActivateSpec("debug")
	fakeBlockVerifier := &fake.BlockVerifier{}
	streamClientMock := &fake.DeliverClient{}
	hr := blocksprovider.NewBFTHeaderReceiver("testchannel", "10.10.10.11:666", streamClientMock, fakeBlockVerifier, nil, flogging.MustGetLogger("test.BFTHeaderReceiver"))

	seqCh := make(chan uint64)
	goodSig := uint32(1)
	streamClientMock.RecvCalls(
		func() (*orderer.DeliverResponse, error) {
			time.Sleep(time.Millisecond)

			seqNew, ok := <-seqCh
			if ok {
				return prepareBlock(seqNew, orderer.SeekInfo_HEADER_WITH_SIG, atomic.LoadUint32(&goodSig)), nil
			} else {
				return nil, errors.New("test closed")
			}
		},
	)
	streamClientMock.CloseSendReturns(nil)
	fakeBlockVerifier.VerifyBlockAttestationCalls(naiveBlockVerifier)

	go hr.DeliverHeaders()

	seqCh <- uint64(5)
	require.Eventually(t, func() bool {
		bNum, bTime, err := hr.LastBlockNum()
		return err == nil && bNum == uint64(5) && !bTime.IsZero()
	}, time.Second, time.Millisecond)

	for i := 0; i < 10; i++ {
		bNum, bTime, err := hr.LastBlockNum()
		assert.NoError(t, err)
		assert.Equal(t, uint64(5), bNum)
		assert.True(t, !bTime.IsZero())
	}
	assert.Equal(t, fakeBlockVerifier.VerifyBlockAttestationCallCount(), 1)

	hr.Stop()
	assert.Eventually(t, hr.IsStopped, time.Second, time.Millisecond)
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

func naiveBlockVerifier(_ string, signedBlock *common.Block) error {
	sigArray := signedBlock.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES]
	sig := string(sigArray)
	if sig == "good" {
		return nil
	}
	return errors.New("test: bad signature")
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
