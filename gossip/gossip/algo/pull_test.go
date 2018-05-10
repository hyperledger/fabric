/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package algo

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func init() {
	util.SetupTestLogging()
	SetDigestWaitTime(time.Duration(100) * time.Millisecond)
	SetRequestWaitTime(time.Duration(200) * time.Millisecond)
	SetResponseWaitTime(time.Duration(200) * time.Millisecond)
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
	digest []string
	source string
}

type reqMsg struct {
	items  []string
	nonce  uint64
	source string
}

type resMsg struct {
	items []string
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

func (p *pullTestInstance) SendDigest(digest []string, nonce uint64, context interface{}) {
	p.peers[context.(string)].msgQueue <- &digestMsg{source: p.name, nonce: nonce, digest: digest}
}

func (p *pullTestInstance) SendReq(dest string, items []string, nonce uint64) {
	p.peers[dest].msgQueue <- &reqMsg{nonce: nonce, source: p.name, items: items}
}

func (p *pullTestInstance) SendRes(items []string, context interface{}, nonce uint64) {
	p.peers[context.(string)].msgQueue <- &resMsg{items: items, nonce: nonce}
}

func TestPullEngine_Add(t *testing.T) {
	t.Parallel()
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	defer inst1.Stop()
	inst1.Add("0")
	inst1.Add("0")
	assert.True(t, inst1.PullEngine.state.Exists("0"))
}

func TestPullEngine_Remove(t *testing.T) {
	t.Parallel()
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	defer inst1.Stop()
	inst1.Add("0")
	assert.True(t, inst1.PullEngine.state.Exists("0"))
	inst1.Remove("0")
	assert.False(t, inst1.PullEngine.state.Exists("0"))
	inst1.Remove("0") // remove twice
	assert.False(t, inst1.PullEngine.state.Exists("0"))
}

func TestPullEngine_Stop(t *testing.T) {
	t.Parallel()
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	inst2 := newPushPullTestInstance("p2", peers)
	defer inst2.stop()
	inst2.setNextPeerSelection([]string{"p1"})
	go func() {
		for i := 0; i < 100; i++ {
			inst1.Add(string(i))
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
	t.Parallel()
	// Scenario: spawn 10 nodes, each 50 ms after the other
	// and have them transfer data between themselves.
	// Expected outcome: obviously, everything should succeed.
	// Isn't that's why we're here?
	instanceCount := 10
	peers := make(map[string]*pullTestInstance)

	for i := 0; i < instanceCount; i++ {
		inst := newPushPullTestInstance(fmt.Sprintf("p%d", i+1), peers)
		inst.Add(string(i + 1))
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
	t.Parallel()
	// Scenario: inst1 has {1, 3} and inst2 has {0,1,2,3}.
	// inst1 initiates to inst2
	// Expected outcome: inst1 asks for 0,2 and inst2 sends 0,2 only
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	inst2 := newPushPullTestInstance("p2", peers)
	defer inst1.stop()
	defer inst2.stop()

	inst1.Add("1", "3")
	inst2.Add("0", "1", "2", "3")

	// Ensure inst2 sent a proper digest to inst1
	inst1.hook(func(m interface{}) {
		if dig, isDig := m.(*digestMsg); isDig {
			assert.True(t, util.IndexInSlice(dig.digest, "0", Strcmp) != -1)
			assert.True(t, util.IndexInSlice(dig.digest, "1", Strcmp) != -1)
			assert.True(t, util.IndexInSlice(dig.digest, "2", Strcmp) != -1)
			assert.True(t, util.IndexInSlice(dig.digest, "3", Strcmp) != -1)
		}
	})

	// Ensure inst1 requested only needed updates from inst2
	inst2.hook(func(m interface{}) {
		if req, isReq := m.(*reqMsg); isReq {
			assert.True(t, util.IndexInSlice(req.items, "1", Strcmp) == -1)
			assert.True(t, util.IndexInSlice(req.items, "3", Strcmp) == -1)

			assert.True(t, util.IndexInSlice(req.items, "0", Strcmp) != -1)
			assert.True(t, util.IndexInSlice(req.items, "2", Strcmp) != -1)
		}
	})

	// Ensure inst1 received only needed updates from inst2
	inst1.hook(func(m interface{}) {
		if res, isRes := m.(*resMsg); isRes {
			assert.True(t, util.IndexInSlice(res.items, "1", Strcmp) == -1)
			assert.True(t, util.IndexInSlice(res.items, "3", Strcmp) == -1)

			assert.True(t, util.IndexInSlice(res.items, "0", Strcmp) != -1)
			assert.True(t, util.IndexInSlice(res.items, "2", Strcmp) != -1)
		}
	})

	inst1.setNextPeerSelection([]string{"p2"})

	time.Sleep(time.Duration(2000) * time.Millisecond)
	assert.Equal(t, len(inst2.state.ToArray()), len(inst1.state.ToArray()))
}

func TestByzantineResponder(t *testing.T) {
	t.Parallel()
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

	inst2.Add("1", "2", "3")
	inst3.Add("1", "6", "7")

	inst2.hook(func(m interface{}) {
		if _, isHello := m.(*helloMsg); isHello {
			inst3.SendDigest([]string{"5", "6", "7"}, 0, "p1")
		}
	})

	inst1.hook(func(m interface{}) {
		if dig, isDig := m.(*digestMsg); isDig {
			if dig.source == "p3" {
				atomic.StoreInt32(&receivedDigestFromInst3, int32(1))
				time.AfterFunc(time.Duration(150)*time.Millisecond, func() {
					inst3.SendRes([]string{"5", "6", "7"}, "p1", 0)
				})
			}
		}

		if res, isRes := m.(*resMsg); isRes {
			// the response is from p3
			if util.IndexInSlice(res.items, "6", Strcmp) != -1 {
				// inst1 is currently accepting responses
				assert.Equal(t, int32(1), atomic.LoadInt32(&(inst1.acceptingResponses)), "inst1 is not accepting digests")
			}
		}
	})

	inst1.setNextPeerSelection([]string{"p2"})

	time.Sleep(time.Duration(1000) * time.Millisecond)

	assert.Equal(t, int32(1), atomic.LoadInt32(&receivedDigestFromInst3), "inst1 hasn't received a digest from inst3")

	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "1", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "2", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "3", Strcmp) != -1)

	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "5", Strcmp) == -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "6", Strcmp) == -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "7", Strcmp) == -1)

}

