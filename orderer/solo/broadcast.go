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

	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/golang/protobuf/proto"
)

type broadcastServer struct {
	queueSize     int
	batchSize     int
	batchTimeout  time.Duration
	rl            rawledger.Writer
	filter        *broadcastfilter.RuleSet
	configManager configtx.Manager
	sendChan      chan *cb.Envelope
	exitChan      chan struct{}
}

func newBroadcastServer(queueSize, batchSize int, batchTimeout time.Duration, rl rawledger.Writer, filters *broadcastfilter.RuleSet, configManager configtx.Manager) *broadcastServer {
	bs := newPlainBroadcastServer(queueSize, batchSize, batchTimeout, rl, filters, configManager)
	go bs.main()
	return bs
}

func newPlainBroadcastServer(queueSize, batchSize int, batchTimeout time.Duration, rl rawledger.Writer, filters *broadcastfilter.RuleSet, configManager configtx.Manager) *broadcastServer {
	bs := &broadcastServer{
		queueSize:     queueSize,
		batchSize:     batchSize,
		batchTimeout:  batchTimeout,
		rl:            rl,
		filter:        filters,
		configManager: configManager,
		sendChan:      make(chan *cb.Envelope),
		exitChan:      make(chan struct{}),
	}
	return bs
}

func (bs *broadcastServer) halt() {
	close(bs.exitChan)
}

func (bs *broadcastServer) main() {
	var curBatch []*cb.Envelope
	var timer <-chan time.Time

	cutBatch := func() {
		bs.rl.Append(curBatch, nil)
		curBatch = nil
		timer = nil
	}

	for {
		select {
		case msg := <-bs.sendChan:
			// The messages must be filtered a second time in case configuration has changed since the message was received
			action, _ := bs.filter.Apply(msg)
			switch action {
			case broadcastfilter.Accept:
				curBatch = append(curBatch, msg)

				if len(curBatch) >= bs.batchSize {
					logger.Debugf("Batch size met, creating block")
					cutBatch()
				} else if len(curBatch) == 1 {
					// If this is the first request in a batch, start the batch timer
					timer = time.After(bs.batchTimeout)
				}
			case broadcastfilter.Reconfigure:
				// TODO, this is unmarshaling for a second time, we need a cleaner interface, maybe Apply returns a second arg with thing to put in the batch
				payload := &cb.Payload{}
				if err := proto.Unmarshal(msg.Payload, payload); err != nil {
					logger.Errorf("A change was flagged as configuration, but could not be unmarshaled: %v", err)
					continue
				}
				newConfig := &cb.ConfigurationEnvelope{}
				if err := proto.Unmarshal(payload.Data, newConfig); err != nil {
					logger.Errorf("A change was flagged as configuration, but could not be unmarshaled: %v", err)
					continue
				}
				err := bs.configManager.Apply(newConfig)
				if err != nil {
					logger.Warningf("A configuration change made it through the ingress filter but could not be included in a batch: %v", err)
					continue
				}

				logger.Debugf("Configuration change applied successfully, committing previous block and configuration block")
				cutBatch()
				bs.rl.Append([]*cb.Envelope{msg}, nil)
			case broadcastfilter.Reject:
				fallthrough
			case broadcastfilter.Forward:
				logger.Debugf("Ignoring message because it was not accepted by a filter")
			default:
				logger.Fatalf("Received an unknown rule response: %v", action)
			}
		case <-timer:
			if len(curBatch) == 0 {
				logger.Warningf("Batch timer expired with no pending requests, this might indicate a bug")
				continue
			}
			logger.Debugf("Batch timer expired, creating block")
			cutBatch()
		case <-bs.exitChan:
			logger.Debugf("Exiting")
			return
		}
	}
}

func (bs *broadcastServer) handleBroadcast(srv ab.AtomicBroadcast_BroadcastServer) error {
	b := newBroadcaster(bs)
	defer close(b.queue)
	go b.drainQueue()
	return b.queueEnvelopes(srv)
}

type broadcaster struct {
	bs    *broadcastServer
	queue chan *cb.Envelope
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

func (b *broadcaster) queueEnvelopes(srv ab.AtomicBroadcast_BroadcastServer) error {

	for {
		msg, err := srv.Recv()
		if err != nil {
			return err
		}

		action, _ := b.bs.filter.Apply(msg)

		switch action {
		case broadcastfilter.Reconfigure:
			fallthrough
		case broadcastfilter.Accept:
			select {
			case b.queue <- msg:
				err = srv.Send(&ab.BroadcastResponse{Status: cb.Status_SUCCESS})
			default:
				err = srv.Send(&ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE})
			}
		case broadcastfilter.Forward:
			fallthrough
		case broadcastfilter.Reject:
			err = srv.Send(&ab.BroadcastResponse{Status: cb.Status_BAD_REQUEST})
		default:
			logger.Fatalf("Unknown filter action :%v", action)
		}

		if err != nil {
			return err
		}
	}
}

func newBroadcaster(bs *broadcastServer) *broadcaster {
	b := &broadcaster{
		bs:    bs,
		queue: make(chan *cb.Envelope, bs.queueSize),
	}
	return b
}
