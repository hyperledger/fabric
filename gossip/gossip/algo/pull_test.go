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

package algo

import (
	"sync"
	"testing"
	"time"

	"fmt"
	"sync/atomic"

	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/assert"
)

func init() {
	requestWaitTime = time.Duration(200) * time.Millisecond
	digestWaitTime = time.Duration(100) * time.Millisecond
	responseWaitTime = time.Duration(200) * time.Millisecond
}

type messageHook func(interface{})

type pullTestInstance struct {
	msgHooks          []messageHook
	peers             map[string]*pullTestInstance
	name              string
	nextPeerSelection []string
	msgQueue          chan interface{}
	lock              sync.Mutex
	stopChan          chan struct{}
	*PullEngine
}

type helloMsg struct {
	nonce  uint64
	source string
}

type digestMsg struct {
	nonce  uint64
	digest []uint64
	source string
}

type reqMsg struct {
	items  []uint64
	nonce  uint64
	source string
}

type resMsg struct {
	items []uint64
	nonce uint64
}

func newPushPullTestInstance(name string, peers map[string]*pullTestInstance) *pullTestInstance {
	inst := &pullTestInstance{
		msgHooks:          make([]messageHook, 0),
		peers:             peers,
		msgQueue:          make(chan interface{}, 100),
		nextPeerSelection: make([]string, 0),
		stopChan:          make(chan struct{}, 1),
		name:              name,
	}

	inst.PullEngine = NewPullEngine(inst, time.Duration(500)*time.Millisecond)

	peers[name] = inst
	go func() {
		for {
			select {
			case <-inst.stopChan:
				return
			case m := <-inst.msgQueue:
				inst.handleMessage(m)
				break
			}
		}
	}()

	return inst
}

// Used to test the messages one peer sends to another.
// Assert statements should be passed via the messageHook f
func (p *pullTestInstance) hook(f messageHook) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.msgHooks = append(p.msgHooks, f)
}

func (p *pullTestInstance) handleMessage(m interface{}) {
	p.lock.Lock()
	for _, f := range p.msgHooks {
		f(m)
	}
	p.lock.Unlock()

	if helloMsg, isHello := m.(*helloMsg); isHello {
		p.OnHello(helloMsg.nonce, helloMsg.source)
		return
	}

	if digestMsg, isDigest := m.(*digestMsg); isDigest {
		p.OnDigest(digestMsg.digest, digestMsg.nonce, digestMsg.source)
		return
	}

	if reqMsg, isReq := m.(*reqMsg); isReq {
		p.OnReq(reqMsg.items, reqMsg.nonce, reqMsg.source)
		return
	}

	if resMsg, isRes := m.(*resMsg); isRes {
		p.OnRes(resMsg.items, resMsg.nonce)
	}
}

func (p *pullTestInstance) stop() {
	p.stopChan <- struct{}{}
	p.Stop()
}

func (p *pullTestInstance) setNextPeerSelection(selection []string) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.nextPeerSelection = selection
}

func (p *pullTestInstance) SelectPeers() []string {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.nextPeerSelection
}

func (p *pullTestInstance) Hello(dest string, nonce uint64) {
	p.peers[dest].msgQueue <- &helloMsg{nonce: nonce, source: p.name}
}

func (p *pullTestInstance) SendDigest(digest []uint64, nonce uint64, context interface{}) {
	p.peers[context.(string)].msgQueue <- &digestMsg{source: p.name, nonce: nonce, digest: digest}
}

func (p *pullTestInstance) SendReq(dest string, items []uint64, nonce uint64) {
	p.peers[dest].msgQueue <- &reqMsg{nonce: nonce, source: p.name, items: items}
}

func (p *pullTestInstance) SendRes(items []uint64, context interface{}, nonce uint64) {
	p.peers[context.(string)].msgQueue <- &resMsg{items: items, nonce: nonce}
}

func TestPullEngine_Add(t *testing.T) {
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	defer inst1.Stop()
	inst1.Add(uint64(0))
	inst1.Add(uint64(0))
	assert.True(t, inst1.PullEngine.state.Exists(uint64(0)))
}