func TestMultipleInitiators(t *testing.T) {
	t.Parallel()
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

	inst4.Add("1", "2", "3", "4")
	inst1.setNextPeerSelection([]string{"p4"})
	inst2.setNextPeerSelection([]string{"p4"})
	inst3.setNextPeerSelection([]string{"p4"})

	time.Sleep(time.Duration(2000) * time.Millisecond)

	for _, inst := range []*pullTestInstance{inst1, inst2, inst3} {
		assert.True(t, util.IndexInSlice(inst.state.ToArray(), "1", Strcmp) != -1)
		assert.True(t, util.IndexInSlice(inst.state.ToArray(), "2", Strcmp) != -1)
		assert.True(t, util.IndexInSlice(inst.state.ToArray(), "3", Strcmp) != -1)
		assert.True(t, util.IndexInSlice(inst.state.ToArray(), "4", Strcmp) != -1)
	}

}

func TestLatePeers(t *testing.T) {
	t.Parallel()
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
	inst2.Add("1", "2", "3", "4")
	inst3.Add("5", "6", "7", "8")
	inst2.hook(func(m interface{}) {
		time.Sleep(time.Duration(600) * time.Millisecond)
	})
	inst1.setNextPeerSelection([]string{"p2", "p3"})

	time.Sleep(time.Duration(2000) * time.Millisecond)

	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "1", Strcmp) == -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "2", Strcmp) == -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "3", Strcmp) == -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "4", Strcmp) == -1)

	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "5", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "6", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "7", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "8", Strcmp) != -1)

}

func TestBiDiUpdates(t *testing.T) {
	t.Parallel()
	// Scenario: inst1 has {1, 3} and inst2 has {0,2} and both initiate to the other at the same time.
	// Expected outcome: both have {0,1,2,3} in the end
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	inst2 := newPushPullTestInstance("p2", peers)
	defer inst1.stop()
	defer inst2.stop()

	inst1.Add("1", "3")
	inst2.Add("0", "2")

	inst1.setNextPeerSelection([]string{"p2"})
	inst2.setNextPeerSelection([]string{"p1"})

	time.Sleep(time.Duration(2000) * time.Millisecond)

	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "0", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "1", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "2", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst1.state.ToArray(), "3", Strcmp) != -1)

	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), "0", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), "1", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), "2", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), "3", Strcmp) != -1)

}

