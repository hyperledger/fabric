/*
Copyright IBM Corp. 2018 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/roundrobin"
)

// protects gRPC balancer registration
var gRPCBalancerLock = sync.Mutex{}

func init() {
	factory.InitFactories(nil)
}

//go:generate counterfeiter -o mocks/signer_serializer.go --fake-name SignerSerializer . signerSerializer

type signerSerializer interface {
	identity.SignerSerializer
}

type wrappedBalancer struct {
	balancer.Balancer
	cd *countingDialer
}

func (wb *wrappedBalancer) Close() {
	defer atomic.AddUint32(&wb.cd.connectionCount, ^uint32(0))
	wb.Balancer.Close()
}

type countingDialer struct {
	name            string
	baseBuilder     balancer.Builder
	connectionCount uint32
}

func newCountingDialer() *countingDialer {
	gRPCBalancerLock.Lock()
	builder := balancer.Get(roundrobin.Name)
	gRPCBalancerLock.Unlock()

	buff := make([]byte, 16)
	rand.Read(buff)
	cb := &countingDialer{
		name:        hex.EncodeToString(buff),
		baseBuilder: builder,
	}

	gRPCBalancerLock.Lock()
	balancer.Register(cb)
	gRPCBalancerLock.Unlock()

	return cb
}

func (d *countingDialer) Build(cc balancer.ClientConn, opts balancer.BuildOptions) balancer.Balancer {
	defer atomic.AddUint32(&d.connectionCount, 1)
	lb := d.baseBuilder.Build(cc, opts)
	return &wrappedBalancer{
		Balancer: lb,
		cd:       d,
	}
}

func (d *countingDialer) Name() string {
	return d.name
}

func (d *countingDialer) assertAllConnectionsClosed(t *testing.T) {
	timeLimit := time.Now().Add(timeout)
	for atomic.LoadUint32(&d.connectionCount) != uint32(0) && time.Now().Before(timeLimit) {
		time.Sleep(time.Millisecond)
	}
	require.Equal(t, uint32(0), atomic.LoadUint32(&d.connectionCount))
}

func (d *countingDialer) Dial(address cluster.EndpointCriteria) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()

	gRPCBalancerLock.Lock()
	balancer := grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingConfig": [{"%s":{}}]}`, d.name))
	gRPCBalancerLock.Unlock()
	return grpc.DialContext(ctx, address.Endpoint, grpc.WithBlock(), grpc.WithInsecure(), balancer)
}

func noopBlockVerifierf(_ []*common.Block, _ string) error {
	return nil
}

func readSeekEnvelope(stream orderer.AtomicBroadcast_DeliverServer) (*orderer.SeekInfo, string, error) {
	env, err := stream.Recv()
	if err != nil {
		return nil, "", err
	}
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, "", err
	}
	seekInfo := &orderer.SeekInfo{}
	if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
		return nil, "", err
	}
	chdr := &common.ChannelHeader{}
	if err = proto.Unmarshal(payload.Header.ChannelHeader, chdr); err != nil {
		return nil, "", err
	}
	return seekInfo, chdr.ChannelId, nil
}

type deliverServer struct {
	logger *flogging.FabricLogger

	t *testing.T
	sync.Mutex
	err            error
	srv            *comm.GRPCServer
	seekAssertions chan func(*orderer.SeekInfo, string)
	blockResponses chan *orderer.DeliverResponse
	done           chan struct{}
}

func (ds *deliverServer) endpointCriteria() cluster.EndpointCriteria {
	return cluster.EndpointCriteria{Endpoint: ds.srv.Address()}
}

func (ds *deliverServer) isFaulty() bool {
	ds.Lock()
	defer ds.Unlock()
	return ds.err != nil
}

func (*deliverServer) Broadcast(orderer.AtomicBroadcast_BroadcastServer) error {
	panic("implement me")
}

func (ds *deliverServer) Deliver(stream orderer.AtomicBroadcast_DeliverServer) error {
	ds.Lock()
	err := ds.err
	ds.Unlock()

	if err != nil {
		return err
	}
	seekInfo, channel, err := readSeekEnvelope(stream)
	require.NoError(ds.t, err)

	// FAB-16233 This is meant to mitigate timeouts when
	// seekAssertions does not receive a value
	timer := time.NewTimer(1 * time.Minute)
	defer timer.Stop()

	select {
	case <-timer.C:
		ds.t.Fatalf("timed out waiting for seek assertions to receive a value\n")
	// Get the next seek assertion and ensure the next seek is of the expected type
	case seekAssert := <-ds.seekAssertions:
		ds.logger.Debugf("Received seekInfo: %+v", seekInfo)
		seekAssert(seekInfo, channel)
	case <-ds.done:
		return nil
	}

	if seekInfo.GetStart().GetSpecified() != nil {
		return ds.deliverBlocks(stream)
	}
	if seekInfo.GetStart().GetNewest() != nil {
		select {
		case resp := <-ds.blocks():
			if resp == nil {
				return nil
			}
			return stream.Send(resp)
		case <-ds.done:
		}
	}
	ds.t.Fatalf("expected either specified or newest seek but got %v\n", seekInfo.GetStart())
	return nil // unreachable
}

func (ds *deliverServer) deliverBlocks(stream orderer.AtomicBroadcast_DeliverServer) error {
	for {
		blockChan := ds.blocks()
		var response *orderer.DeliverResponse
		select {
		case response = <-blockChan:
		case <-ds.done:
			return nil
		}

		// A nil response is a signal from the test to close the stream.
		// This is needed to avoid reading from the block buffer, hence
		// consuming by accident a block that is tabled to be pulled
		// later in the test.
		if response == nil {
			return nil
		}
		if err := stream.Send(response); err != nil {
			return err
		}
	}
}

func (ds *deliverServer) blocks() chan *orderer.DeliverResponse {
	ds.Lock()
	defer ds.Unlock()
	blockChan := ds.blockResponses
	return blockChan
}

func (ds *deliverServer) setBlocks(blocks chan *orderer.DeliverResponse) {
	ds.Lock()
	defer ds.Unlock()
	ds.blockResponses = blocks
}

func (ds *deliverServer) port() int {
	_, portStr, err := net.SplitHostPort(ds.srv.Address())
	require.NoError(ds.t, err)

	port, err := strconv.ParseInt(portStr, 10, 32)
	require.NoError(ds.t, err)
	return int(port)
}

func (ds *deliverServer) resurrect() {
	var err error
	// copy the responses channel into a fresh one
	respChan := make(chan *orderer.DeliverResponse, 100)
	for resp := range ds.blocks() {
		respChan <- resp
	}
	ds.blockResponses = respChan
	ds.done = make(chan struct{})
	ds.srv.Stop()
	// And re-create the gRPC server on that port
	ds.srv, err = comm.NewGRPCServer(fmt.Sprintf("127.0.0.1:%d", ds.port()), comm.ServerConfig{})
	require.NoError(ds.t, err)
	orderer.RegisterAtomicBroadcastServer(ds.srv.Server(), ds)
	go ds.srv.Start()
}

func (ds *deliverServer) stop() {
	ds.srv.Stop()
	close(ds.blocks())
	close(ds.done)
}

func (ds *deliverServer) enqueueResponse(seq uint64) {
	select {
	case ds.blocks() <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{Block: protoutil.NewBlock(seq, nil)},
	}:
	case <-ds.done:
	}
}

func (ds *deliverServer) addExpectProbeAssert() {
	select {
	case ds.seekAssertions <- func(info *orderer.SeekInfo, _ string) {
		require.NotNil(ds.t, info.GetStart().GetNewest())
		require.Equal(ds.t, info.ErrorResponse, orderer.SeekInfo_BEST_EFFORT)
	}:
	case <-ds.done:
	}
}

func (ds *deliverServer) addExpectPullAssert(seq uint64) {
	select {
	case ds.seekAssertions <- func(info *orderer.SeekInfo, _ string) {
		seekPosition := info.GetStart()
		require.NotNil(ds.t, seekPosition)
		seekSpecified := seekPosition.GetSpecified()
		require.NotNil(ds.t, seekSpecified)
		require.Equal(ds.t, seq, seekSpecified.Number)
		require.Equal(ds.t, info.ErrorResponse, orderer.SeekInfo_BEST_EFFORT)
	}:
	case <-ds.done:
	}
}

func newClusterNode(t *testing.T) *deliverServer {
	srv, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{})
	require.NoError(t, err)
	ds := &deliverServer{
		logger:         flogging.MustGetLogger("test.debug"),
		t:              t,
		seekAssertions: make(chan func(*orderer.SeekInfo, string), 100),
		blockResponses: make(chan *orderer.DeliverResponse, 100),
		done:           make(chan struct{}),
		srv:            srv,
	}
	orderer.RegisterAtomicBroadcastServer(srv.Server(), ds)
	go srv.Start()
	return ds
}

func newBlockPuller(dialer *countingDialer, orderers ...string) *cluster.BlockPuller {
	return &cluster.BlockPuller{
		Dialer:              dialer,
		Channel:             "mychannel",
		Signer:              &mocks.SignerSerializer{},
		Endpoints:           endpointCriteriaFromEndpoints(orderers...),
		FetchTimeout:        time.Second * 10,
		MaxTotalBufferBytes: 1024 * 1024, // 1MB
		RetryTimeout:        time.Millisecond * 10,
		VerifyBlockSequence: noopBlockVerifierf,
		Logger:              flogging.MustGetLogger("test"),
		StopChannel:         make(chan struct{}),
	}
}

func endpointCriteriaFromEndpoints(orderers ...string) []cluster.EndpointCriteria {
	var res []cluster.EndpointCriteria
	for _, orderer := range orderers {
		res = append(res, cluster.EndpointCriteria{Endpoint: orderer})
	}
	return res
}

func TestBlockPullerBasicHappyPath(t *testing.T) {
	// Scenario: Single ordering node,
	// and the block puller pulls blocks 5 to 10.
	osn := newClusterNode(t)
	defer osn.stop()

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, osn.srv.Address())

	// The first seek request asks for the latest block
	osn.addExpectProbeAssert()
	// The first response says that the height is 10
	osn.enqueueResponse(10)
	// The next seek request is for block 5
	osn.addExpectPullAssert(5)
	// The later responses are the block themselves
	for i := 5; i <= 10; i++ {
		osn.enqueueResponse(uint64(i))
	}

	for i := 5; i <= 10; i++ {
		require.Equal(t, uint64(i), bp.PullBlock(uint64(i)).Header.Number)
	}
	require.Len(t, osn.blockResponses, 0)

	bp.Close()
	dialer.assertAllConnectionsClosed(t)
}

func TestBlockPullerDuplicate(t *testing.T) {
	// Scenario: The address of the ordering node
	// is found twice in the configuration, but this
	// doesn't create a problem.
	osn := newClusterNode(t)
	defer osn.stop()

	dialer := newCountingDialer()
	// Add the address twice
	bp := newBlockPuller(dialer, osn.srv.Address(), osn.srv.Address())

	// The first seek request asks for the latest block (twice)
	osn.addExpectProbeAssert()
	osn.addExpectProbeAssert()
	// The first response says that the height is 3
	osn.enqueueResponse(3)
	osn.enqueueResponse(3)
	// The next seek request is for block 1, only from some OSN
	osn.addExpectPullAssert(1)
	// The later responses are the block themselves
	for i := 1; i <= 3; i++ {
		osn.enqueueResponse(uint64(i))
	}

	for i := 1; i <= 3; i++ {
		require.Equal(t, uint64(i), bp.PullBlock(uint64(i)).Header.Number)
	}
	require.Len(t, osn.blockResponses, 0)

	bp.Close()
	dialer.assertAllConnectionsClosed(t)
}

func TestBlockPullerHeavyBlocks(t *testing.T) {
	// Scenario: Single ordering node,
	// and the block puller pulls 50 blocks, each
	// weighing 1K, but the buffer can only contain
	// 10K, so it should pull the 50 blocks in chunks of 10,
	// and verify 5 sequences at a time.

	osn := newClusterNode(t)
	defer osn.stop()
	osn.addExpectProbeAssert()
	osn.addExpectPullAssert(1)
	osn.enqueueResponse(100) // The last sequence is 100

	enqueueBlockBatch := func(start, end uint64) {
		for seq := start; seq <= end; seq++ {
			resp := &orderer.DeliverResponse{
				Type: &orderer.DeliverResponse_Block{
					Block: protoutil.NewBlock(seq, nil),
				},
			}
			data := resp.GetBlock().Data.Data
			resp.GetBlock().Data.Data = append(data, make([]byte, 1024))
			osn.blockResponses <- resp
		}
	}

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, osn.srv.Address())
	var gotBlockMessageCount int
	bp.Logger = bp.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if strings.Contains(entry.Message, "Got block") {
			gotBlockMessageCount++
		}
		return nil
	}))
	bp.MaxTotalBufferBytes = 1024 * 10 // 10K

	// Enqueue only the next batch in the orderer node.
	// This ensures that only 10 blocks are fetched into the buffer
	// and not more.
	for i := uint64(0); i < 5; i++ {
		enqueueBlockBatch(i*10+uint64(1), i*10+uint64(10))
		for seq := i*10 + uint64(1); seq <= i*10+uint64(10); seq++ {
			require.Equal(t, seq, bp.PullBlock(seq).Header.Number)
		}
	}

	require.Equal(t, 50, gotBlockMessageCount)
	bp.Close()
	dialer.assertAllConnectionsClosed(t)
}

func TestBlockPullerClone(t *testing.T) {
	// Scenario: We have a block puller that is connected
	// to an ordering node, and we clone it.
	// We expect that the new block puller is a clean slate
	// and doesn't share any internal state with its origin.
	osn1 := newClusterNode(t)
	defer osn1.stop()

	osn1.addExpectProbeAssert()
	osn1.addExpectPullAssert(1)
	// last block sequence is 100
	osn1.enqueueResponse(100)
	osn1.enqueueResponse(1)
	// The block puller is expected to disconnect after pulling
	// a single block. So signal the server-side to avoid
	// grabbing the next block after block 1 is pulled.
	osn1.blockResponses <- nil

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, osn1.srv.Address())
	bp.FetchTimeout = time.Millisecond * 100
	// Pull a block at a time and don't buffer them
	bp.MaxTotalBufferBytes = 1
	// Clone the block puller
	bpClone := bp.Clone()
	// and override its channel
	bpClone.Channel = "foo"
	// Ensure the channel change doesn't reflect in the original puller
	require.Equal(t, "mychannel", bp.Channel)

	block := bp.PullBlock(1)
	require.Equal(t, uint64(1), block.Header.Number)

	// After the original block puller is closed, the
	// clone should not be affected
	bp.Close()
	dialer.assertAllConnectionsClosed(t)

	// The clone block puller should not have cached the internal state
	// from its origin block puller, thus it should probe again for the last block
	// sequence as if it is a new block puller.
	osn1.addExpectProbeAssert()
	osn1.addExpectPullAssert(2)
	osn1.enqueueResponse(100)
	osn1.enqueueResponse(2)

	block = bpClone.PullBlock(2)
	require.Equal(t, uint64(2), block.Header.Number)

	bpClone.Close()
	dialer.assertAllConnectionsClosed(t)
}

func TestBlockPullerHeightsByEndpoints(t *testing.T) {
	// Scenario: We ask for the latest block from all the known ordering nodes.
	// One ordering node is unavailable (offline).
	// One ordering node doesn't have blocks for that channel.
	// The remaining node returns the latest block.
	osn1 := newClusterNode(t)

	osn2 := newClusterNode(t)
	defer osn2.stop()

	osn3 := newClusterNode(t)
	defer osn3.stop()

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, osn1.srv.Address(), osn2.srv.Address(), osn3.srv.Address())

	// The first seek request asks for the latest block from some ordering node
	osn1.addExpectProbeAssert()
	osn2.addExpectProbeAssert()
	osn3.addExpectProbeAssert()

	// The first ordering node is offline
	osn1.stop()
	// The second returns a bad answer
	osn2.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Status{Status: common.Status_FORBIDDEN},
	}
	// The third returns the latest block
	osn3.enqueueResponse(5)

	res, err := bp.HeightsByEndpoints()
	require.NoError(t, err)
	expected := map[string]uint64{
		osn3.srv.Address(): 6,
	}
	require.Equal(t, expected, res)

	bp.Close()
	dialer.assertAllConnectionsClosed(t)
}

func TestBlockPullerMultipleOrderers(t *testing.T) {
	// Scenario: 3 ordering nodes,
	// and the block puller pulls blocks 3 to 5 from some
	// orderer node.
	// We ensure that exactly a single orderer node sent blocks.

	osn1 := newClusterNode(t)
	defer osn1.stop()

	osn2 := newClusterNode(t)
	defer osn2.stop()

	osn3 := newClusterNode(t)
	defer osn3.stop()

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, osn1.srv.Address(), osn2.srv.Address(), osn3.srv.Address())

	// The first seek request asks for the latest block from some ordering node
	osn1.addExpectProbeAssert()
	osn2.addExpectProbeAssert()
	osn3.addExpectProbeAssert()
	// The first response says that the height is 5
	osn1.enqueueResponse(5)
	osn2.enqueueResponse(5)
	osn3.enqueueResponse(5)

	// The next seek request is for block 3
	osn1.addExpectPullAssert(3)
	osn2.addExpectPullAssert(3)
	osn3.addExpectPullAssert(3)

	// The later responses are the block themselves
	for i := 3; i <= 5; i++ {
		osn1.enqueueResponse(uint64(i))
		osn2.enqueueResponse(uint64(i))
		osn3.enqueueResponse(uint64(i))
	}

	initialTotalBlockAmount := len(osn1.blockResponses) + len(osn2.blockResponses) + len(osn3.blockResponses)

	for i := 3; i <= 5; i++ {
		require.Equal(t, uint64(i), bp.PullBlock(uint64(i)).Header.Number)
	}

	// Assert the cumulative amount of blocks in the OSNs went down by 6:
	// blocks 3, 4, 5 were pulled - that's 3 blocks.
	// block 5 was pulled 3 times at the probe phase.
	finalTotalBlockAmount := len(osn1.blockResponses) + len(osn2.blockResponses) + len(osn3.blockResponses)
	require.Equal(t, initialTotalBlockAmount-6, finalTotalBlockAmount)

	bp.Close()
	dialer.assertAllConnectionsClosed(t)
}

func TestBlockPullerFailover(t *testing.T) {
	// Scenario:
	// The block puller is expected to pull blocks 1 to 3.
	// There are two ordering nodes, but at first -  only node 1 is available.
	// the block puller first connects to it, but as it pulls the first block,
	// it crashes.
	// A second orderer is then spawned and the block puller is expected to
	// connect to it and pull the rest of the blocks.

	osn1 := newClusterNode(t)
	osn1.addExpectProbeAssert()
	osn1.addExpectPullAssert(1)
	osn1.enqueueResponse(3)
	osn1.enqueueResponse(1)

	osn2 := newClusterNode(t)
	defer osn2.stop()

	osn2.addExpectProbeAssert()
	osn2.addExpectPullAssert(1)
	// First response is for the probe
	osn2.enqueueResponse(3)
	// Next three responses are for the pulling.
	osn2.enqueueResponse(1)
	osn2.enqueueResponse(2)
	osn2.enqueueResponse(3)

	// First we stop node 2 to make sure the block puller can't connect to it when it is created
	osn2.stop()

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, osn1.srv.Address(), osn2.srv.Address())

	// We don't want to rely on fetch timeout, but on pure failover logic,
	// so make the fetch timeout huge
	bp.FetchTimeout = time.Hour

	// Configure the block puller logger to signal the wait group once the block puller
	// received the first block.
	pulledBlock1Done := make(chan struct{})
	var once sync.Once
	bp.Logger = bp.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if strings.Contains(entry.Message, "Got block [1] of size") {
			once.Do(func() { close(pulledBlock1Done) })
		}
		return nil
	}))

	go func() {
		// Wait for the block puller to pull the first block
		select {
		case <-pulledBlock1Done:
		case <-time.After(10 * time.Second):
			t.Errorf("message was not logged before timeout")
			return
		}
		// and now crash node1 and resurrect node 2
		osn1.stop()
		osn2.resurrect()
	}()

	// Assert reception of blocks 1 to 3
	require.Equal(t, uint64(1), bp.PullBlock(uint64(1)).Header.Number)
	require.Equal(t, uint64(2), bp.PullBlock(uint64(2)).Header.Number)
	require.Equal(t, uint64(3), bp.PullBlock(uint64(3)).Header.Number)

	bp.Close()
	dialer.assertAllConnectionsClosed(t)
}

func TestBlockPullerNoneResponsiveOrderer(t *testing.T) {
	// Scenario: There are two ordering nodes, and the block puller
	// connects to one of them.
	// it fetches the first block, but then the second block isn't sent
	// for too long, and the block puller should abort, and try the other
	// node.

	osn1 := newClusterNode(t)
	defer osn1.stop()

	osn2 := newClusterNode(t)
	defer osn2.stop()

	osn1.addExpectProbeAssert()
	osn2.addExpectProbeAssert()
	osn1.enqueueResponse(3)
	osn2.enqueueResponse(3)

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, osn1.srv.Address(), osn2.srv.Address())
	bp.FetchTimeout = time.Millisecond * 100

	notInUseOrdererNode := osn2
	// Configure the logger to tell us who is the orderer node that the block puller
	// isn't connected to. This is done by intercepting the appropriate message
	waitForConnectionDone := make(chan struct{})
	var once sync.Once
	bp.Logger = bp.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if !strings.Contains(entry.Message, "Sending request for block [1]") {
			return nil
		}
		defer once.Do(func() { close(waitForConnectionDone) })
		s := entry.Message[len("Sending request for block [1] to 127.0.0.1:"):]
		port, err := strconv.ParseInt(s, 10, 32)
		require.NoError(t, err)
		// If osn2 is the current orderer we're connected to,
		// the orderer we're not connected to, is osn1
		if osn2.port() == int(port) {
			notInUseOrdererNode = osn1
			// Enqueue block 1 to the current orderer the block puller
			// is connected to
			osn2.enqueueResponse(1)
			osn2.addExpectPullAssert(1)
		} else {
			// We're connected to osn1, so enqueue block 1
			osn1.enqueueResponse(1)
			osn1.addExpectPullAssert(1)
		}
		return nil
	}))

	go func() {
		select {
		case <-waitForConnectionDone:
		case <-time.After(10 * time.Second):
			t.Errorf("message was not logged before timeout")
			return
		}

		// Enqueue the height int the orderer we're connected to
		notInUseOrdererNode.enqueueResponse(3)
		notInUseOrdererNode.addExpectProbeAssert()
		// Enqueue blocks 1, 2, 3 to the orderer node we're not connected to.
		notInUseOrdererNode.addExpectPullAssert(1)
		notInUseOrdererNode.enqueueResponse(1)
		notInUseOrdererNode.enqueueResponse(2)
		notInUseOrdererNode.enqueueResponse(3)
	}()

	// Assert reception of blocks 1 to 3
	require.Equal(t, uint64(1), bp.PullBlock(uint64(1)).Header.Number)
	require.Equal(t, uint64(2), bp.PullBlock(uint64(2)).Header.Number)
	require.Equal(t, uint64(3), bp.PullBlock(uint64(3)).Header.Number)

	bp.Close()
	dialer.assertAllConnectionsClosed(t)
}

func TestBlockPullerNoOrdererAliveAtStartup(t *testing.T) {
	// Scenario: Single ordering node, and when the block puller
	// starts up - the orderer is nowhere to be found.
	osn := newClusterNode(t)
	osn.stop()
	defer osn.stop()

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, osn.srv.Address())

	// Configure the logger to wait until the a failed connection attempt was made
	connectionAttemptDone := make(chan struct{})
	var closeOnce sync.Once
	bp.Logger = bp.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if strings.Contains(entry.Message, "Failed connecting to") {
			closeOnce.Do(func() { close(connectionAttemptDone) })
		}
		return nil
	}))

	go func() {
		select {
		case <-connectionAttemptDone:
		case <-time.After(10 * time.Second):
			t.Errorf("message was not logged before timeout")
			return
		}

		osn.resurrect()
		// The first seek request asks for the latest block
		osn.addExpectProbeAssert()
		// The first response says that the last sequence is 2
		osn.enqueueResponse(2)
		// The next seek request is for block 1
		osn.addExpectPullAssert(1)
		// And the orderer returns block 1 and 2
		osn.enqueueResponse(1)
		osn.enqueueResponse(2)
	}()

	require.Equal(t, uint64(1), bp.PullBlock(1).Header.Number)
	require.Equal(t, uint64(2), bp.PullBlock(2).Header.Number)

	bp.Close()
	dialer.assertAllConnectionsClosed(t)
}

func TestBlockPullerFailures(t *testing.T) {
	// This test case is flaky, so let's add some logs for the next time it fails.
	flogging.ActivateSpec("debug")
	defer flogging.ActivateSpec("info")
	testLogger := flogging.MustGetLogger("test.debug")

	// Scenario: Single ordering node is faulty, but later
	// on it recovers.
	failureError := errors.New("oops, something went wrong")
	failStream := func(osn *deliverServer, _ *cluster.BlockPuller) {
		testLogger.Info("failStream")
		osn.Lock()
		osn.err = failureError
		osn.Unlock()
	}

	badSigErr := errors.New("bad signature")
	malformBlockSignatureAndRecreateOSNBuffer := func(osn *deliverServer, bp *cluster.BlockPuller) {
		testLogger.Info("FailFunc of: failure at verifying pulled block")
		bp.VerifyBlockSequence = func(_ []*common.Block, _ string) error {
			close(osn.blocks())
			// After failing once, recover and remove the bad signature error.
			defer func() {
				// Skip recovery if we already recovered.
				if badSigErr == nil {
					return
				}
				badSigErr = nil
				osn.setBlocks(make(chan *orderer.DeliverResponse, 100))
				osn.enqueueResponse(3)
				osn.enqueueResponse(1)
				osn.enqueueResponse(2)
				osn.enqueueResponse(3)
			}()
			return badSigErr
		}
	}

	noFailFunc := func(_ *deliverServer, _ *cluster.BlockPuller) {}

	recover := func(osn *deliverServer, bp *cluster.BlockPuller) func(entry zapcore.Entry) error {
		return func(entry zapcore.Entry) error {
			if osn.isFaulty() && strings.Contains(entry.Message, failureError.Error()) {
				testLogger.Info("recover osn.err")
				osn.Lock()
				osn.err = nil
				osn.Unlock()
			}
			if strings.Contains(entry.Message, "Failed verifying") {
				testLogger.Info("recover verifier")
				bp.VerifyBlockSequence = noopBlockVerifierf
			}
			return nil
		}
	}

	failAfterConnection := func(osn *deliverServer, logTrigger string, failFunc func()) func(entry zapcore.Entry) error {
		once := &sync.Once{}
		return func(entry zapcore.Entry) error {
			if !osn.isFaulty() && strings.Contains(entry.Message, logTrigger) {
				once.Do(func() {
					failFunc()
				})
			}
			return nil
		}
	}

	for _, testCase := range []struct {
		name       string
		logTrigger string
		beforeFunc func(*deliverServer, *cluster.BlockPuller)
		failFunc   func(*deliverServer, *cluster.BlockPuller)
	}{
		{
			name:       "failure at probe",
			logTrigger: "skip this for this test case",
			beforeFunc: func(osn *deliverServer, bp *cluster.BlockPuller) {
				failStream(osn, nil)
				// The first seek request asks for the latest block but it fails
				osn.addExpectProbeAssert()
				// And the last block sequence is returned
				osn.enqueueResponse(3)
				// The next seek request is for block 1
				osn.addExpectPullAssert(1)
			},
			failFunc: noFailFunc,
		},
		{
			name:       "failure at pull",
			logTrigger: "Sending request for block [1]",
			beforeFunc: func(osn *deliverServer, bp *cluster.BlockPuller) {
				// The first seek request asks for the latest block and succeeds
				osn.addExpectProbeAssert()
				// But as the block puller tries to pull, the stream fails, so it should reconnect.
				osn.addExpectProbeAssert()
				// We need to send the latest sequence twice because the stream fails after the first time.
				osn.enqueueResponse(3)
				osn.enqueueResponse(3)
				osn.addExpectPullAssert(1)
			},
			failFunc: failStream,
		},
		{
			name:       "failure at verifying pulled block",
			logTrigger: "Sending request for block [1]",
			beforeFunc: func(osn *deliverServer, bp *cluster.BlockPuller) {
				testLogger.Infof("BeforeFunc of: %s", "failure at verifying pulled block")
				// The first seek request asks for the latest block and succeeds
				osn.addExpectProbeAssert()
				osn.enqueueResponse(3)
				// It then pulls all 3 blocks, but fails verifying them.
				osn.addExpectPullAssert(1)
				osn.enqueueResponse(1)
				osn.enqueueResponse(2)
				osn.enqueueResponse(3)
				// So it probes again, and then requests block 1 again.
				osn.addExpectProbeAssert()
				osn.addExpectPullAssert(1)
			},
			failFunc: malformBlockSignatureAndRecreateOSNBuffer,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			testLogger.Infof("Starting test case: %s", testCase.name)
			osn := newClusterNode(t)
			defer osn.stop()

			dialer := newCountingDialer()
			bp := newBlockPuller(dialer, osn.srv.Address())

			testCase.beforeFunc(osn, bp)

			// Configure the logger to trigger failure at the appropriate time.
			fail := func() {
				testCase.failFunc(osn, bp)
			}
			bp.Logger = bp.Logger.WithOptions(zap.Hooks(recover(osn, bp), failAfterConnection(osn, testCase.logTrigger, fail)))

			// The orderer sends blocks 1 to 3
			osn.enqueueResponse(1)
			osn.enqueueResponse(2)
			osn.enqueueResponse(3)

			require.Equal(t, uint64(1), bp.PullBlock(uint64(1)).Header.Number)
			require.Equal(t, uint64(2), bp.PullBlock(uint64(2)).Header.Number)
			require.Equal(t, uint64(3), bp.PullBlock(uint64(3)).Header.Number)

			bp.Close()
			dialer.assertAllConnectionsClosed(t)
		})
	}
}

func TestBlockPullerBadBlocks(t *testing.T) {
	// Scenario: ordering node sends malformed blocks.

	// This test case is flaky, so let's add some logs for the next time it fails.
	flogging.ActivateSpec("debug")
	defer flogging.ActivateSpec("info")
	testLogger := flogging.MustGetLogger("test.debug")

	removeHeader := func(resp *orderer.DeliverResponse) *orderer.DeliverResponse {
		resp.GetBlock().Header = nil
		return resp
	}

	removeData := func(resp *orderer.DeliverResponse) *orderer.DeliverResponse {
		resp.GetBlock().Data = nil
		return resp
	}

	removeMetadata := func(resp *orderer.DeliverResponse) *orderer.DeliverResponse {
		resp.GetBlock().Metadata = nil
		return resp
	}

	changeType := func(resp *orderer.DeliverResponse) *orderer.DeliverResponse {
		resp.Type = nil
		return resp
	}

	statusType := func(resp *orderer.DeliverResponse) *orderer.DeliverResponse {
		resp.Type = &orderer.DeliverResponse_Status{
			Status: common.Status_INTERNAL_SERVER_ERROR,
		}
		return resp
	}

	changeSequence := func(resp *orderer.DeliverResponse) *orderer.DeliverResponse {
		resp.GetBlock().Header.Number = 3
		return resp
	}

	testcases := []struct {
		name           string
		corruptBlock   func(block *orderer.DeliverResponse) *orderer.DeliverResponse
		expectedErrMsg string
	}{
		{
			name:           "nil header",
			corruptBlock:   removeHeader,
			expectedErrMsg: "block header is nil",
		},
		{
			name:           "nil data",
			corruptBlock:   removeData,
			expectedErrMsg: "block data is nil",
		},
		{
			name:           "nil metadata",
			corruptBlock:   removeMetadata,
			expectedErrMsg: "block metadata is empty",
		},
		{
			name:           "wrong type",
			corruptBlock:   changeType,
			expectedErrMsg: "response is of type <nil>, but expected a block",
		},
		{
			name:           "bad type",
			corruptBlock:   statusType,
			expectedErrMsg: "faulty node, received: status:INTERNAL_SERVER_ERROR ",
		},
		{
			name:           "wrong number",
			corruptBlock:   changeSequence,
			expectedErrMsg: "got unexpected sequence",
		},
	}

	for _, testCase := range testcases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			osn := newClusterNode(t)
			defer osn.stop()

			osn.addExpectProbeAssert()
			osn.addExpectPullAssert(10)

			dialer := newCountingDialer()
			bp := newBlockPuller(dialer, osn.srv.Address())

			osn.enqueueResponse(10)
			osn.enqueueResponse(10)
			// Fetch the first
			block := <-osn.blockResponses
			// Enqueue the probe response
			// re-insert the pull response after corrupting it, to the tail of the queue
			osn.blockResponses <- testCase.corruptBlock(block)
			// [ 10  10* ]
			// Expect the block puller to retry and this time give it what it wants
			osn.addExpectProbeAssert()
			osn.addExpectPullAssert(10)
			require.Len(t, osn.seekAssertions, 4)

			// Wait until the block is pulled and the malleability is detected
			detectedBadBlockDone := make(chan struct{})
			var closeOnce sync.Once
			bp.Logger = bp.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
				if strings.Contains(entry.Message, fmt.Sprintf("Failed pulling blocks: %s", testCase.expectedErrMsg)) {
					closeOnce.Do(func() { close(detectedBadBlockDone) })

					// Close the channel to make the current server-side deliver stream close
					close(osn.blocks())
					testLogger.Infof("Expected err log has been printed: %s\n", testCase.expectedErrMsg)
					// And reset the block buffer to be able to write into it again
					osn.setBlocks(make(chan *orderer.DeliverResponse, 100))
					// Put a correct block after it, 1 for the probing and 1 for the fetch
					osn.enqueueResponse(10)
					osn.enqueueResponse(10)
				}
				return nil
			}))

			endPullBlock := make(chan struct{})
			go func() {
				bp.PullBlock(10)
				close(endPullBlock)
			}()

			select {
			case <-detectedBadBlockDone:
			case <-time.After(10 * time.Second):
				t.Fatalf("expected %q to be logged but it was not seen", testCase.expectedErrMsg)
			}

			select {
			case <-endPullBlock:
			case <-time.After(10 * time.Second):
				// Signal PullBlock to give up on the retries & mark the test as fail
				close(bp.StopChannel)
				t.Fatalf("PullBlock did not complete within time")
			}

			bp.Close()
			dialer.assertAllConnectionsClosed(t)
		})
	}
}

func TestImpatientStreamFailure(t *testing.T) {
	osn := newClusterNode(t)
	dialer := newCountingDialer()
	defer dialer.assertAllConnectionsClosed(t)
	// Wait for OSN to start
	// by trying to connect to it
	var conn *grpc.ClientConn
	var err error

	gt := gomega.NewGomegaWithT(t)
	gt.Eventually(func() (bool, error) {
		conn, err = dialer.Dial(osn.endpointCriteria())
		return true, err
	}).Should(gomega.BeTrue())
	newStream := cluster.NewImpatientStream(conn, time.Millisecond*100)
	defer conn.Close()
	// Shutdown the OSN
	osn.stop()
	// Ensure the port isn't open anymore
	gt.Eventually(func() (bool, error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*10)
		defer cancel()
		conn, _ := grpc.DialContext(ctx, osn.srv.Address(), grpc.WithBlock(), grpc.WithInsecure())
		if conn != nil {
			conn.Close()
			return false, nil
		}
		return true, nil
	}).Should(gomega.BeTrue())
	stream, err := newStream()
	if err != nil {
		return
	}
	_, err = stream.Recv()
	require.Error(t, err)
}

func TestBlockPullerMaxRetriesExhausted(t *testing.T) {
	// Scenario:
	// The block puller is expected to pull blocks 1 to 3.
	// But the orderer only has blocks 1,2, and from some reason
	// it sends back block 2 twice (we do this so that we
	// don't rely on timeout, because timeouts are flaky in tests).
	// It should attempt to re-connect and to send requests
	// until the attempt number is exhausted, after which
	// it gives up, and nil is returned.

	osn := newClusterNode(t)
	defer osn.stop()

	// We report having up to block 3.
	osn.enqueueResponse(3)
	osn.addExpectProbeAssert()
	// We send blocks 1
	osn.addExpectPullAssert(1)
	osn.enqueueResponse(1)
	// And 2, twice.
	osn.enqueueResponse(2)
	osn.enqueueResponse(2)
	// A nil message signals the deliver stream closes.
	// This is to signal the server side to prepare for a new deliver
	// stream that the client should open.
	osn.blockResponses <- nil

	for i := 0; i < 2; i++ {
		// Therefore, the block puller should disconnect and reconnect.
		osn.addExpectProbeAssert()
		// We report having up to block 3.
		osn.enqueueResponse(3)
		// And we expect to be asked for block 3, since blocks 1, 2
		// have already been passed to the caller.
		osn.addExpectPullAssert(3)
		// Once again, we send 2 instead of 3
		osn.enqueueResponse(2)
		// The client disconnects again
		osn.blockResponses <- nil
	}

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, osn.srv.Address())

	var exhaustedRetryAttemptsLogged bool

	bp.Logger = bp.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if entry.Message == "Failed pulling block [3]: retry count exhausted(2)" {
			exhaustedRetryAttemptsLogged = true
		}
		return nil
	}))

	bp.MaxPullBlockRetries = 2
	// We don't expect to timeout in this test, so make the timeout large
	// to prevent flakes due to CPU starvation.
	bp.FetchTimeout = time.Hour
	// Make the buffer tiny, only a single byte - in order deliver blocks
	// to the caller one by one and not store them in the buffer.
	bp.MaxTotalBufferBytes = 1

	// Assert reception of blocks 1 to 3
	require.Equal(t, uint64(1), bp.PullBlock(uint64(1)).Header.Number)
	require.Equal(t, uint64(2), bp.PullBlock(uint64(2)).Header.Number)
	require.Nil(t, bp.PullBlock(uint64(3)))

	bp.Close()
	dialer.assertAllConnectionsClosed(t)
	require.True(t, exhaustedRetryAttemptsLogged)
}

func TestBlockPullerToBadEndpointWithStop(t *testing.T) {
	// Scenario:
	// The block puller is initialized with endpoints that do not exist.
	// It should attempt to re-connect but will fail, causing the PullBlock()
	// to block the calling go-routine. Closing the StopChannel should signal
	// the blocked go-routine to gives up, and nil is returned.

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, "10.10.10.10:666") // Nobody is there

	var couldNotConnectLogged bool
	var receivedStopLogged bool

	bp.Logger = bp.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if entry.Message == "Could not connect to any endpoint of [{\"CAs\":null,\"Endpoint\":\"10.10.10.10:666\"}]" {
			couldNotConnectLogged = true
		}
		if entry.Message == "Received a stop signal" {
			receivedStopLogged = true
		}
		return nil
	}))

	// Retry for a long time
	bp.MaxPullBlockRetries = 100
	bp.RetryTimeout = time.Hour

	releaseDone := make(chan struct{})
	var closeOnce sync.Once

	go func() {
		// But this will get stuck until the StopChannel is closed
		require.Nil(t, bp.PullBlock(uint64(1)))
		bp.Close()
		closeOnce.Do(func() { close(releaseDone) })
	}()

	close(bp.StopChannel)
	select {
	case <-releaseDone:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for PullBlock to complete")
	}

	dialer.assertAllConnectionsClosed(t)
	require.True(t, couldNotConnectLogged)
	require.True(t, receivedStopLogged)
}

func TestBlockPullerToBadEndpoint(t *testing.T) {
	// Scenario: The block puller does not get stuck in an endless loop if it cannot connect to any endpoint.
	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, "10.10.10.10:666") // Nobody is there

	var exhaustedRetryAttemptsLogged bool
	var couldNotConnectLogged bool

	bp.Logger = bp.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if entry.Message == "Failed pulling block [1]: retry count exhausted(2)" {
			exhaustedRetryAttemptsLogged = true
		}
		if entry.Message == "Failed to connect to some endpoint, attempts exhausted(2), seq: 1, endpoints: [{\"CAs\":null,\"Endpoint\":\"10.10.10.10:666\"}]" {
			couldNotConnectLogged = true
		}
		return nil
	}))

	bp.MaxPullBlockRetries = 2
	// We don't expect to timeout in this test, so make the timeout large
	// to prevent flakes due to CPU starvation.
	bp.FetchTimeout = time.Hour

	require.Nil(t, bp.PullBlock(uint64(1)))

	bp.Close()
	dialer.assertAllConnectionsClosed(t)
	require.True(t, exhaustedRetryAttemptsLogged)
	require.True(t, couldNotConnectLogged)
}

func TestBlockPuller_UpdateEndpoint(t *testing.T) {
	t.Run("HeightsByEndpoints are changing", func(t *testing.T) {
		// Scenario: We ask for the latest block from a set of ordering nodes.
		// We then add an endpoint and remove an endpoint, update the puller's endpoints, and ask again.
		osn1 := newClusterNode(t)
		defer osn1.stop()
		osn2 := newClusterNode(t)
		defer osn2.stop()
		osn3 := newClusterNode(t)
		defer osn3.stop()

		dialer := newCountingDialer()
		bp := newBlockPuller(dialer, osn1.srv.Address(), osn2.srv.Address(), osn3.srv.Address())

		// The first seek request asks for the latest block from some ordering node
		osn1.addExpectProbeAssert()
		osn2.addExpectProbeAssert()
		osn3.addExpectProbeAssert()

		osn1.enqueueResponse(3)
		osn2.enqueueResponse(4)
		osn3.enqueueResponse(5)

		res, err := bp.HeightsByEndpoints()
		require.NoError(t, err)
		expected := map[string]uint64{
			osn1.srv.Address(): 4,
			osn2.srv.Address(): 5,
			osn3.srv.Address(): 6,
		}
		require.Equal(t, expected, res)

		osn4 := newClusterNode(t)
		defer osn4.stop()

		bp.UpdateEndpoints(endpointCriteriaFromEndpoints(osn2.srv.Address(), osn3.srv.Address(), osn4.srv.Address()))

		osn2.addExpectProbeAssert()
		osn3.addExpectProbeAssert()
		osn4.addExpectProbeAssert()

		osn2.enqueueResponse(44)
		osn3.enqueueResponse(55)
		osn4.enqueueResponse(66)

		res, err = bp.HeightsByEndpoints()
		require.NoError(t, err)
		expected = map[string]uint64{
			osn2.srv.Address(): 45,
			osn3.srv.Address(): 56,
			osn4.srv.Address(): 67,
		}
		require.Equal(t, expected, res)

		bp.Close()
		dialer.assertAllConnectionsClosed(t)
	})

	t.Run("switch pulling between endpoints", func(t *testing.T) {
		// Scenario:
		// The block puller is expected to pull blocks 1 to 2 from osn1.
		// After block 2 the endpoints are updated to osn2.
		// The block puller is expected to pull blocks 3 to 4 from osn2.

		osn1 := newClusterNode(t)
		defer osn1.stop()

		osn1.enqueueResponse(2)
		osn1.addExpectProbeAssert()
		// We send blocks 1 and 2
		osn1.addExpectPullAssert(1)
		osn1.enqueueResponse(1)
		osn1.enqueueResponse(2)

		dialer := newCountingDialer()
		bp := newBlockPuller(dialer, osn1.srv.Address())

		require.Equal(t, uint64(1), bp.PullBlock(uint64(1)).Header.Number)
		require.Equal(t, uint64(2), bp.PullBlock(uint64(2)).Header.Number)

		osn2 := newClusterNode(t)
		defer osn2.stop()

		osn2.enqueueResponse(4)
		osn2.addExpectProbeAssert()
		// We send blocks 1 and 2
		osn2.addExpectPullAssert(3)
		osn2.enqueueResponse(3)
		osn2.enqueueResponse(4)

		// This will disconnect
		bp.UpdateEndpoints(endpointCriteriaFromEndpoints(osn2.srv.Address()))

		require.Equal(t, uint64(3), bp.PullBlock(uint64(3)).Header.Number)
		require.Equal(t, uint64(4), bp.PullBlock(uint64(4)).Header.Number)

		bp.Close()
		dialer.assertAllConnectionsClosed(t)
	})
	t.Run("update to a bad endpoint", func(t *testing.T) {
		// Scenario:
		// The block puller is expected to pull blocks 1 to 3. The target orderer delivers only blocks 1,2.
		// The puller pulls blocks 1 & 2 into the buffer. After block 1 the endpoints are updated to a bad endpoint.
		// The puller gets block 2 from the buffer. It should attempt to re-connect until the attempt number is exhausted,
		// after which it gives up, and nil is returned.

		osn := newClusterNode(t)

		osn.enqueueResponse(2)
		osn.addExpectProbeAssert()
		// We send blocks 1 and 2
		osn.addExpectPullAssert(1)
		osn.enqueueResponse(1)
		osn.enqueueResponse(2)

		dialer := newCountingDialer()
		bp := newBlockPuller(dialer, osn.srv.Address())

		var exhaustedRetryAttemptsLogged bool
		var couldNotConnectLogged bool
		bp.Logger = bp.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
			if entry.Message == "Failed pulling block [3]: retry count exhausted(2)" {
				exhaustedRetryAttemptsLogged = true
			}
			if entry.Message ==
				"Failed to connect to some endpoint, attempts exhausted(2), seq: 3, endpoints: [{\"CAs\":null,\"Endpoint\":\"10.10.10.10:666\"}]" {
				couldNotConnectLogged = true
			}
			return nil
		}))

		bp.MaxPullBlockRetries = 2
		// We don't expect to timeout in this test, so make the timeout large
		// to prevent flakes due to CPU starvation.
		bp.FetchTimeout = time.Second

		require.Equal(t, uint64(1), bp.PullBlock(uint64(1)).Header.Number)
		osn.stop()
		// This will disconnect
		bp.UpdateEndpoints(endpointCriteriaFromEndpoints("10.10.10.10:666"))
		// Block 2 from the buffer
		require.Equal(t, uint64(2), bp.PullBlock(uint64(2)).Header.Number)
		// Block 3 will never arrive
		require.Nil(t, bp.PullBlock(uint64(3)))

		bp.Close()
		dialer.assertAllConnectionsClosed(t)
		require.True(t, exhaustedRetryAttemptsLogged)
		require.True(t, couldNotConnectLogged)
	})
}