func TestPullEngine_Remove(t *testing.T) {
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	defer inst1.Stop()
	inst1.Add(uint64(0))
	assert.True(t, inst1.PullEngine.state.Exists(uint64(0)))
	inst1.Remove(uint64(0))
	assert.False(t, inst1.PullEngine.state.Exists(uint64(0)))
	inst1.Remove(uint64(0)) // remove twice
	assert.False(t, inst1.PullEngine.state.Exists(uint64(0)))
}

func TestPullEngine_Stop(t *testing.T) {
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	inst2 := newPushPullTestInstance("p2", peers)
	defer inst2.stop()
	inst2.setNextPeerSelection([]string{"p1"})
	go func() {
		for i := 0; i < 100; i++ {
			inst1.Add(uint64(i))
			time.Sleep(time.Duration(10) * time.Millisecond)
		}
	}()

	time.Sleep(time.Duration(800) * time.Millisecond)
	len1 := len(inst2.state.ToArray())
	inst1.stop()
	time.Sleep(time.Duration(800) * time.Millisecond)
	len2 := len(inst2.state.ToArray())
	assert.Equal(t, len1, len2, "PullEngine was still active after Stop() was invoked!")
}

func TestPullEngineAll2AllWithIncrementalSpawning(t *testing.T) {
	// Scenario: spawn 10 nodes, each 50 ms after the other
	// and have them transfer data between themselves.
	// Expected outcome: obviously, everything should succeed.
	// Isn't that's why we're here?
	instanceCount := 10
	peers := make(map[string]*pullTestInstance)

	for i := 0; i < instanceCount; i++ {
		inst := newPushPullTestInstance(fmt.Sprintf("p%d", i+1), peers)
		inst.Add(uint64(i + 1))
		time.Sleep(time.Duration(50) * time.Millisecond)
	}
	for i := 0; i < instanceCount; i++ {
		pID := fmt.Sprintf("p%d", i+1)
		peers[pID].setNextPeerSelection(keySet(pID, peers))
	}
	time.Sleep(time.Duration(4000) * time.Millisecond)

	for i := 0; i < instanceCount; i++ {
		pID := fmt.Sprintf("p%d", i+1)
		assert.Equal(t, instanceCount, len(peers[pID].state.ToArray()))
	}
}

func TestPullEngineSelectiveUpdates(t *testing.T) {
	// Scenario: inst1 has {1, 3} and inst2 has {0,1,2,3}.
	// inst1 initiates to inst2
	// Expected outcome: inst1 asks for 0,2 and inst2 sends 0,2 only
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	inst2 := newPushPullTestInstance("p2", peers)
	defer inst1.stop()
	defer inst2.stop()

	inst1.Add(uint64(1), uint64(3))
	inst2.Add(uint64(0), uint64(1), uint64(2), uint64(3))

	// Ensure inst2 sent a proper digest to inst1
	inst1.hook(func(m interface{}) {
		if dig, isDig := m.(*digestMsg); isDig {
			assert.True(t, util.IndexInSlice(dig.digest, uint64(0), numericCompare) != -1)
			assert.True(t, util.IndexInSlice(dig.digest, uint64(1), numericCompare) != -1)
			assert.True(t, util.IndexInSlice(dig.digest, uint64(2), numericCompare) != -1)
			assert.True(t, util.IndexInSlice(dig.digest, uint64(3), numericCompare) != -1)
		}
	})

	// Ensure inst1 requested only needed updates from inst2
	inst2.hook(func(m interface{}) {
		if req, isReq := m.(*reqMsg); isReq {
			assert.True(t, util.IndexInSlice(req.items, uint64(1), numericCompare) == -1)
			assert.True(t, util.IndexInSlice(req.items, uint64(3), numericCompare) == -1)

			assert.True(t, util.IndexInSlice(req.items, uint64(0), numericCompare) != -1)
			assert.True(t, util.IndexInSlice(req.items, uint64(2), numericCompare) != -1)
		}
	})

	// Ensure inst1 received only needed updates from inst2
	inst1.hook(func(m interface{}) {
		if res, isRes := m.(*resMsg); isRes {
			assert.True(t, util.IndexInSlice(res.items, uint64(1), numericCompare) == -1)
			assert.True(t, util.IndexInSlice(res.items, uint64(3), numericCompare) == -1)

			assert.True(t, util.IndexInSlice(res.items, uint64(0), numericCompare) != -1)
			assert.True(t, util.IndexInSlice(res.items, uint64(2), numericCompare) != -1)
		}
	})

	inst1.setNextPeerSelection([]string{"p2"})

	time.Sleep(time.Duration(2000) * time.Millisecond)
	assert.Equal(t, len(inst2.state.ToArray()), len(inst1.state.ToArray()))
}

