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

package solo

import (
	"time"

	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/rawledger"
)

type broadcastServer struct {
	queueSize    int
	batchSize    int
	batchTimeout time.Duration
	rl           rawledger.Writer
	filter       *broadcastfilter.RuleSet
	sendChan     chan *ab.BroadcastMessage
	exitChan     chan struct{}
}

func newBroadcastServer(queueSize, batchSize int, batchTimeout time.Duration, rl rawledger.Writer) *broadcastServer {
	bs := newPlainBroadcastServer(queueSize, batchSize, batchTimeout, rl)
	go bs.main()
	return bs
}

func newPlainBroadcastServer(queueSize, batchSize int, batchTimeout time.Duration, rl rawledger.Writer) *broadcastServer {
	bs := &broadcastServer{
		queueSize:    queueSize,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		rl:           rl,
		filter:       broadcastfilter.NewRuleSet([]broadcastfilter.Rule{broadcastfilter.EmptyRejectRule, broadcastfilter.AcceptRule}),
		sendChan:     make(chan *ab.BroadcastMessage),
		exitChan:     make(chan struct{}),
	}
	return bs
}

func (bs *broadcastServer) halt() {
	close(bs.exitChan)
}

func (bs *broadcastServer) main() {
	var curBatch []*ab.BroadcastMessage
outer:
	for {
		timer := time.After(bs.batchTimeout)
		for {
			select {
			case msg := <-bs.sendChan:
				// The messages must be filtered a second time in case configuration has changed since the message was received
				action, _ := bs.filter.Apply(msg)
				switch action {
				case broadcastfilter.Accept:
					curBatch = append(curBatch, msg)
					if len(curBatch) < bs.batchSize {
						continue
					}
					logger.Debugf("Batch size met, creating block")
				case broadcastfilter.Forward:
					logger.Debugf("Ignoring message because it was not accepted by a filter")
				default:
					// TODO add support for other cases, unreachable for now
					logger.Fatalf("NOT IMPLEMENTED YET")
				}
			case <-timer:
				if len(curBatch) == 0 {
					continue outer
				}
				logger.Debugf("Batch timer expired, creating block")
			case <-bs.exitChan:
				logger.Debugf("Exiting")
				return
			}
			break
		}

		bs.rl.Append(curBatch, nil)
		curBatch = nil
	}
}

func (bs *broadcastServer) handleBroadcast(srv ab.AtomicBroadcast_BroadcastServer) error {
	b := newBroadcaster(bs)
	defer close(b.queue)
	go b.drainQueue()
	return b.queueBroadcastMessages(srv)
}

type broadcaster struct {
	bs    *broadcastServer
	queue chan *ab.BroadcastMessage
}

func (b *broadcaster) drainQueue() {
	for {
		select {
		case msg, ok := <-b.queue:
			if ok {
				select {
				case b.bs.sendChan <- msg:
				case <-b.bs.exitChan:
					return
				}
			} else {
				return
			}
		case <-b.bs.exitChan:
			return
		}
	}
}

func (b *broadcaster) queueBroadcastMessages(srv ab.AtomicBroadcast_BroadcastServer) error {

	for {
		msg, err := srv.Recv()
		if err != nil {
			return err
		}

		action, _ := b.bs.filter.Apply(msg)

		switch action {
		case broadcastfilter.Accept:
			select {
			case b.queue <- msg:
				err = srv.Send(&ab.BroadcastResponse{ab.Status_SUCCESS})
			default:
				err = srv.Send(&ab.BroadcastResponse{ab.Status_SERVICE_UNAVAILABLE})
			}
		case broadcastfilter.Forward:
			fallthrough
		case broadcastfilter.Reject:
			err = srv.Send(&ab.BroadcastResponse{ab.Status_BAD_REQUEST})
		default:
			// TODO add support for other cases, unreachable for now
			logger.Fatalf("NOT IMPLEMENTED YET")
		}

		if err != nil {
			return err
		}
	}
}

func newBroadcaster(bs *broadcastServer) *broadcaster {
	b := &broadcaster{
		bs:    bs,
		queue: make(chan *ab.BroadcastMessage, bs.queueSize),
	}
	return b
}
