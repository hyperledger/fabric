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

	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/orderer/config"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/golang/protobuf/proto"
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

	batchChan  chan *cb.Envelope
	messages   [][]byte
	nextNumber uint64
	prevHash   []byte
}

func newBroadcaster(conf *config.TopLevel) Broadcaster {
	return &broadcasterImpl{
		producer:   newProducer(conf),
		config:     conf,
		batchChan:  make(chan *cb.Envelope, conf.General.BatchSize),
		messages:   [][]byte{[]byte("genesis")},
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
	data := &cb.BlockData{
		Data: b.messages,
	}
	block := &cb.Block{
		Header: &cb.BlockHeader{
			Number:       b.nextNumber,
			PreviousHash: b.prevHash,
			DataHash:     data.Hash(),
		},
		Data: data,
	}
	logger.Debugf("Prepared block %d with %d messages (%+v)", block.Header.Number, len(block.Data.Data), block)

	b.messages = [][]byte{}
	b.nextNumber++
	b.prevHash = block.Header.Hash()

	blockBytes, err := proto.Marshal(block)

	if err != nil {
		logger.Fatalf("Error marshaling block: %s", err)
	}

	return b.producer.Send(blockBytes)
}

func (b *broadcasterImpl) cutBlock(period time.Duration, maxSize uint) {
	timer := time.NewTimer(period)

	for {
		select {
		case msg := <-b.batchChan:
			data, err := proto.Marshal(msg)
			if err != nil {
				panic(fmt.Errorf("Error marshaling what should be a valid proto message: %s", err))
			}
			b.messages = append(b.messages, data)
			if len(b.messages) >= int(maxSize) {
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(period)
				if err := b.sendBlock(); err != nil {
					panic(fmt.Errorf("Cannot communicate with Kafka broker: %s", err))
				}
			}
		case <-timer.C:
			timer.Reset(period)
			if len(b.messages) > 0 {
				if err := b.sendBlock(); err != nil {
					panic(fmt.Errorf("Cannot communicate with Kafka broker: %s", err))
				}
			}
		}
	}
}

func (b *broadcasterImpl) recvRequests(stream ab.AtomicBroadcast_BroadcastServer) error {
	context, cancel := context.WithCancel(context.Background())
	defer cancel()
	bsr := newBroadcastSessionResponder(context, stream, b.config.General.QueueSize)
	for {
		msg, err := stream.Recv()
		if err != nil {
			logger.Debug("Can no longer receive requests from client (exited?)")
			return err
		}

		b.batchChan <- msg
		bsr.reply(cb.Status_SUCCESS) // TODO This shouldn't always be a success

	}
}

type broadcastSessionResponder struct {
	queue chan *ab.BroadcastResponse
}

func newBroadcastSessionResponder(context context.Context, stream ab.AtomicBroadcast_BroadcastServer, queueSize uint) *broadcastSessionResponder {
	bsr := &broadcastSessionResponder{
		queue: make(chan *ab.BroadcastResponse, queueSize),
	}
	go bsr.sendReplies(context, stream)
	return bsr
}

func (bsr *broadcastSessionResponder) reply(status cb.Status) {
	bsr.queue <- &ab.BroadcastResponse{Status: status}
}

func (bsr *broadcastSessionResponder) sendReplies(context context.Context, stream ab.AtomicBroadcast_BroadcastServer) {
	for {
		select {
		case reply := <-bsr.queue:
			if err := stream.Send(reply); err != nil {
				logger.Info("Cannot send broadcast reply to client")
			}
			logger.Debugf("Sent broadcast reply %v to client", reply.Status.String())
		case <-context.Done():
			return
		}
	}
}
