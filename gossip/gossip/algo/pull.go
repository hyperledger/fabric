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
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/gossip/util"
)

/* PullEngine is an object that performs pull-based gossip, and maintains an internal state of items
   identified by uint64 numbers.
   The protocol is as follows:
   1) The Initiator sends a Hello message with a specific NONCE to a set of remote peers.
   2) Each remote peer responds with a digest of its messages and returns that NONCE.
   3) The initiator checks the validity of the NONCEs received, aggregates the digests,
      and crafts a request containing specific item ids it wants to receive from each remote peer and then
      sends each request to its corresponding peer.
   4) Each peer sends back the response containing the items requested, if it still holds them and the NONCE.

    Other peer				   			   Initiator
	 O	<-------- Hello <NONCE> -------------------------	O
	/|\	--------- Digest <[3,5,8, 10...], NONCE> -------->     /|\
	 |	<-------- Request <[3,8], NONCE> -----------------      |
	/ \	--------- Response <[item3, item8], NONCE>------->     / \

*/

const (
	DEF_DIGEST_WAIT_TIME   = time.Duration(4) * time.Second
	DEF_REQUEST_WAIT_TIME  = time.Duration(4) * time.Second
	DEF_RESPONSE_WAIT_TIME = time.Duration(7) * time.Second
)

func init() {
	rand.Seed(42)
}

var defaultDigestWaitTime = DEF_DIGEST_WAIT_TIME
var defaultRequestWaitTime = DEF_REQUEST_WAIT_TIME
var defaultResponseWaitTime = DEF_RESPONSE_WAIT_TIME

// PullAdapter is needed by the PullEngine in order to
// send messages to the remote PullEngine instances.
// The PullEngine expects to be invoked with
// OnHello, OnDigest, OnReq, OnRes when the respective message arrives
// from a remote PullEngine
type PullAdapter interface {
	// SelectPeers returns a slice of peers which the engine will initiate the protocol with
	SelectPeers() []string

	// Hello sends a hello message to initiate the protocol
	// and returns an NONCE that is expected to be returned
	// in the digest message.
	Hello(dest string, nonce uint64)

	// SendDigest sends a digest to a remote PullEngine.
	// The context parameter specifies the remote engine to send to.
	SendDigest(digest []uint64, nonce uint64, context interface{})

	// SendReq sends an array of items to a certain remote PullEngine identified
	// by a string
	SendReq(dest string, items []uint64, nonce uint64)

	// SendRes sends an array of items to a remote PullEngine identified by a context.
	SendRes(items []uint64, context interface{}, nonce uint64)
}

type PullEngine struct {
	PullAdapter
	stopFlag           int32
	state              *util.Set
	item2owners        map[uint64][]string
	peers2nonces       map[string]uint64
	nonces2peers       map[uint64]string
	acceptingDigests   int32
	acceptingResponses int32
	lock               sync.Mutex
	nonces             *util.Set
}

func NewPullEngine(participant PullAdapter, sleepTime time.Duration) *PullEngine {
	engine := &PullEngine{
		PullAdapter:    participant,
		stopFlag:           int32(0),
		state:              util.NewSet(),
		item2owners:        make(map[uint64][]string),
		peers2nonces:       make(map[string]uint64),
		nonces2peers:       make(map[uint64]string),
		acceptingDigests:   int32(0),
		acceptingResponses: int32(0),
		nonces:             util.NewSet(),
	}

	go func() {
		for !engine.toDie() {
			time.Sleep(sleepTime)
			engine.initiatePull()

		}
	}()

	return engine
}

func (engine *PullEngine) toDie() bool {
	return (atomic.LoadInt32(&(engine.stopFlag)) == int32(1))
}

func (engine *PullEngine) acceptResponses() {
	atomic.StoreInt32(&(engine.acceptingResponses), int32(1))
}

func (engine *PullEngine) isAcceptingResponses() bool {
	return atomic.LoadInt32(&(engine.acceptingResponses)) == int32(1)
}

func (engine *PullEngine) acceptDigests() {
	atomic.StoreInt32(&(engine.acceptingDigests), int32(1))
}

func (engine *PullEngine) isAcceptingDigests() bool {
	return atomic.LoadInt32(&(engine.acceptingDigests)) == int32(1)
}