func TestByzantineResponder(t *testing.T) {
	// Scenario: inst1 sends hello to inst2 but inst3 is byzantine so it attempts to send a digest and a response to inst1.
	// expected outcome is for inst1 not to process updates from inst3.
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	inst2 := newPushPullTestInstance("p2", peers)
	inst3 := newPushPullTestInstance("p3", peers)
	defer inst1.stop()
	defer inst2.stop()
	defer inst3.stop()

	receivedDigestFromInst3 := int32(0)

	inst2.Add(uint64(1), uint64(2), uint64(3))
	inst3.Add(uint64(5), uint64(6), uint64(7))

	inst2.hook(func(m interface{}) {
		if _, isHello := m.(*helloMsg); isHello {
			inst3.SendDigest([]uint64{uint64(5), uint64(6), uint64(7)}, 0, "p1")
		}
	})

	inst1.hook(func(m interface{}) {
		if dig, isDig := m.(*digestMsg); isDig {
			if dig.source == "p3" {
				atomic.StoreInt32(&receivedDigestFromInst3, int32(1))
				time.AfterFunc(time.Duration(150)*time.Millisecond, func() {
					inst3.SendRes([]uint64{uint64(5), uint64(6), uint64(7)}, "p1", 0)
				})
			}
		}

		if res, isRes := m.(*resMsg); isRes {
			// the response is from p3
			if util.IndexInSlice(res.items, uint64(6), numericCompare) != -1 {
				// inst1 is currently accepting responses
				assert.Equal(t, int32(1), atomic.LoadInt32(&(inst1.acceptingResponses)), "inst1 is not accepting digests")
			}
		}
	})

	inst1.setNextPeerSelection([]string{"p2"})

	time.Sleep(time.Duration(1000) * time.Millisecond)

	assert.Equal(t, int32(1), atomic.LoadInt32(&receivedDigestFromInst3), "inst1 hasn't received a digest from inst3")

	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(1), numericCompare) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(2), numericCompare) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(3), numericCompare) != -1)

	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(5), numericCompare) == -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(6), numericCompare) == -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(7), numericCompare) == -1)

}

func TestMultipleInitiators(t *testing.T) {
	// Scenario: inst1, inst2 and inst3 both start protocol with inst4 at the same time.
	// Expected outcome: inst4 successfully transfers state to all of them
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	inst2 := newPushPullTestInstance("p2", peers)
	inst3 := newPushPullTestInstance("p3", peers)
	inst4 := newPushPullTestInstance("p4", peers)
	defer inst1.stop()
	defer inst2.stop()
	defer inst3.stop()
	defer inst4.stop()

	inst4.Add(uint64(1), uint64(2), uint64(3), uint64(4))
	inst1.setNextPeerSelection([]string{"p4"})
	inst2.setNextPeerSelection([]string{"p4"})
	inst3.setNextPeerSelection([]string{"p4"})

	time.Sleep(time.Duration(2000) * time.Millisecond)

	for _, inst := range []*pullTestInstance{inst1, inst2, inst3} {
		assert.True(t, util.IndexInSlice(inst.state.ToArray(), uint64(1), numericCompare) != -1)
		assert.True(t, util.IndexInSlice(inst.state.ToArray(), uint64(2), numericCompare) != -1)
		assert.True(t, util.IndexInSlice(inst.state.ToArray(), uint64(3), numericCompare) != -1)
		assert.True(t, util.IndexInSlice(inst.state.ToArray(), uint64(4), numericCompare) != -1)
	}

}

