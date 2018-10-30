/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blockcutter

import (
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	cb "github.com/hyperledger/fabric/protos/common"
)

var logger = flogging.MustGetLogger("orderer.mocks.common.blockcutter")

// Receiver mocks the blockcutter.Receiver interface
type Receiver struct {
	// IsolatedTx causes Ordered returns [][]{curBatch, []{newTx}}, false when set to true
	IsolatedTx bool

	// CutAncestors causes Ordered returns [][]{curBatch}, true when set to true
	CutAncestors bool

	// CutNext causes Ordered returns [][]{append(curBatch, newTx)}, false when set to true
	CutNext bool

	// SkipAppendCurBatch causes Ordered to skip appending to curBatch
	SkipAppendCurBatch bool

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
		IsolatedTx:   false,
		CutAncestors: false,
		CutNext:      false,
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

	if mbc.IsolatedTx {
		logger.Debugf("Receiver: Returning dual batch")
		res := [][]*cb.Envelope{mbc.curBatch, {env}}
		mbc.curBatch = nil
		return res, false
	}

	if mbc.CutAncestors {
		logger.Debugf("Receiver: Returning current batch and appending newest env")
		res := [][]*cb.Envelope{mbc.curBatch}
		mbc.curBatch = []*cb.Envelope{env}
		return res, true
	}

	if !mbc.SkipAppendCurBatch {
		mbc.curBatch = append(mbc.curBatch, env)
	}

	if mbc.CutNext {
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
