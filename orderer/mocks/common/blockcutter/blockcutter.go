/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blockcutter

import (
	"sync"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/flogging"
)

var logger = flogging.MustGetLogger("orderer.mocks.common.blockcutter")

// Receiver mocks the blockcutter.Receiver interface
type Receiver struct {
	// isolatedTx causes Ordered returns [][]{curBatch, []{newTx}}, false when set to true
	isolatedTx bool

	// cutAncestors causes Ordered returns [][]{curBatch}, true when set to true
	cutAncestors bool

	// cutNext causes Ordered returns [][]{append(curBatch, newTx)}, false when set to true
	cutNext bool

	// skipAppendCurBatch causes Ordered to skip appending to curBatch
	skipAppendCurBatch bool

	// Lock to serialize writes access to curBatch
	mutex sync.Mutex

	// curBatch is the currently outstanding messages in the batch
	curBatch []*cb.Envelope

	// Block is a channel which is read from before returning from Ordered, it is useful for synchronization
	// If you do not wish synchronization for whatever reason, simply close the channel
	Block chan struct{}
}

// NewReceiver returns the mock blockcutter.Receiver implementation
func NewReceiver() *Receiver {
	return &Receiver{
		isolatedTx:   false,
		cutAncestors: false,
		cutNext:      false,
		Block:        make(chan struct{}),
	}
}

// Ordered will add or cut the batch according to the state of Receiver, it blocks reading from Block on return
func (mbc *Receiver) Ordered(env *cb.Envelope) ([][]*cb.Envelope, bool) {
	defer func() {
		<-mbc.Block
	}()

	mbc.mutex.Lock()
	defer mbc.mutex.Unlock()

	if mbc.isolatedTx {
		logger.Debugf("Receiver: Returning dual batch")
		res := [][]*cb.Envelope{mbc.curBatch, {env}}
		mbc.curBatch = nil
		return res, false
	}

	if mbc.cutAncestors {
		logger.Debugf("Receiver: Returning current batch and appending newest env")
		res := [][]*cb.Envelope{mbc.curBatch}
		mbc.curBatch = []*cb.Envelope{env}
		return res, true
	}

	if !mbc.skipAppendCurBatch {
		mbc.curBatch = append(mbc.curBatch, env)
	}

	if mbc.cutNext {
		logger.Debugf("Receiver: Returning regular batch")
		res := [][]*cb.Envelope{mbc.curBatch}
		mbc.curBatch = nil
		return res, false
	}

	logger.Debugf("Appending to batch")
	return nil, true
}

// Cut terminates the current batch, returning it
func (mbc *Receiver) Cut() []*cb.Envelope {
	mbc.mutex.Lock()
	defer mbc.mutex.Unlock()
	logger.Debugf("Cutting batch")
	res := mbc.curBatch
	mbc.curBatch = nil
	return res
}

func (mbc *Receiver) CurBatch() []*cb.Envelope {
	mbc.mutex.Lock()
	defer mbc.mutex.Unlock()
	return mbc.curBatch
}

// SetIsolatedTx is used to change the isolatedTx field in a thread safe manner.
func (mbc *Receiver) SetIsolatedTx(isolatedTx bool) {
	mbc.mutex.Lock()
	defer mbc.mutex.Unlock()
	mbc.isolatedTx = isolatedTx
}

// SetCutAncestors is used to change the cutAncestors field in a thread safe manner.
func (mbc *Receiver) SetCutAncestors(cutAncestors bool) {
	mbc.mutex.Lock()
	defer mbc.mutex.Unlock()
	mbc.cutAncestors = cutAncestors
}

// SetCutNext is used to change the cutNext field in a thread safe manner.
func (mbc *Receiver) SetCutNext(cutNext bool) {
	mbc.mutex.Lock()
	defer mbc.mutex.Unlock()
	mbc.cutNext = cutNext
}

// SkipAppendCurBatchSet is used to change the skipAppendCurBatch field in a thread safe manner.
func (mbc *Receiver) SkipAppendCurBatchSet(skipAppendCurBatch bool) {
	mbc.mutex.Lock()
	defer mbc.mutex.Unlock()
	mbc.skipAppendCurBatch = skipAppendCurBatch
}
