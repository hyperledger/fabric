/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statemachine

import (
	"fmt"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
)

type ackKey struct {
	clientID uint64
	reqNo    uint64
	digest   string
}

func ackToKey(ack *msgs.RequestAck) ackKey {
	return ackKey{
		clientID: ack.ClientId,
		reqNo:    ack.ReqNo,
		digest:   string(ack.Digest),
	}
}

func newOutstandingReqs(clientTracker *clientTracker, networkState *msgs.NetworkState, logger Logger) *allOutstandingReqs {
	clientTracker.availableList.resetIterator()

	ao := &allOutstandingReqs{
		buckets:             map[bucketID]*bucketOutstandingReqs{},
		correctRequests:     map[ackKey]*msgs.RequestAck{},
		outstandingRequests: map[ackKey]*sequence{},
		availableIterator:   clientTracker.availableList,
	}

	numBuckets := int(networkState.Config.NumberOfBuckets)

	for i := bucketID(0); i < bucketID(numBuckets); i++ {
		bo := &bucketOutstandingReqs{
			clients: map[uint64]*clientOutstandingReqs{},
		}
		ao.buckets[i] = bo

		for _, client := range networkState.Clients {
			var firstUncommitted uint64
			for j := 0; j < numBuckets; j++ {
				reqNo := client.LowWatermark + uint64(j)
				if clientReqToBucket(client.Id, reqNo, networkState.Config) == i {
					firstUncommitted = reqNo
					break
				}
			}

			cors := &clientOutstandingReqs{
				nextReqNo:  firstUncommitted,
				numBuckets: uint64(networkState.Config.NumberOfBuckets),
				client:     client,
			}
			cors.skipPreviouslyCommitted()

			logger.Log(LevelDebug, "initializing outstanding reqs for client", "client_id", client.Id, "bucket_id", i, "low_watermark", client.LowWatermark, "next_req_no", cors.nextReqNo)
			bo.clients[client.Id] = cors
		}
	}

	ao.advanceRequests() // Note, this can return no actions as no sequences have allocated

	return ao
}

type allOutstandingReqs struct {
	buckets             map[bucketID]*bucketOutstandingReqs
	availableIterator   *availableList
	correctRequests     map[ackKey]*msgs.RequestAck
	outstandingRequests map[ackKey]*sequence
}

type bucketOutstandingReqs struct {
	clients map[uint64]*clientOutstandingReqs // TODO, obvious optimization is to make this active clients and initialize this lazily
}

type clientOutstandingReqs struct {
	nextReqNo  uint64
	numBuckets uint64
	client     *msgs.NetworkState_Client
}

func (cors *clientOutstandingReqs) skipPreviouslyCommitted() {
	for {
		if !isCommitted(cors.nextReqNo, cors.client) {
			break
		}

		cors.nextReqNo += cors.numBuckets
	}
}

func (ao *allOutstandingReqs) advanceRequests() *ActionList {
	actions := &ActionList{}
	for ao.availableIterator.hasNext() {
		ack := ao.availableIterator.next()
		key := ackToKey(ack)

		if seq, ok := ao.outstandingRequests[key]; ok {
			delete(ao.outstandingRequests, key)
			actions.concat(seq.satisfyOutstanding(ack))
			continue
		}

		ao.correctRequests[key] = ack
	}

	return actions
}

// TODO, bucket probably can/should be stored in the *sequence
func (ao *allOutstandingReqs) applyAcks(bucket bucketID, seq *sequence, batch []*msgs.RequestAck) (*ActionList, error) {
	bo, ok := ao.buckets[bucket]
	assertTruef(ok, "told to apply acks for bucket %d which does not exist", bucket)

	outstandingReqs := map[ackKey]struct{}{}

	for _, req := range batch {
		co, ok := bo.clients[req.ClientId]
		if !ok {
			return nil, fmt.Errorf("no such client")
		}

		if co.nextReqNo != req.ReqNo {
			return nil, fmt.Errorf("expected ClientId=%d next request for Bucket=%d to have ReqNo=%d but got ReqNo=%d", req.ClientId, bucket, co.nextReqNo, req.ReqNo)
		}

		// TODO, return an error if the request proposed is for a seqno before this request is valid

		key := ackToKey(req)
		if _, ok := ao.correctRequests[key]; ok {
			delete(ao.correctRequests, key)
		} else {
			ao.outstandingRequests[key] = seq
			outstandingReqs[key] = struct{}{}
		}

		co.nextReqNo += co.numBuckets
		co.skipPreviouslyCommitted()
	}

	return seq.allocate(batch, outstandingReqs), nil
}
