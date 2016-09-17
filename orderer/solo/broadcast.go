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
	"github.com/hyperledger/fabric/orderer/rawledger"
)

type broadcastServer struct {
	queue        chan *ab.BroadcastMessage
	batchSize    int
	batchTimeout time.Duration
	rl           rawledger.Writer
	exitChan     chan struct{}
}

func newBroadcastServer(queueSize, batchSize int, batchTimeout time.Duration, rl rawledger.Writer) *broadcastServer {
	bs := newPlainBroadcastServer(queueSize, batchSize, batchTimeout, rl)
	go bs.main()
	return bs
}

func newPlainBroadcastServer(queueSize, batchSize int, batchTimeout time.Duration, rl rawledger.Writer) *broadcastServer {
	bs := &broadcastServer{
		queue:        make(chan *ab.BroadcastMessage, queueSize),
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		rl:           rl,
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
			case msg := <-bs.queue:
				curBatch = append(curBatch, msg)
				if len(curBatch) < bs.batchSize {
					continue
				}
				logger.Debugf("Batch size met, creating block")
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
	for {
		msg, err := srv.Recv()
		if err != nil {
			return err
		}

		if msg.Data == nil {
			err = srv.Send(&ab.BroadcastResponse{ab.Status_BAD_REQUEST})
			if err != nil {
				return err
			}
		}

		select {
		case bs.queue <- msg:
			err = srv.Send(&ab.BroadcastResponse{ab.Status_SUCCESS})
		default:
			err = srv.Send(&ab.BroadcastResponse{ab.Status_SERVICE_UNAVAILABLE})
		}

		if err != nil {
			return err
		}
	}
}
