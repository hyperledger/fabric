/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package processor

import (
	"bytes"
	"container/list"
	"sync"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
	"github.com/hyperledger-labs/mirbft/pkg/statemachine"
)

var ErrClientNotExist error = errors.New("client does not exist")

func (cs *Clients) Client(clientID uint64) *Client {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	if cs.clients == nil {
		cs.clients = map[uint64]*Client{}
	}

	c, ok := cs.clients[clientID]
	if !ok {
		c = newClient(clientID, cs.Hasher, cs.RequestStore)
		cs.clients[clientID] = c
	}
	return c
}

type Clients struct {
	Hasher       Hasher
	RequestStore RequestStore

	mutex   sync.Mutex
	clients map[uint64]*Client
}

func (c *Clients) ProcessClientActions(actions *statemachine.ActionList) (*statemachine.EventList, error) {
	events := &statemachine.EventList{}
	iter := actions.Iterator()
	for action := iter.Next(); action != nil; action = iter.Next() {
		switch t := action.Type.(type) {
		case *state.Action_AllocatedRequest:
			r := t.AllocatedRequest
			client := c.Client(r.ClientId)
			digest, err := client.allocate(r.ReqNo)
			if err != nil {
				return nil, err
			}

			if digest == nil {
				continue
			}

			events.RequestPersisted(&msgs.RequestAck{
				ClientId: r.ClientId,
				ReqNo:    r.ReqNo,
				Digest:   digest,
			})
		case *state.Action_CorrectRequest:
			client := c.Client(t.CorrectRequest.ClientId)
			err := client.addCorrectDigest(t.CorrectRequest.ReqNo, t.CorrectRequest.Digest)
			if err != nil {
				return nil, err
			}
		case *state.Action_StateApplied:
			for _, client := range t.StateApplied.NetworkState.Clients {
				c.Client(client.Id).stateApplied(client)
			}
		default:
			return nil, errors.Errorf("unexpected type for client action: %T", action.Type)
		}
	}
	return events, nil
}

// TODO, client needs to be updated based on the state applied events, to give it a low watermark
// minimally and to clean up the reqNoMap
type Client struct {
	mutex        sync.Mutex
	hasher       Hasher
	clientID     uint64
	nextReqNo    uint64
	requestStore RequestStore
	requests     *list.List
	reqNoMap     map[uint64]*list.Element
}

func newClient(clientID uint64, hasher Hasher, reqStore RequestStore) *Client {
	return &Client{
		clientID:     clientID,
		hasher:       hasher,
		requestStore: reqStore,
		requests:     list.New(),
		reqNoMap:     map[uint64]*list.Element{},
	}
}

type clientRequest struct {
	reqNo                 uint64
	localAllocationDigest []byte
	remoteCorrectDigests  [][]byte
}

func (c *Client) stateApplied(state *msgs.NetworkState_Client) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for reqNo, el := range c.reqNoMap {
		if reqNo < state.LowWatermark {
			c.requests.Remove(el)
			delete(c.reqNoMap, reqNo)
		}
	}
	if c.nextReqNo < state.LowWatermark {
		c.nextReqNo = state.LowWatermark
	}
}

func (c *Client) allocate(reqNo uint64) ([]byte, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	el, ok := c.reqNoMap[reqNo]
	if ok {
		clientReq := el.Value.(*clientRequest)
		return clientReq.localAllocationDigest, nil
	}

	cr := &clientRequest{
		reqNo: reqNo,
	}
	el = c.requests.PushBack(cr)
	c.reqNoMap[reqNo] = el

	digest, err := c.requestStore.GetAllocation(c.clientID, reqNo)
	if err != nil {
		return nil, errors.WithMessagef(err, "could not get key for %d.%d", c.clientID, reqNo)
	}

	cr.localAllocationDigest = digest

	return digest, nil
}

func (c *Client) addCorrectDigest(reqNo uint64, digest []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.requests.Len() == 0 {
		return ErrClientNotExist
	}

	el, ok := c.reqNoMap[reqNo]
	if !ok {
		if reqNo < c.requests.Front().Value.(*clientRequest).reqNo {
			return nil
		}
		return errors.Errorf("unallocated client request for req_no=%d marked correct", reqNo)
	}

	clientReq := el.Value.(*clientRequest)
	for _, otherDigest := range clientReq.remoteCorrectDigests {
		if bytes.Equal(digest, otherDigest) {
			return nil
		}
	}

	clientReq.remoteCorrectDigests = append(clientReq.remoteCorrectDigests, digest)

	return nil
}

func (c *Client) NextReqNo() (uint64, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.requests.Len() == 0 {
		return 0, ErrClientNotExist
	}

	return c.nextReqNo, nil
}

func (c *Client) Propose(reqNo uint64, data []byte) (*statemachine.EventList, error) {
	h := c.hasher.New()
	h.Write(data)
	digest := h.Sum(nil)

	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.requests.Len() == 0 {
		return nil, ErrClientNotExist
	}

	if reqNo < c.nextReqNo {
		return &statemachine.EventList{}, nil
	}

	if reqNo == c.nextReqNo {
		for {
			c.nextReqNo++
			// Keep incrementing 'nextRequest' until we find one we don't have

			el, ok := c.reqNoMap[c.nextReqNo]
			if !ok {
				break
			}

			if el.Value.(*clientRequest).localAllocationDigest == nil {
				break
			}
		}
	}

	el, ok := c.reqNoMap[reqNo]
	previouslyAllocated := ok
	if !ok {
		// TODO, limit the distance ahead a client can allocate?
		el = c.requests.PushBack(&clientRequest{
			reqNo: reqNo,
		})
		c.reqNoMap[reqNo] = el
	}

	cr := el.Value.(*clientRequest)

	if cr.localAllocationDigest != nil {
		if bytes.Equal(cr.localAllocationDigest, digest) {
			return &statemachine.EventList{}, nil
		}

		return nil, errors.Errorf("cannot store request with digest %x, already stored request with different digest %x", digest, cr.localAllocationDigest)
	}

	if len(cr.remoteCorrectDigests) > 0 {
		found := false
		for _, rd := range cr.remoteCorrectDigests {
			if bytes.Equal(rd, digest) {
				found = true
				break
			}
		}

		if !found {
			return nil, errors.New("other known correct digest exist for reqno")
		}
	}

	ack := &msgs.RequestAck{
		ClientId: c.clientID,
		ReqNo:    reqNo,
		Digest:   digest,
	}

	err := c.requestStore.PutRequest(ack, data)
	if err != nil {
		return nil, errors.WithMessage(err, "could not store requests")
	}

	err = c.requestStore.PutAllocation(c.clientID, reqNo, digest)
	if err != nil {
		return nil, err
	}
	cr.localAllocationDigest = digest

	if previouslyAllocated {
		return (&statemachine.EventList{}).RequestPersisted(ack), nil
	}

	return &statemachine.EventList{}, nil
}