func TestSpread(t *testing.T) {
	t.Parallel()
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
		item := fmt.Sprintf("%d", i)
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

func TestFilter(t *testing.T) {
	t.Parallel()
	// Scenario: 3 instances, items [0-5] are found only in the first instance, the other 2 have none.
	//           and also the first instance only gives the 2nd instance even items, and odd items to the 3rd.
	//           also, instances 2 and 3 don't know each other.
	// Expected outcome: inst2 has only even items, and inst3 has only odd items
	peers := make(map[string]*pullTestInstance)
	inst1 := newPushPullTestInstance("p1", peers)
	inst2 := newPushPullTestInstance("p2", peers)
	inst3 := newPushPullTestInstance("p3", peers)
	defer inst1.stop()
	defer inst2.stop()
	defer inst3.stop()

	inst1.PullEngine.digFilter = func(context interface{}) func(digestItem string) bool {
		return func(digestItem string) bool {
			n, _ := strconv.ParseInt(digestItem, 10, 64)
			if context == "p2" {
				return n%2 == 0
			}
			return n%2 == 1
		}
	}

	inst1.Add("0", "1", "2", "3", "4", "5")
	inst2.setNextPeerSelection([]string{"p1"})
	inst3.setNextPeerSelection([]string{"p1"})

	time.Sleep(time.Second * 2)

	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), "0", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), "1", Strcmp) == -1)
	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), "2", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), "3", Strcmp) == -1)
	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), "4", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst2.state.ToArray(), "5", Strcmp) == -1)

	assert.True(t, util.IndexInSlice(inst3.state.ToArray(), "0", Strcmp) == -1)
	assert.True(t, util.IndexInSlice(inst3.state.ToArray(), "1", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst3.state.ToArray(), "2", Strcmp) == -1)
	assert.True(t, util.IndexInSlice(inst3.state.ToArray(), "3", Strcmp) != -1)
	assert.True(t, util.IndexInSlice(inst3.state.ToArray(), "4", Strcmp) == -1)
	assert.True(t, util.IndexInSlice(inst3.state.ToArray(), "5", Strcmp) != -1)

}

func TestDefaultConfig(t *testing.T) {
	preDigestWaitTime := util.GetDurationOrDefault("peer.gossip.digestWaitTime", defDigestWaitTime)
	preRequestWaitTime := util.GetDurationOrDefault("peer.gossip.requestWaitTime", defRequestWaitTime)
	preResponseWaitTime := util.GetDurationOrDefault("peer.gossip.responseWaitTime", defResponseWaitTime)
	defer func() {
		SetDigestWaitTime(preDigestWaitTime)
		SetRequestWaitTime(preRequestWaitTime)
		SetResponseWaitTime(preResponseWaitTime)
	}()

	// Check if we can read default duration when no properties are
	// defined in config file.
	viper.Reset()
	assert.Equal(t, time.Duration(1000)*time.Millisecond, util.GetDurationOrDefault("peer.gossip.digestWaitTime", defDigestWaitTime))
	assert.Equal(t, time.Duration(1500)*time.Millisecond, util.GetDurationOrDefault("peer.gossip.requestWaitTime", defRequestWaitTime))
	assert.Equal(t, time.Duration(2000)*time.Millisecond, util.GetDurationOrDefault("peer.gossip.responseWaitTime", defResponseWaitTime))

	// Check if the properties in the config file (core.yaml)
	// are set to the desired duration.
	viper.Reset()
	viper.SetConfigName("core")
	viper.SetEnvPrefix("CORE")
	configtest.AddDevConfigPath(nil)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	err := viper.ReadInConfig()
	assert.NoError(t, err)
	assert.Equal(t, time.Duration(1000)*time.Millisecond, util.GetDurationOrDefault("peer.gossip.digestWaitTime", defDigestWaitTime))
	assert.Equal(t, time.Duration(1500)*time.Millisecond, util.GetDurationOrDefault("peer.gossip.requestWaitTime", defRequestWaitTime))
	assert.Equal(t, time.Duration(2000)*time.Millisecond, util.GetDurationOrDefault("peer.gossip.responseWaitTime", defResponseWaitTime))
}

func Strcmp(a interface{}, b interface{}) bool {
	return a.(string) == b.(string)
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
