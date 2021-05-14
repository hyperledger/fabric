/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statemachine

import (
	"bytes"
	"fmt"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
)

type batchTracker struct {

	// Indexes the observed batches by their digests.
	batchesByDigest map[string]*batch

	// Maps batch digests to lists of sequence numbers.
	// For each batch digest, stores the list of sequence numbers
	// for which the batch is being fetched (e.g. during an epoch change).
	// The list (instead of just a boolean) is necessary,
	// since we might need to fetch batches that happen to have the same digest
	// (empty batches) for multiple different sequence numbers.
	// Referring to a batch solely by its digest is thus not enough.
	fetchInFlight map[string][]uint64

	persisted *persisted
}

type batch struct {
	observedFor map[uint64]struct{}
	requestAcks []*msgs.RequestAck
}

func newBatchTracker(persisted *persisted) *batchTracker {
	return &batchTracker{
		batchesByDigest: map[string]*batch{},
		fetchInFlight:   map[string][]uint64{},
		persisted:       persisted,
	}
}

func (bt *batchTracker) reinitialize() {
	bt.persisted.iterate(logIterator{
		onQEntry: func(qEntry *msgs.QEntry) {
			bt.addBatch(qEntry.SeqNo, qEntry.Digest, qEntry.Requests)
		},
	})
}

func (bt *batchTracker) step(source nodeID, msg *msgs.Msg) *ActionList {
	switch innerMsg := msg.Type.(type) {
	case *msgs.Msg_FetchBatch:
		msg := innerMsg.FetchBatch
		return bt.replyFetchBatch(uint64(source), msg.SeqNo, msg.Digest)
	case *msgs.Msg_ForwardBatch:
		msg := innerMsg.ForwardBatch
		return bt.applyForwardBatchMsg(source, msg.SeqNo, msg.Digest, msg.RequestAcks)
	default:
		panic(fmt.Sprintf("unexpected bad batch message type %T, this indicates a bug", msg.Type))
	}
}

func (bt *batchTracker) truncate(seqNo uint64) {
	for digest, batch := range bt.batchesByDigest {
		for seq := range batch.observedFor {
			if seq < seqNo {
				delete(batch.observedFor, seq)
			}
		}
		if len(batch.observedFor) == 0 {
			delete(bt.batchesByDigest, digest)
		}
	}
}

// Adds a request batch to the batch tracker.
func (bt *batchTracker) addBatch(seqNo uint64, digest []byte, requestAcks []*msgs.RequestAck) {

	// Create an entry in the batch index if this is the first time we see the batch.
	b, ok := bt.batchesByDigest[string(digest)]
	if !ok {
		b = &batch{
			observedFor: map[uint64]struct{}{},
			requestAcks: requestAcks,
		}
		bt.batchesByDigest[string(digest)] = b
	}

	// Mark the sequence number for which the added batch is destined.
	b.observedFor[seqNo] = struct{}{}

	// If this same batch is currently being fetched, potentially for other sequence numbers
	// (this can generally be the case for empty batches), mark it as observed for those
	// sequence numbers as well and remove the "fetching" flag.
	inFlight, ok := bt.fetchInFlight[string(digest)]
	if ok {
		for _, ifSeqNo := range inFlight {
			b.observedFor[ifSeqNo] = struct{}{}
		}
		delete(bt.fetchInFlight, string(digest))
	}
}

func (bt *batchTracker) fetchBatch(seqNo uint64, digest []byte, sources []uint64) *ActionList {

	// Check if a batch with this digest is already being fetched.
	inFlight, ok := bt.fetchInFlight[string(digest)]

	// If it is, return immediately.
	if ok {
		// It's a weird, but possible case, that two batches have
		// identical digests, for different seqNos.  If so, we need
		// to track them separately.
		for _, ifSeqNo := range inFlight {
			if ifSeqNo == seqNo {
				return &ActionList{}
			}
		}
	}

	// Mark the batch as "being fetched" (if inFlight is nil, it behaves like an empty slice).
	inFlight = append(inFlight, seqNo)
	bt.fetchInFlight[string(digest)] = inFlight

	// Request the batch from all nodes that have it.
	// TODO: If there are many sources, we probably get bombarded by loads of data.
	//       Use a less agressive approach with timeouts?
	return (&ActionList{}).Send(
		sources,
		&msgs.Msg{
			Type: &msgs.Msg_FetchBatch{
				FetchBatch: &msgs.FetchBatch{
					SeqNo:  seqNo,
					Digest: digest,
				},
			},
		},
	)
}

func (bt *batchTracker) replyFetchBatch(source uint64, seqNo uint64, digest []byte) *ActionList {
	batch, ok := bt.getBatch(digest)
	if !ok {
		// TODO, is this worth logging, or just ignore? (It's not necessarily byzantine)
		return &ActionList{}
	}

	return (&ActionList{}).Send(
		[]uint64{source},
		&msgs.Msg{
			Type: &msgs.Msg_ForwardBatch{
				ForwardBatch: &msgs.ForwardBatch{
					SeqNo:       seqNo,
					Digest:      digest,
					RequestAcks: batch.requestAcks,
				},
			},
		},
	)
}

func (bt *batchTracker) applyForwardBatchMsg(source nodeID, seqNo uint64, digest []byte, requestAcks []*msgs.RequestAck) *ActionList {
	_, ok := bt.fetchInFlight[string(digest)]
	if !ok {
		// We did not request this batch digest, so we don't know if we can trust it, discard
		// TODO, maybe log? Maybe not though, since we delete from the map when we get it.
		return &ActionList{}
	}

	data := make([][]byte, len(requestAcks))
	for i, requestAck := range requestAcks {
		data[i] = requestAck.Digest
	}
	return (&ActionList{}).Hash(data, &state.HashOrigin{
		Type: &state.HashOrigin_VerifyBatch_{
			VerifyBatch: &state.HashOrigin_VerifyBatch{
				Source:         uint64(source),
				SeqNo:          seqNo,
				RequestAcks:    requestAcks,
				ExpectedDigest: digest,
			},
		},
	})
}

func (bt *batchTracker) applyVerifyBatchHashResult(digest []byte, verifyBatch *state.HashOrigin_VerifyBatch) {
	if !bytes.Equal(verifyBatch.ExpectedDigest, digest) {
		panic("byzantine")
		// XXX this should be a log only, but panic-ing to make dev easier for now
	}

	inFlight, ok := bt.fetchInFlight[string(digest)]
	if !ok {
		// We must have gotten multiple responses, and already
		// committed one, which is fine.
		return
	}

	b, ok := bt.batchesByDigest[string(digest)]
	if !ok {
		b = &batch{
			observedFor: map[uint64]struct{}{},
			requestAcks: verifyBatch.RequestAcks,
		}
		bt.batchesByDigest[string(digest)] = b
	}

	for _, seqNo := range inFlight {
		b.observedFor[seqNo] = struct{}{}
	}

	delete(bt.fetchInFlight, string(digest))
}

func (bt *batchTracker) hasFetchInFlight() bool {
	return len(bt.fetchInFlight) > 0
}

func (bt *batchTracker) getBatch(digest []byte) (*batch, bool) {
	b, ok := bt.batchesByDigest[string(digest)]
	return b, ok
}
