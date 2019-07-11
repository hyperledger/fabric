/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pull

import (
	"bytes"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
)

var pullInterval time.Duration
var timeoutInterval = 20 * time.Second

func init() {
	util.SetupTestLogging()
	pullInterval = 500 * time.Millisecond
}

type pullMsg struct {
	respondChan chan *pullMsg
	msg         *proto.SignedGossipMessage
}

// GetSourceMessage Returns the SignedGossipMessage the ReceivedMessage was
// constructed with
func (pm *pullMsg) GetSourceEnvelope() *proto.Envelope {
	return pm.msg.Envelope
}

func (pm *pullMsg) Respond(msg *proto.GossipMessage) {
	sMsg, _ := msg.NoopSign()
	pm.respondChan <- &pullMsg{
		msg:         sMsg,
		respondChan: pm.respondChan,
	}
}

func (pm *pullMsg) GetGossipMessage() *proto.SignedGossipMessage {
	return pm.msg
}

func (pm *pullMsg) GetConnectionInfo() *proto.ConnectionInfo {
	return nil
}

// Ack returns to the sender an acknowledgement for the message
func (pm *pullMsg) Ack(err error) {

}

type pullInstance struct {
	self          discovery.NetworkMember
	mediator      Mediator
	items         *util.Set
	msgChan       chan *pullMsg
	peer2PullInst map[string]*pullInstance
	stopChan      chan struct{}
	pullAdapter   *PullAdapter
	config        Config
}

func (p *pullInstance) Send(msg *proto.SignedGossipMessage, peers ...*comm.RemotePeer) {
	for _, peer := range peers {
		m := &pullMsg{
			respondChan: p.msgChan,
			msg:         msg,
		}
		p.peer2PullInst[peer.Endpoint].msgChan <- m
	}
}

func (p *pullInstance) GetMembership() []discovery.NetworkMember {
	members := []discovery.NetworkMember{}
	for _, peer := range p.peer2PullInst {
		if bytes.Equal(peer.self.PKIid, p.self.PKIid) {
			// peer instance itself should not be part of the membership
			continue
		}
		members = append(members, peer.self)
	}
	return members
}

func (p *pullInstance) start() {
	p.mediator = NewPullMediator(p.config, p.pullAdapter)
	go func() {
		for {
			select {
			case <-p.stopChan:
				return
			case msg := <-p.msgChan:
				p.mediator.HandleMessage(msg)
			}
		}
	}()
}

func (p *pullInstance) stop() {
	p.mediator.Stop()
	p.stopChan <- struct{}{}
}

func (p *pullInstance) wrapPullMsg(msg *proto.SignedGossipMessage) proto.ReceivedMessage {
	return &pullMsg{
		msg:         msg,
		respondChan: p.msgChan,
	}
}

func createPullInstance(endpoint string, peer2PullInst map[string]*pullInstance) *pullInstance {
	return createPullInstanceWithFilters(endpoint, peer2PullInst, nil, nil)
}

func createPullInstanceWithFilters(endpoint string, peer2PullInst map[string]*pullInstance, df EgressDigestFilter, digestsFilter IngressDigestFilter) *pullInstance {
	inst := &pullInstance{
		items:         util.NewSet(),
		stopChan:      make(chan struct{}),
		peer2PullInst: peer2PullInst,
		self:          discovery.NetworkMember{Endpoint: endpoint, Metadata: []byte{}, PKIid: []byte(endpoint)},
		msgChan:       make(chan *pullMsg, 10),
	}

	peer2PullInst[endpoint] = inst

	conf := Config{
		MsgType:           proto.PullMsgType_BLOCK_MSG,
		Channel:           []byte(""),
		ID:                endpoint,
		PeerCountToSelect: 3,
		PullInterval:      pullInterval,
		Tag:               proto.GossipMessage_EMPTY,
		PullEngineConfig: algo.PullEngineConfig{
			DigestWaitTime:   time.Duration(100) * time.Millisecond,
			RequestWaitTime:  time.Duration(200) * time.Millisecond,
			ResponseWaitTime: time.Duration(300) * time.Millisecond,
		},
	}
	seqNumFromMsg := func(msg *proto.SignedGossipMessage) string {
		dataMsg := msg.GetDataMsg()
		if dataMsg == nil {
			return ""
		}
		if dataMsg.Payload == nil {
			return ""
		}
		return fmt.Sprintf("%d", dataMsg.Payload.SeqNum)
	}
	blockConsumer := func(msg *proto.SignedGossipMessage) {
		inst.items.Add(msg.GetDataMsg().Payload.SeqNum)
	}
	inst.pullAdapter = &PullAdapter{
		Sndr:             inst,
		MemSvc:           inst,
		IdExtractor:      seqNumFromMsg,
		MsgCons:          blockConsumer,
		EgressDigFilter:  df,
		IngressDigFilter: digestsFilter,
	}
	inst.config = conf

	return inst
}