func (engine *PullEngine) ignoreDigests() {
	atomic.StoreInt32(&(engine.acceptingDigests), int32(0))
}

func (engine *PullEngine) Stop() {
	atomic.StoreInt32(&(engine.stopFlag), int32(1))
}

func (engine *PullEngine) initiatePull() {
	engine.lock.Lock()
	defer engine.lock.Unlock()

	engine.acceptDigests()
	for _, peer := range engine.SelectPeers() {
		nonce := engine.newNONCE()
		engine.nonces.Add(nonce)
		engine.nonces2peers[nonce] = peer
		engine.peers2nonces[peer] = nonce
		engine.Hello(peer, nonce)
	}

	time.AfterFunc(defaultDigestWaitTime, func() {
		engine.processIncomingDigests()
	})
}

func (engine *PullEngine) processIncomingDigests() {
	engine.ignoreDigests()

	engine.lock.Lock()
	defer engine.lock.Unlock()

	requestMapping := make(map[string][]uint64)
	for n, sources := range engine.item2owners {
		// select a random source
		source := sources[rand.Intn(len(sources))]
		if _, exists := requestMapping[source]; !exists {
			requestMapping[source] = make([]uint64, 0)
		}
		// append the number to that source
		requestMapping[source] = append(requestMapping[source], n)
	}

	engine.acceptResponses()

	for dest, seqsToReq := range requestMapping {
		engine.SendReq(dest, seqsToReq, engine.peers2nonces[dest])
	}

	time.AfterFunc(defaultResponseWaitTime, engine.endPull)

}

func (engine *PullEngine) endPull() {
	engine.lock.Lock()
	defer engine.lock.Unlock()

	atomic.StoreInt32(&(engine.acceptingResponses), int32(0))
	engine.nonces.Clear()

	engine.item2owners = make(map[uint64][]string)
	engine.peers2nonces = make(map[string]uint64)
	engine.nonces2peers = make(map[uint64]string)
}

func (engine *PullEngine) OnDigest(digest []uint64, nonce uint64, context interface{}) {
	if !engine.isAcceptingDigests() || !engine.nonces.Exists(nonce) {
		return
	}

	engine.lock.Lock()
	defer engine.lock.Unlock()

	for _, n := range digest {
		if engine.state.Exists(n) {
			continue
		}

		if _, exists := engine.item2owners[n]; !exists {
			engine.item2owners[n] = make([]string, 0)
		}

		engine.item2owners[n] = append(engine.item2owners[n], engine.nonces2peers[nonce])
	}
}

func (engine *PullEngine) Add(seqs ...uint64) {
	for _, seq := range seqs {
		engine.state.Add(seq)
	}
}

func (engine *PullEngine) Remove(seqs ...uint64) {
	for _, seq := range seqs {
		engine.state.Remove(seq)
	}
}

func (engine *PullEngine) OnHello(nonce uint64, context interface{}) {
	engine.nonces.Add(nonce)
	time.AfterFunc(defaultRequestWaitTime, func() {
		engine.nonces.Remove(nonce)
	})

	a := engine.state.ToArray()
	digest := make([]uint64, len(a))
	for i, item := range a {
		digest[i] = item.(uint64)
	}
	engine.SendDigest(digest, nonce, context)
}

func (engine *PullEngine) OnReq(items []uint64, nonce uint64, context interface{}) {
	if !engine.nonces.Exists(nonce) {
		return
	}
	engine.lock.Lock()
	defer engine.lock.Unlock()

	items2Send := make([]uint64, 0)
	for _, item := range items {
		if engine.state.Exists(item) {
			items2Send = append(items2Send, item)
		}
	}

	engine.SendRes(items2Send, context, nonce)
}

func (engine *PullEngine) OnRes(items []uint64, nonce uint64) {
	if !engine.nonces.Exists(nonce) || !engine.isAcceptingResponses() {
		return
	}

	engine.Add(items...)
}

func (engine *PullEngine) newNONCE() uint64 {
	n := uint64(0)
	for {
		n = uint64(rand.Int63())
		if !engine.nonces.Exists(n) {
			return n
		}
	}
}
