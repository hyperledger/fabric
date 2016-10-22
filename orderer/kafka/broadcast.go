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

package kafka

import (
	"fmt"
	"sync"
	"time"

	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
	"github.com/hyperledger/fabric/orderer/config"
)

// Broadcaster allows the caller to submit messages to the orderer
type Broadcaster interface {
	Broadcast(stream ab.AtomicBroadcast_BroadcastServer) error
	Closeable
}

type broadcasterImpl struct {
	producer Producer
	config   *config.TopLevel
	once     sync.Once

	batchChan  chan *ab.BroadcastMessage
	messages   []*ab.BroadcastMessage
	nextNumber uint64
	prevHash   []byte
}

func newBroadcaster(conf *config.TopLevel) Broadcaster {
	return &broadcasterImpl{
		producer:   newProducer(conf),
		config:     conf,
		batchChan:  make(chan *ab.BroadcastMessage, conf.General.BatchSize),
		messages:   []*ab.BroadcastMessage{&ab.BroadcastMessage{Data: []byte("genesis")}},
		nextNumber: 0,
	}
}

// Broadcast receives ordering requests by clients and sends back an
// acknowledgement for each received message in order, indicating
// success or type of failure
func (b *broadcasterImpl) Broadcast(stream ab.AtomicBroadcast_BroadcastServer) error {
	b.once.Do(func() {
		// Send the genesis block to create the topic
		// otherwise consumers will throw an exception.
		b.sendBlock()
		// Spawn the goroutine that cuts blocks
		go b.cutBlock(b.config.General.BatchTimeout, b.config.General.BatchSize)
	})
	return b.recvRequests(stream)
}

// Close shuts down the broadcast side of the orderer
func (b *broadcasterImpl) Close() error {
	if b.producer != nil {
		return b.producer.Close()
	}
	return nil
}

func (b *broadcasterImpl) sendBlock() error {
	block := &ab.Block{
		Messages: b.messages,
		Number:   b.nextNumber,
		PrevHash: b.prevHash,
	}
	logger.Debugf("Prepared block %d with %d messages (%+v)", block.Number, len(block.Messages), block)

	b.messages = []*ab.BroadcastMessage{}
	b.nextNumber++
	hash, data := hashBlock(block)
	b.prevHash = hash

	return b.producer.Send(data)
}

func (b *broadcasterImpl) cutBlock(period time.Duration, maxSize uint) {
	timer := time.NewTimer(period)

	for {
		select {
		case msg := <-b.batchChan:
			b.messages = append(b.messages, msg)
			if len(b.messages) >= int(maxSize) {
				if err := b.sendBlock(); err != nil {
					panic(fmt.Errorf("Cannot communicate with Kafka broker: %s", err))
				}
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(period)
			}
		case <-timer.C:
			if len(b.messages) > 0 {
				if err := b.sendBlock(); err != nil {
					panic(fmt.Errorf("Cannot communicate with Kafka broker: %s", err))
				}
			}
		}
	}
}

func (b *broadcasterImpl) recvRequests(stream ab.AtomicBroadcast_BroadcastServer) error {
	reply := new(ab.BroadcastResponse)
	for {
		msg, err := stream.Recv()
		if err != nil {
			logger.Debug("Can no longer receive requests from client (exited?)")
			return err
		}

		b.batchChan <- msg
		reply.Status = ab.Status_SUCCESS // TODO This shouldn't always be a success

		if err := stream.Send(reply); err != nil {
			logger.Info("Cannot send broadcast reply to client")
			return err
		}
		logger.Debugf("Sent broadcast reply %v to client", reply.Status.String())
	}
}