func TestCreateAndStop(t *testing.T) {
	t.Parallel()
	pullInst := createPullInstance("localhost:2000", make(map[string]*pullInstance))
	pullInst.start()
	pullInst.stop()
}

func TestRegisterMsgHook(t *testing.T) {
	t.Parallel()
	peer2pullInst := make(map[string]*pullInstance)
	inst1 := createPullInstance("localhost:5611", peer2pullInst)
	inst2 := createPullInstance("localhost:5612", peer2pullInst)
	inst1.start()
	inst2.start()
	defer inst1.stop()
	defer inst2.stop()

	receivedMsgTypes := util.NewSet()

	for _, msgType := range []MsgType{HelloMsgType, DigestMsgType, RequestMsgType, ResponseMsgType} {
		mType := msgType
		inst1.mediator.RegisterMsgHook(mType, func(_ []string, items []*proto.SignedGossipMessage, _ proto.ReceivedMessage) {
			receivedMsgTypes.Add(mType)
		})
	}

	inst1.mediator.Add(dataMsg(1))
	inst2.mediator.Add(dataMsg(2))

	// Ensure all message types are received
	waitUntilOrFail(t, func() bool { return len(receivedMsgTypes.ToArray()) == 4 })

}

func TestFilter(t *testing.T) {
	t.Parallel()
	peer2pullInst := make(map[string]*pullInstance)

	eq := func(a interface{}, b interface{}) bool {
		return a == b
	}
	df := func(msg proto.ReceivedMessage) func(string) bool {
		if msg.GetGossipMessage().IsDataReq() {
			req := msg.GetGossipMessage().GetDataReq()
			return func(item string) bool {
				return util.IndexInSlice(util.BytesToStrings(req.Digests), item, eq) != -1
			}
		}
		return func(digestItem string) bool {
			n, _ := strconv.ParseInt(digestItem, 10, 64)
			return n%2 == 0
		}
	}
	inst1 := createPullInstanceWithFilters("localhost:5611", peer2pullInst, df, nil)
	inst2 := createPullInstance("localhost:5612", peer2pullInst)
	defer inst1.stop()
	defer inst2.stop()
	inst1.start()
	inst2.start()

	inst1.mediator.Add(dataMsg(0))
	inst1.mediator.Add(dataMsg(1))
	inst1.mediator.Add(dataMsg(2))
	inst1.mediator.Add(dataMsg(3))

	waitUntilOrFail(t, func() bool { return inst2.items.Exists(uint64(0)) })
	waitUntilOrFail(t, func() bool { return inst2.items.Exists(uint64(2)) })
	assert.False(t, inst2.items.Exists(uint64(1)))
	assert.False(t, inst2.items.Exists(uint64(3)))
}

