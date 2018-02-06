/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package deliverclient

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// broadcastSetup is a function that is called by the broadcastClient immediately after each
// successful connection to the ordering service
type broadcastSetup func(blocksprovider.BlocksDeliverer) error

// retryPolicy receives as parameters the number of times the attempt has failed
// and a duration that specifies the total elapsed time passed since the first attempt.
// If further attempts should be made, it returns:
// 	- a time duration after which the next attempt would be made, true
// Else, a zero duration, false
type retryPolicy func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool)

// clientFactory creates a gRPC broadcast client out of a ClientConn
type clientFactory func(*grpc.ClientConn) orderer.AtomicBroadcastClient

type broadcastClient struct {
	stopFlag int32
	sync.Mutex
	stopChan     chan struct{}
	createClient clientFactory
	shouldRetry  retryPolicy
	onConnect    broadcastSetup
	prod         comm.ConnectionProducer
	blocksprovider.BlocksDeliverer
	conn *connection
}

// NewBroadcastClient returns a broadcastClient with the given params
func NewBroadcastClient(prod comm.ConnectionProducer, clFactory clientFactory, onConnect broadcastSetup, bos retryPolicy) *broadcastClient {
	return &broadcastClient{prod: prod, onConnect: onConnect, shouldRetry: bos, createClient: clFactory, stopChan: make(chan struct{}, 1)}
}

// Recv receives a message from the ordering service
func (bc *broadcastClient) Recv() (*orderer.DeliverResponse, error) {
	o, err := bc.try(func() (interface{}, error) {
		if bc.shouldStop() {
			return nil, errors.New("closing")
		}
		return bc.BlocksDeliverer.Recv()
	})
	if err != nil {
		return nil, err
	}
	return o.(*orderer.DeliverResponse), nil
}

// Send sends a message to the ordering service
func (bc *broadcastClient) Send(msg *common.Envelope) error {
	_, err := bc.try(func() (interface{}, error) {
		if bc.shouldStop() {
			return nil, errors.New("closing")
		}
		return nil, bc.BlocksDeliverer.Send(msg)
	})
	return err
}

func (bc *broadcastClient) try(action func() (interface{}, error)) (interface{}, error) {
	attempt := 0
	var totalRetryTime time.Duration
	var backoffDuration time.Duration
	retry := true
	for retry && !bc.shouldStop() {
		attempt++
		resp, err := bc.doAction(action)
		if err != nil {
			backoffDuration, retry = bc.shouldRetry(attempt, totalRetryTime)
			if !retry {
				logger.Warning("Got error:", err, "at", attempt, "attempt. Ceasing to retry")
				break
			}
			logger.Warning("Got error:", err, ", at", attempt, "attempt. Retrying in", backoffDuration)
			totalRetryTime += backoffDuration
			bc.sleep(backoffDuration)
			continue
		}
		return resp, nil
	}
	if bc.shouldStop() {
		return nil, errors.New("Client is closing")
	}
	return nil, fmt.Errorf("Attempts (%d) or elapsed time (%v) exhausted", attempt, totalRetryTime)
}

func (bc *broadcastClient) doAction(action func() (interface{}, error)) (interface{}, error) {
	if bc.conn == nil {
		err := bc.connect()
		if err != nil {
			return nil, err
		}
	}
	resp, err := action()
	if err != nil {
		bc.Disconnect()
		return nil, err
	}
	return resp, nil
}

func (bc *broadcastClient) sleep(duration time.Duration) {
	select {
	case <-time.After(duration):
	case <-bc.stopChan:
	}
}

func (bc *broadcastClient) connect() error {
	conn, endpoint, err := bc.prod.NewConnection()
	logger.Debug("Connected to", endpoint)
	if err != nil {
		logger.Error("Failed obtaining connection:", err)
		return err
	}
	ctx, cf := context.WithCancel(context.Background())
	logger.Debug("Establishing gRPC stream with", endpoint, "...")
	abc, err := bc.createClient(conn).Deliver(ctx)
	if err != nil {
		logger.Error("Connection to ", endpoint, "established but was unable to create gRPC stream:", err)
		conn.Close()
		return err
	}
	err = bc.afterConnect(conn, abc, cf)
	if err == nil {
		return nil
	}
	logger.Warning("Failed running post-connection procedures:", err)
	// If we reached here, lets make sure connection is closed
	// and nullified before we return
	bc.Disconnect()
	return err
}

func (bc *broadcastClient) afterConnect(conn *grpc.ClientConn, abc orderer.AtomicBroadcast_DeliverClient, cf context.CancelFunc) error {
	logger.Debug("Entering")
	defer logger.Debug("Exiting")
	bc.Lock()
	bc.conn = &connection{ClientConn: conn, cancel: cf}
	bc.BlocksDeliverer = abc
	if bc.shouldStop() {
		bc.Unlock()
		return errors.New("closing")
	}
	bc.Unlock()
	// If the client is closed at this point- before onConnect,
	// any use of this object by onConnect would return an error.
	err := bc.onConnect(bc)
	// If the client is closed right after onConnect, but before
	// the following lock- this method would return an error because
	// the client has been closed.
	bc.Lock()
	defer bc.Unlock()
	if bc.shouldStop() {
		return errors.New("closing")
	}
	// If the client is closed right after this method exits,
	// it's because this method returned nil and not an error.
	// So- connect() would return nil also, and the flow of the goroutine
	// is returned to doAction(), where action() is invoked - and is configured
	// to check whether the client has closed or not.
	if err == nil {
		return nil
	}
	logger.Error("Failed setting up broadcast:", err)
	return err
}

func (bc *broadcastClient) shouldStop() bool {
	return atomic.LoadInt32(&bc.stopFlag) == int32(1)
}

// Close makes the client close its connection and shut down
func (bc *broadcastClient) Close() {
	logger.Debug("Entering")
	defer logger.Debug("Exiting")
	bc.Lock()
	defer bc.Unlock()
	if bc.shouldStop() {
		return
	}
	atomic.StoreInt32(&bc.stopFlag, int32(1))
	bc.stopChan <- struct{}{}
	if bc.conn == nil {
		return
	}
	bc.conn.Close()
}

// Disconnect makes the client close the existing connection
func (bc *broadcastClient) Disconnect() {
	logger.Debug("Entering")
	defer logger.Debug("Exiting")
	bc.Lock()
	defer bc.Unlock()
	if bc.conn == nil {
		return
	}
	bc.conn.Close()
	bc.conn = nil
	bc.BlocksDeliverer = nil
}

type connection struct {
	sync.Once
	*grpc.ClientConn
	cancel context.CancelFunc
}

func (c *connection) Close() error {
	var err error
	c.Once.Do(func() {
		c.cancel()
		err = c.ClientConn.Close()
	})
	return err
}
