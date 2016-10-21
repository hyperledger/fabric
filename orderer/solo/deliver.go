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
	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
	"github.com/hyperledger/fabric/orderer/rawledger"
)

type deliverServer struct {
	rl        rawledger.Reader
	maxWindow int
}

func newDeliverServer(rl rawledger.Reader, maxWindow int) *deliverServer {
	return &deliverServer{
		rl:        rl,
		maxWindow: maxWindow,
	}
}

func (ds *deliverServer) handleDeliver(srv ab.AtomicBroadcast_DeliverServer) error {
	logger.Debugf("Starting new Deliver loop")
	d := newDeliverer(ds, srv)
	return d.recv()

}

type deliverer struct {
	ds              *deliverServer
	srv             ab.AtomicBroadcast_DeliverServer
	cursor          rawledger.Iterator
	nextBlockNumber uint64
	windowSize      uint64
	lastAck         uint64
	recvChan        chan *ab.DeliverUpdate
	exitChan        chan struct{}
}

func newDeliverer(ds *deliverServer, srv ab.AtomicBroadcast_DeliverServer) *deliverer {
	d := &deliverer{
		ds:       ds,
		srv:      srv,
		exitChan: make(chan struct{}),
		recvChan: make(chan *ab.DeliverUpdate),
	}
	go d.main()
	return d
}

func (d *deliverer) halt() {
	close(d.exitChan)
}

func (d *deliverer) main() {
	var signal <-chan struct{}
	for {
		select {
		case update := <-d.recvChan:
			logger.Debugf("Receiving message %v", update)
			switch t := update.Type.(type) {
			case *ab.DeliverUpdate_Acknowledgement:
				logger.Debugf("Received acknowledgement from client")
				d.lastAck = t.Acknowledgement.Number
			case *ab.DeliverUpdate_Seek:
				if !d.processUpdate(t.Seek) {
					return
				}
			case nil:
				logger.Errorf("Nil update")
				close(d.exitChan)
				return
			default:
				logger.Errorf("Unknown type: %T:%v", t, t)
				close(d.exitChan)
				return
			}
		case <-signal:
			block, status := d.cursor.Next()
			if status != ab.Status_SUCCESS {
				logger.Errorf("Error reading from channel, cause was: %v", status)
				if !d.sendErrorReply(status) {
					return
				}
				d.cursor = nil
			} else {
				d.nextBlockNumber = block.Number + 1
				if !d.sendBlockReply(block) {
					return
				}
			}
		case <-d.exitChan:
			return
		}

		if d.cursor == nil {
			signal = nil
			continue
		}

		if d.lastAck+d.windowSize < d.nextBlockNumber {
			signal = nil
			continue
		}

		logger.Debugf("Room for more blocks, activating channel")
		signal = d.cursor.ReadyChan()
	}
}

func (d *deliverer) recv() error {
	for {
		msg, err := d.srv.Recv()
		if err != nil {
			return err
		}
		logger.Debugf("Received message %v", msg)
		select {
		case <-d.exitChan:
			return nil // something has gone wrong enough we want to disconnect
		case d.recvChan <- msg:
			logger.Debugf("Sent update")
		}
	}
}

func (d *deliverer) sendErrorReply(status ab.Status) bool {
	err := d.srv.Send(&ab.DeliverResponse{
		Type: &ab.DeliverResponse_Error{Error: status},
	})

	if err != nil {
		close(d.exitChan)
		return false
	}

	return true

}

func (d *deliverer) sendBlockReply(block *ab.Block) bool {
	err := d.srv.Send(&ab.DeliverResponse{
		Type: &ab.DeliverResponse_Block{Block: block},
	})

	if err != nil {
		close(d.exitChan)
		return false
	}

	return true

}

func (d *deliverer) processUpdate(update *ab.SeekInfo) bool {
	if d.cursor != nil {
		d.cursor = nil
	}
	logger.Debugf("Updating properties for client")

	if update == nil || update.WindowSize == 0 || update.WindowSize > uint64(d.ds.maxWindow) {
		close(d.exitChan)
		return d.sendErrorReply(ab.Status_BAD_REQUEST)
	}

	d.windowSize = update.WindowSize

	d.cursor, d.nextBlockNumber = d.ds.rl.Iterator(update.Start, update.SpecifiedNumber)
	d.lastAck = d.nextBlockNumber - 1

	return true
}