func TestAddAndRemove(t *testing.T) {
	t.Parallel()
	peer2pullInst := make(map[string]*pullInstance)
	inst1 := createPullInstance("localhost:5611", peer2pullInst)
	inst2 := createPullInstance("localhost:5612", peer2pullInst)
	inst1.start()
	inst2.start()
	defer inst1.stop()
	defer inst2.stop()

	msgCount := 3

	go func() {
		for i := 0; i < msgCount; i++ {
			time.Sleep(pullInterval)
			inst1.mediator.Add(dataMsg(i))
		}
	}()

	// Ensure instance 2 got all messages
	waitUntilOrFail(t, func() bool { return len(inst2.items.ToArray()) == msgCount })

	// Remove message 0 from both instances
	inst2.mediator.Remove("0")
	inst1.mediator.Remove("0")
	inst2.items.Remove(uint64(0))

	// Add a message to inst1
	inst1.mediator.Add(dataMsg(10))

	// Need to make sure that instance 2, will issue pull
	// of missing data message with sequence number 10, i.e.
	// 1. Instance 2 sending Hello to instance 1
	// 2. Instance 1 answers with digest of current messages
	// 3. Instance 2 will request missing message 10
	// 4. Instance 1 provides missing item
	// Eventually need to ensure message 10 persisted in the state
	wg := sync.WaitGroup{}
	wg.Add(4)

	// Make sure there is a Hello message
	inst1.mediator.RegisterMsgHook(HelloMsgType, func(_ []string, items []*proto.SignedGossipMessage, msg proto.ReceivedMessage) {
		wg.Done()
	})

	// Instance 1 answering with digest
	inst2.mediator.RegisterMsgHook(DigestMsgType, func(_ []string, items []*proto.SignedGossipMessage, msg proto.ReceivedMessage) {
		wg.Done()
	})

	// Instance 2 requesting missing items
	inst1.mediator.RegisterMsgHook(RequestMsgType, func(_ []string, items []*proto.SignedGossipMessage, msg proto.ReceivedMessage) {
		wg.Done()
	})

	// Instance 1 sends missing item
	inst2.mediator.RegisterMsgHook(ResponseMsgType, func(_ []string, items []*proto.SignedGossipMessage, msg proto.ReceivedMessage) {
		wg.Done()
	})

	// Waiting for pull engine message exchanges
	wg.Wait()

	// Ensure instance 2 got new message
	assert.True(t, inst2.items.Exists(uint64(10)), "Instance 2 should have receive message 10 but didn't")

	// Ensure instance 2 doesn't have message 0
	assert.False(t, inst2.items.Exists(uint64(0)), "Instance 2 has message 0 but shouldn't have")
}

func TestDigestsFilters(t *testing.T) {
	t.Parallel()
	df1 := createDigestsFilter(2)
	inst1 := createPullInstanceWithFilters("localhost:5611", make(map[string]*pullInstance), nil, df1)
	inst2 := createPullInstance("localhost:5612", make(map[string]*pullInstance))
	inst1ReceivedDigest := int32(0)
	inst1.start()
	inst2.start()

	defer inst1.stop()
	defer inst2.stop()

	inst1.mediator.RegisterMsgHook(DigestMsgType, func(itemIds []string, _ []*proto.SignedGossipMessage, _ proto.ReceivedMessage) {
		if atomic.LoadInt32(&inst1ReceivedDigest) == int32(1) {
			return
		}
		for i := range itemIds {
			seqNum, err := strconv.ParseUint(itemIds[i], 10, 64)
			assert.NoError(t, err, "Can't parse seq number")
			assert.True(t, seqNum >= 2, "Digest with wrong ( ", seqNum, " ) seqNum passed")
		}
		assert.Len(t, itemIds, 2, "Not correct number of seqNum passed")
		atomic.StoreInt32(&inst1ReceivedDigest, int32(1))
	})

	inst2.mediator.Add(dataMsg(0))
	inst2.mediator.Add(dataMsg(1))
	inst2.mediator.Add(dataMsg(2))
	inst2.mediator.Add(dataMsg(3))

	// inst1 sends hello to inst2
	sMsg, _ := helloMsg().NoopSign()
	inst2.mediator.HandleMessage(inst1.wrapPullMsg(sMsg))

	// inst2 is expected to send digest to inst1
	waitUntilOrFail(t, func() bool { return atomic.LoadInt32(&inst1ReceivedDigest) == int32(1) })

}