func TestLatePeers(t *testing.T) {
	// Scenario: inst1 initiates to inst2 (items: {1,2,3,4}) and inst3 (items: {5,6,7,8}),
	// but inst2 is too slow to respond, and all items
	// should be received from inst3.
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	inst2 := newPushPullTestInstance("p2", peers)
	inst3 := newPushPullTestInstance("p3", peers)
	defer inst1.stop()
	defer inst2.stop()
	defer inst3.stop()
	inst2.Add(uint64(1), uint64(2), uint64(3), uint64(4))
	inst3.Add(uint64(5), uint64(6), uint64(7), uint64(8))
	inst2.hook(func(m interface{}) {
		time.Sleep(time.Duration(600) * time.Millisecond)
	})
	inst1.setNextPeerSelection([]string{"p2", "p3"})

	time.Sleep(time.Duration(2000) * time.Millisecond)

	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(1), numericCompare) == -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(2), numericCompare) == -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(3), numericCompare) == -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(4), numericCompare) == -1)

	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(5), numericCompare) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(6), numericCompare) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(7), numericCompare) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(8), numericCompare) != -1)

}

func TestBiDiUpdates(t *testing.T) {
	// Scenario: inst1 has {1, 3} and inst2 has {0,2} and both initiate to the other at the same time.
	// Expected outcome: both have {0,1,2,3} in the end
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	inst2 := newPushPullTestInstance("p2", peers)
	defer inst1.stop()
	defer inst2.stop()

	inst1.Add(uint64(1), uint64(3))
	inst2.Add(uint64(0), uint64(2))

	inst1.setNextPeerSelection([]string{"p2"})
	inst2.setNextPeerSelection([]string{"p1"})

	time.Sleep(time.Duration(2000) * time.Millisecond)

	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(0), numericCompare) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(1), numericCompare) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(2), numericCompare) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), uint64(3), numericCompare) != -1)

	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), uint64(0), numericCompare) != -1)
	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), uint64(1), numericCompare) != -1)
	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), uint64(2), numericCompare) != -1)
	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), uint64(3), numericCompare) != -1)

}

func TestSpread(t *testing.T) {
	// Scenario: inst1 initiates to inst2, inst3 inst4 and each have items 0-100. inst5 also has the same items but isn't selected
	// Expected outcome: each responder (inst2, inst3 and inst4) is chosen at least once (the probability for not choosing each of them is slim)
	// inst5 isn't selected at all
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	inst2 := newPushPullTestInstance("p2", peers)
	inst3 := newPushPullTestInstance("p3", peers)
	inst4 := newPushPullTestInstance("p4", peers)
	inst5 := newPushPullTestInstance("p5", peers)
	defer inst1.stop()
	defer inst2.stop()
	defer inst3.stop()
	defer inst4.stop()
	defer inst5.stop()

	chooseCounters := make(map[string]int)
	chooseCounters["p2"] = 0
	chooseCounters["p3"] = 0
	chooseCounters["p4"] = 0
	chooseCounters["p5"] = 0

	lock := &sync.Mutex{}

	addToCounters := func(dest string) func(m interface{}) {
		return func(m interface{}) {
			if _, isReq := m.(*reqMsg); isReq {
				lock.Lock()
				chooseCounters[dest]++
				lock.Unlock()
			}
		}
	}

	inst2.hook(addToCounters("p2"))
	inst3.hook(addToCounters("p3"))
	inst4.hook(addToCounters("p4"))
	inst5.hook(addToCounters("p5"))

	for i := 0; i < 100; i++ {
		item := uint64(i)
		inst2.Add(item)
		inst3.Add(item)
		inst4.Add(item)
	}

	inst1.setNextPeerSelection([]string{"p2", "p3", "p4"})

	time.Sleep(time.Duration(2000) * time.Millisecond)

	lock.Lock()
	for pI, counter := range chooseCounters {
		if pI == "p5" {
			assert.Equal(t, 0, counter)
		} else {
			assert.True(t, counter > 0, "%s was not selected!", pI)
		}
	}
	lock.Unlock()

}

func numericCompare(a interface{}, b interface{}) bool {
	return a.(uint64) == b.(uint64)
}

func keySet(selfPeer string, m map[string]*pullTestInstance) []string {
	peers := make([]string, len(m)-1)
	i := 0
	for pID := range m {
		if pID == selfPeer {
			continue
		}
		peers[i] = pID
		i++
	}

	return peers
}
