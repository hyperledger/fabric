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
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
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
	cutter       blockcutter.Receiver
	rl           rawledger.Writer
	sendChan     chan *cb.Envelope
	exitChan     chan struct{}
}

func NewConsenter(batchSize int, batchTimeout time.Duration, rl rawledger.Writer, filters *broadcastfilter.RuleSet, configManager configtx.Manager) *consenter {
	bs := newPlainConsenter(batchSize, batchTimeout, rl, filters, configManager)
	go bs.main()
	return bs
}

func newPlainConsenter(batchSize int, batchTimeout time.Duration, rl rawledger.Writer, filters *broadcastfilter.RuleSet, configManager configtx.Manager) *consenter {
	bs := &consenter{
		cutter:       blockcutter.NewReceiverImpl(batchSize, filters, configManager),
		batchTimeout: batchTimeout,
		rl:           rl,
		sendChan:     make(chan *cb.Envelope),
		exitChan:     make(chan struct{}),
	}
	return bs
}

func (bs *consenter) halt() {
	close(bs.exitChan)
}

// Enqueue accepts a message and returns true on acceptance, or false on shutdown
func (bs *consenter) Enqueue(env *cb.Envelope) bool {
	select {
	case bs.sendChan <- env:
		return true
	case <-bs.exitChan:
		return false
	}
}

func (bs *consenter) main() {
	var timer <-chan time.Time

	for {
		select {
		case msg := <-bs.sendChan:
			batches, ok := bs.cutter.Ordered(msg)
			if ok && len(batches) == 0 && timer == nil {
				timer = time.After(bs.batchTimeout)
				continue
			}
			for _, batch := range batches {
				bs.rl.Append(batch, nil)
			}
		case <-timer:
			batch := bs.cutter.Cut()
			if len(batch) == 0 {
				logger.Warningf("Batch timer expired with no pending requests, this might indicate a bug")
				continue
			}
			logger.Debugf("Batch timer expired, creating block")
			bs.rl.Append(batch, nil)
		case <-bs.exitChan:
			logger.Debugf("Exiting")
			return
		}
	}
}