func TestHandleMessage(t *testing.T) {
	t.Parallel()

	inst1 := createPullInstance("localhost:5611", make(map[string]*pullInstance))
	inst2 := createPullInstance("localhost:5612", make(map[string]*pullInstance))
	inst1.start()
	inst2.start()
	defer inst1.stop()
	defer inst2.stop()

	inst2.mediator.Add(dataMsg(0))
	inst2.mediator.Add(dataMsg(1))
	inst2.mediator.Add(dataMsg(2))

	inst1ReceivedDigest := int32(0)
	inst1ReceivedResponse := int32(0)

	inst1.mediator.RegisterMsgHook(DigestMsgType, func(itemIds []string, _ []*proto.SignedGossipMessage, _ proto.ReceivedMessage) {
		if atomic.LoadInt32(&inst1ReceivedDigest) == int32(1) {
			return
		}
		atomic.StoreInt32(&inst1ReceivedDigest, int32(1))
		assert.True(t, len(itemIds) == 3)
	})

	inst1.mediator.RegisterMsgHook(ResponseMsgType, func(_ []string, items []*proto.SignedGossipMessage, msg proto.ReceivedMessage) {
		if atomic.LoadInt32(&inst1ReceivedResponse) == int32(1) {
			return
		}
		atomic.StoreInt32(&inst1ReceivedResponse, int32(1))
		assert.True(t, len(items) == 3)
	})

	// inst1 sends hello to inst2
	sMsg, _ := helloMsg().NoopSign()
	inst2.mediator.HandleMessage(inst1.wrapPullMsg(sMsg))

	// inst2 is expected to send digest to inst1
	waitUntilOrFail(t, func() bool { return atomic.LoadInt32(&inst1ReceivedDigest) == int32(1) })

	// inst1 sends request to inst2
	sMsg, _ = reqMsg("0", "1", "2").NoopSign()
	inst2.mediator.HandleMessage(inst1.wrapPullMsg(sMsg))

	// inst2 is expected to send response to inst1
	waitUntilOrFail(t, func() bool { return atomic.LoadInt32(&inst1ReceivedResponse) == int32(1) })
	assert.True(t, inst1.items.Exists(uint64(0)))
	assert.True(t, inst1.items.Exists(uint64(1)))
	assert.True(t, inst1.items.Exists(uint64(2)))
}

func waitUntilOrFail(t *testing.T, pred func() bool) {
	start := time.Now()
	limit := start.UnixNano() + timeoutInterval.Nanoseconds()
	for time.Now().UnixNano() < limit {
		if pred() {
			return
		}
		time.Sleep(timeoutInterval / 1000)
	}
	util.PrintStackTrace()
	assert.Fail(t, "Timeout expired!")
}

func dataMsg(seqNum int) *proto.SignedGossipMessage {
	sMsg, _ := (&proto.GossipMessage{
		Nonce: 0,
		Tag:   proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{
				Payload: &proto.Payload{
					Data:   []byte{},
					SeqNum: uint64(seqNum),
				},
			},
		},
	}).NoopSign()
	return sMsg
}

func helloMsg() *proto.GossipMessage {
	return &proto.GossipMessage{
		Channel: []byte(""),
		Tag:     proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_Hello{
			Hello: &proto.GossipHello{
				Nonce:    0,
				Metadata: nil,
				MsgType:  proto.PullMsgType_BLOCK_MSG,
			},
		},
	}
}

func reqMsg(digest ...string) *proto.GossipMessage {
	return &proto.GossipMessage{
		Channel: []byte(""),
		Tag:     proto.GossipMessage_EMPTY,
		Nonce:   0,
		Content: &proto.GossipMessage_DataReq{
			DataReq: &proto.DataRequest{
				MsgType: proto.PullMsgType_BLOCK_MSG,
				Nonce:   0,
				Digests: util.StringsToBytes(digest),
			},
		},
	}
}

func createDigestsFilter(level uint64) IngressDigestFilter {
	return func(digestMsg *proto.DataDigest) *proto.DataDigest {
		res := &proto.DataDigest{
			MsgType: digestMsg.MsgType,
			Nonce:   digestMsg.Nonce,
		}
		for i := range digestMsg.Digests {
			seqNum, err := strconv.ParseUint(string(digestMsg.Digests[i]), 10, 64)
			if err != nil || seqNum < level {
				continue
			}
			res.Digests = append(res.Digests, digestMsg.Digests[i])

		}
		return res
	}
}
