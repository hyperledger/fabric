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
	"sync"

	"github.com/hyperledger/fabric/orderer/localconfig"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

// Deliverer allows the caller to receive blocks from the orderer
type Deliverer interface {
	Deliver(stream ab.AtomicBroadcast_DeliverServer) error
	Closeable
}

type delivererImpl struct {
	config   *config.TopLevel
	deadChan chan struct{}
	wg       sync.WaitGroup
}

func newDeliverer(conf *config.TopLevel) Deliverer {
	return &delivererImpl{
		config:   conf,
		deadChan: make(chan struct{}),
	}
}

// Deliver receives updates from connected clients and adjusts
// the transmission of ordered messages to them accordingly
func (d *delivererImpl) Deliver(stream ab.AtomicBroadcast_DeliverServer) error {
	cd := newClientDeliverer(d.config, d.deadChan)

	d.wg.Add(1)
	defer d.wg.Done()

	defer cd.Close()
	return cd.Deliver(stream)
}

// Close shuts down the delivery side of the orderer
func (d *delivererImpl) Close() error {
	close(d.deadChan)
	// Wait till all the client-deliverer consumers have closed
	// Note that their recvReplies goroutines keep on going
	d.wg.Wait()
	return nil
}
