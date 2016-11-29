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

	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/multichain"
	"github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/solo")

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

type consenter struct {
	batchTimeout time.Duration
}

type chain struct {
	batchTimeout time.Duration
	rl           rawledger.Writer
	cutter       blockcutter.Receiver
	sendChan     chan *cb.Envelope
	exitChan     chan struct{}
}

func New(batchTimeout time.Duration) multichain.Consenter {
	return &consenter{
		// TODO, ultimately this should come from the configManager at HandleChain
		batchTimeout: batchTimeout,
	}
}

func (solo *consenter) HandleChain(configManager configtx.Manager, cutter blockcutter.Receiver, rl rawledger.Writer, metadata []byte) multichain.Chain {
	return newChain(solo.batchTimeout, configManager, cutter, rl, metadata)
}

func newChain(batchTimeout time.Duration, configManager configtx.Manager, cutter blockcutter.Receiver, rl rawledger.Writer, metadata []byte) *chain {
	return &chain{
		batchTimeout: batchTimeout,
		rl:           rl,
		cutter:       cutter,
		sendChan:     make(chan *cb.Envelope),
		exitChan:     make(chan struct{}),
	}
}

func (ch *chain) Start() {
	go ch.main()
}

func (ch *chain) Halt() {
	close(ch.exitChan)
}

// Enqueue accepts a message and returns true on acceptance, or false on shutdown
func (ch *chain) Enqueue(env *cb.Envelope) bool {
	select {
	case ch.sendChan <- env:
		return true
	case <-ch.exitChan:
		return false
	}
}

func (ch *chain) main() {
	var timer <-chan time.Time

	for {
		select {
		case msg := <-ch.sendChan:
			batches, ok := ch.cutter.Ordered(msg)
			if ok && len(batches) == 0 && timer == nil {
				timer = time.After(ch.batchTimeout)
				continue
			}
			for _, batch := range batches {
				ch.rl.Append(batch, nil)
			}
			if len(batches) > 0 {
				timer = nil
			}
		case <-timer:
			//clear the timer
			timer = nil

			batch := ch.cutter.Cut()
			if len(batch) == 0 {
				logger.Warningf("Batch timer expired with no pending requests, this might indicate a bug")
				continue
			}
			logger.Debugf("Batch timer expired, creating block")
			ch.rl.Append(batch, nil)
		case <-ch.exitChan:
			logger.Debugf("Exiting")
			return
		}
	}
}
