/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package performance

import (
	"context"
	"io"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

const pkgLogID = "orderer/common/performance"

var logger = flogging.MustGetLogger(pkgLogID)

// BenchmarkServer is a pseudo-server that grpc services could be registered to
type BenchmarkServer struct {
	server ab.AtomicBroadcastServer
	start  chan struct{}
	halt   chan struct{}
}

var (
	servers []*BenchmarkServer
	index   int
	mutex   sync.Mutex
)

// InitializeServerPool instantiates a Benchmark server pool of size 'number'
func InitializeServerPool(number int) {
	mutex = sync.Mutex{}
	index = 0
	servers = make([]*BenchmarkServer, number)
	for i := 0; i < number; i++ {
		servers[i] = &BenchmarkServer{
			server: nil,
			start:  make(chan struct{}),
			halt:   make(chan struct{}),
		}
	}
}

// GetBenchmarkServer retrieves next unused server in the pool.
// This method should ONLY be called by orderer main() and it
// should be used after initialization
func GetBenchmarkServer() *BenchmarkServer {
	mutex.Lock()
	defer mutex.Unlock()

	if index >= len(servers) {
		panic("Not enough servers in the pool!")
	}

	defer func() { index++ }()
	return servers[index]
}

// GetBenchmarkServerPool returns the whole server pool for client to use
// This should be used after initialization
func GetBenchmarkServerPool() []*BenchmarkServer {
	return servers
}

// Start blocks until server being halted. It is to prevent main process to exit
func (server *BenchmarkServer) Start() {
	server.halt = make(chan struct{})
	close(server.start) // signal waiters that service is registered

	// Block reading here to prevent process exit
	<-server.halt
}

// Halt server
func Halt(server *BenchmarkServer) { server.Halt() }

// Halt server
func (server *BenchmarkServer) Halt() {
	logger.Debug("Stopping benchmark server")
	server.server = nil
	server.start = make(chan struct{})
	close(server.halt)
}

// WaitForService blocks waiting for service to be registered
func WaitForService(server *BenchmarkServer) { server.WaitForService() }

// WaitForService blocks waiting for service to be registered
func (server *BenchmarkServer) WaitForService() { <-server.start }

// RegisterService registers a grpc service to server
func (server *BenchmarkServer) RegisterService(s ab.AtomicBroadcastServer) {
	server.server = s
}

// CreateBroadcastClient creates a broadcast client of this server
func (server *BenchmarkServer) CreateBroadcastClient() *BroadcastClient {
	client := &BroadcastClient{
		requestChan:  make(chan *cb.Envelope),
		responseChan: make(chan *ab.BroadcastResponse),
		errChan:      make(chan error),
	}
	go func() {
		client.errChan <- server.server.Broadcast(client)
	}()
	return client
}

// BroadcastClient represents a broadcast client that is used to interact
// with `broadcast` API
type BroadcastClient struct {
	grpc.ServerStream
	requestChan  chan *cb.Envelope
	responseChan chan *ab.BroadcastResponse
	errChan      chan error
}

func (BroadcastClient) Context() context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{})
}

// SendRequest sends an envelope to `broadcast` API synchronously
func (bc *BroadcastClient) SendRequest(request *cb.Envelope) {
	// TODO make this async
	bc.requestChan <- request
}

// GetResponse waits for a response of `broadcast` API synchronously
func (bc *BroadcastClient) GetResponse() *ab.BroadcastResponse {
	return <-bc.responseChan
}

// Close closes a broadcast client
func (bc *BroadcastClient) Close() {
	close(bc.requestChan)
}

// Errors returns the channel which return value of broadcast handler is sent to
func (bc *BroadcastClient) Errors() <-chan error {
	return bc.errChan
}

// Send implements AtomicBroadcast_BroadcastServer interface
func (bc *BroadcastClient) Send(br *ab.BroadcastResponse) error {
	bc.responseChan <- br
	return nil
}

// Recv implements AtomicBroadcast_BroadcastServer interface
func (bc *BroadcastClient) Recv() (*cb.Envelope, error) {
	msg, ok := <-bc.requestChan
	if !ok {
		return msg, io.EOF
	}
	return msg, nil
}

// CreateDeliverClient creates a broadcast client of this server
func (server *BenchmarkServer) CreateDeliverClient() *DeliverClient {
	client := &DeliverClient{
		requestChan:  make(chan *cb.Envelope),
		ResponseChan: make(chan *ab.DeliverResponse),
		ResultChan:   make(chan error),
	}
	go func() {
		client.ResultChan <- server.server.Deliver(client)
	}()
	return client
}

// DeliverClient represents a deliver client that is used to interact
// with `deliver` API
type DeliverClient struct {
	grpc.ServerStream
	requestChan  chan *cb.Envelope
	ResponseChan chan *ab.DeliverResponse
	ResultChan   chan error
}

func (DeliverClient) Context() context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{})
}

// SendRequest sends an envelope to `deliver` API synchronously
func (bc *DeliverClient) SendRequest(request *cb.Envelope) {
	// TODO make this async
	bc.requestChan <- request
}

// GetResponse waits for a response of `deliver` API synchronously
func (bc *DeliverClient) GetResponse() *ab.DeliverResponse {
	return <-bc.ResponseChan
}

// Close closes a deliver client
func (bc *DeliverClient) Close() {
	close(bc.requestChan)
}

// Send implements AtomicBroadcast_BroadcastServer interface
func (bc *DeliverClient) Send(br *ab.DeliverResponse) error {
	bc.ResponseChan <- br
	return nil
}

// Recv implements AtomicBroadcast_BroadcastServer interface
func (bc *DeliverClient) Recv() (*cb.Envelope, error) {
	msg, ok := <-bc.requestChan
	if !ok {
		return msg, io.EOF
	}
	return msg, nil
}
