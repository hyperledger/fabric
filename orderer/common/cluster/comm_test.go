/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	comm_utils "github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
)

const (
	testChannel  = "test"
	testChannel2 = "test2"
	timeout      = time.Second * 10
)

var (
	// CA that generates TLS key-pairs.
	// We use only one CA because the authentication
	// is based on TLS pinning
	ca = createCAOrPanic()

	lastNodeID uint64

	testSubReq = &orderer.SubmitRequest{
		Channel: "test",
	}

	testReq = &orderer.SubmitRequest{
		Channel: "test",
		Payload: &common.Envelope{
			Payload: []byte("test"),
		},
	}

	testReq2 = &orderer.SubmitRequest{
		Channel: testChannel2,
		Payload: &common.Envelope{
			Payload: []byte(testChannel2),
		},
	}

	testRes = &orderer.SubmitResponse{
		Info: "test",
	}

	fooReq = wrapSubmitReq(&orderer.SubmitRequest{
		Channel: "foo",
	})

	barReq = wrapSubmitReq(&orderer.SubmitRequest{
		Channel: "bar",
	})

	testConsensusReq = &orderer.StepRequest{
		Payload: &orderer.StepRequest_ConsensusRequest{
			ConsensusRequest: &orderer.ConsensusRequest{
				Payload: []byte{1, 2, 3},
				Channel: testChannel,
			},
		},
	}

	channelExtractor = &mockChannelExtractor{}
)

func nextUnusedID() uint64 {
	return atomic.AddUint64(&lastNodeID, 1)
}

func createCAOrPanic() tlsgen.CA {
	ca, err := tlsgen.NewCA()
	if err != nil {
		panic(fmt.Sprintf("failed creating CA: %+v", err))
	}
	return ca
}

type mockChannelExtractor struct{}

func (*mockChannelExtractor) TargetChannel(msg proto.Message) string {
	switch req := msg.(type) {
	case *orderer.ConsensusRequest:
		return req.Channel
	case *orderer.SubmitRequest:
		return req.Channel
	default:
		return ""
	}
}

type clusterServer interface {
	// Step passes an implementation-specific message to another cluster member.
	Step(server orderer.Cluster_StepServer) error
}

type clusterNode struct {
	lock         sync.Mutex
	frozen       bool
	freezeCond   sync.Cond
	dialer       *cluster.PredicateDialer
	handler      *mocks.Handler
	nodeInfo     cluster.RemoteNode
	srv          *comm_utils.GRPCServer
	bindAddress  string
	clientConfig comm_utils.ClientConfig
	serverConfig comm_utils.ServerConfig
	c            *cluster.Comm
	dispatcher   clusterServer
}

func (cn *clusterNode) Step(stream orderer.Cluster_StepServer) error {
	cn.waitIfFrozen()
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	if submitReq := req.GetSubmitRequest(); submitReq != nil {
		return cn.c.DispatchSubmit(stream.Context(), submitReq)
	}
	if err := cn.c.DispatchConsensus(stream.Context(), req.GetConsensusRequest()); err != nil {
		return err
	}
	return stream.Send(&orderer.StepResponse{})
}

func (cn *clusterNode) waitIfFrozen() {
	cn.lock.Lock()
	// There is no freeze after an unfreeze so no need
	// for a for loop.
	if cn.frozen {
		cn.freezeCond.Wait()
		return
	}
	cn.lock.Unlock()
}

func (cn *clusterNode) freeze() {
	cn.lock.Lock()
	defer cn.lock.Unlock()
	cn.frozen = true
}

func (cn *clusterNode) unfreeze() {
	cn.lock.Lock()
	cn.frozen = false
	cn.lock.Unlock()
	cn.freezeCond.Broadcast()
}

func (cn *clusterNode) resurrect() {
	gRPCServer, err := comm_utils.NewGRPCServer(cn.bindAddress, cn.serverConfig)
	if err != nil {
		panic(fmt.Errorf("failed starting gRPC server: %v", err))
	}
	cn.srv = gRPCServer
	orderer.RegisterClusterServer(gRPCServer.Server(), cn.dispatcher)
	go cn.srv.Start()
}

func (cn *clusterNode) stop() {
	cn.srv.Stop()
	cn.c.Shutdown()
}

func (cn *clusterNode) renewCertificates() {
	clientKeyPair, err := ca.NewClientCertKeyPair()
	if err != nil {
		panic(fmt.Errorf("failed creating client certificate %v", err))
	}
	serverKeyPair, err := ca.NewServerCertKeyPair("127.0.0.1")
	if err != nil {
		panic(fmt.Errorf("failed creating server certificate %v", err))
	}

	cn.nodeInfo.ClientTLSCert = clientKeyPair.TLSCert.Raw
	cn.nodeInfo.ServerTLSCert = serverKeyPair.TLSCert.Raw

	cn.serverConfig.SecOpts.Certificate = serverKeyPair.Cert
	cn.serverConfig.SecOpts.Key = serverKeyPair.Key

	cn.dialer.Config.SecOpts.Key = clientKeyPair.Key
	cn.dialer.Config.SecOpts.Certificate = clientKeyPair.Cert
}

func newTestNodeWithMetrics(t *testing.T, metrics cluster.MetricsProvider, tlsConnGauge metrics.Gauge) *clusterNode {
	serverKeyPair, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)

	clientKeyPair, _ := ca.NewClientCertKeyPair()

	handler := &mocks.Handler{}
	clientConfig := comm_utils.ClientConfig{
		AsyncConnect: true,
		DialTimeout:  time.Hour,
		SecOpts: comm_utils.SecureOptions{
			RequireClientCert: true,
			Key:               clientKeyPair.Key,
			Certificate:       clientKeyPair.Cert,
			ServerRootCAs:     [][]byte{ca.CertBytes()},
			UseTLS:            true,
			ClientRootCAs:     [][]byte{ca.CertBytes()},
		},
	}

	dialer := &cluster.PredicateDialer{
		Config: clientConfig,
	}

	srvConfig := comm_utils.ServerConfig{
		SecOpts: comm_utils.SecureOptions{
			Key:         serverKeyPair.Key,
			Certificate: serverKeyPair.Cert,
			UseTLS:      true,
		},
	}
	gRPCServer, err := comm_utils.NewGRPCServer("127.0.0.1:", srvConfig)
	require.NoError(t, err)

	tstSrv := &clusterNode{
		dialer:       dialer,
		clientConfig: clientConfig,
		serverConfig: srvConfig,
		bindAddress:  gRPCServer.Address(),
		handler:      handler,
		nodeInfo: cluster.RemoteNode{
			NodeAddress: cluster.NodeAddress{
				Endpoint: gRPCServer.Address(),
				ID:       nextUnusedID(),
			},
			NodeCerts: cluster.NodeCerts{
				ServerTLSCert: serverKeyPair.TLSCert.Raw,
				ClientTLSCert: clientKeyPair.TLSCert.Raw,
			},
		},
		srv: gRPCServer,
	}

	if tstSrv.dispatcher == nil {
		tstSrv.dispatcher = tstSrv
	}

	tstSrv.freezeCond.L = &tstSrv.lock

	compareCert := cluster.CachePublicKeyComparisons(func(a, b []byte) bool {
		return crypto.CertificatesWithSamePublicKey(a, b) == nil
	})

	tstSrv.c = &cluster.Comm{
		CertExpWarningThreshold: time.Hour,
		SendBufferSize:          1,
		Logger:                  flogging.MustGetLogger("test"),
		Chan2Members:            make(cluster.MembersByChannel),
		H:                       handler,
		ChanExt:                 channelExtractor,
		Connections:             cluster.NewConnectionStore(dialer, tlsConnGauge),
		Metrics:                 cluster.NewMetrics(metrics),
		CompareCertificate:      compareCert,
	}

	orderer.RegisterClusterServer(gRPCServer.Server(), tstSrv.dispatcher)
	go gRPCServer.Start()
	return tstSrv
}

func newTestNode(t *testing.T) *clusterNode {
	return newTestNodeWithMetrics(t, &disabled.Provider{}, &disabled.Gauge{})
}

func TestSendBigMessage(t *testing.T) {
	// Scenario: Basic test that spawns 5 nodes and sends a big message
	// from one of the nodes to the others.
	// A receiver node's Step() server side method (which calls Recv)
	// is frozen until the sender's node Send method returns,
	// Hence - the sender node finishes calling Send
	// before a receiver node starts calling Recv.
	// This ensures that Send is non blocking even with big messages.
	// In the test, we send a total of 8MB of random data (2MB to each node).
	// The randomness is used so gRPC compression won't compress it to a lower size.

	node1 := newTestNode(t)
	node2 := newTestNode(t)
	node3 := newTestNode(t)
	node4 := newTestNode(t)
	node5 := newTestNode(t)

	for _, node := range []*clusterNode{node2, node3, node4, node5} {
		node.c.SendBufferSize = 1
	}

	defer node1.stop()
	defer node2.stop()
	defer node3.stop()
	defer node4.stop()
	defer node5.stop()

	config := []cluster.RemoteNode{node1.nodeInfo, node2.nodeInfo, node3.nodeInfo, node4.nodeInfo, node5.nodeInfo}
	node1.c.Configure(testChannel, config)
	node2.c.Configure(testChannel, config)
	node3.c.Configure(testChannel, config)
	node4.c.Configure(testChannel, config)
	node5.c.Configure(testChannel, config)

	var messageReceived sync.WaitGroup
	messageReceived.Add(4)

	msgSize := 1024 * 1024 * 2
	bigMsg := &orderer.ConsensusRequest{
		Channel: testChannel,
		Payload: make([]byte, msgSize),
	}

	_, err := rand.Read(bigMsg.Payload)
	require.NoError(t, err)

	wrappedMsg := &orderer.StepRequest{
		Payload: &orderer.StepRequest_ConsensusRequest{
			ConsensusRequest: bigMsg,
		},
	}

	for _, node := range []*clusterNode{node2, node3, node4, node5} {
		node.handler.On("OnConsensus", testChannel, node1.nodeInfo.ID, mock.Anything).Run(func(args mock.Arguments) {
			msg := args.Get(2).(*orderer.ConsensusRequest)
			require.Len(t, msg.Payload, msgSize)
			messageReceived.Done()
		}).Return(nil)
	}

	streams := map[uint64]*cluster.Stream{}

	for _, node := range []*clusterNode{node2, node3, node4, node5} {
		// Freeze the node, in order to block its Recv
		node.freeze()
	}

	for _, node := range []*clusterNode{node2, node3, node4, node5} {
		rm, err := node1.c.Remote(testChannel, node.nodeInfo.ID)
		require.NoError(t, err)

		stream := assertEventualEstablishStream(t, rm)
		streams[node.nodeInfo.ID] = stream
	}

	t0 := time.Now()
	for _, node := range []*clusterNode{node2, node3, node4, node5} {
		stream := streams[node.nodeInfo.ID]

		t1 := time.Now()
		err = stream.Send(wrappedMsg)
		require.NoError(t, err)
		t.Log("Sending took", time.Since(t1))

		// Unfreeze the node. It can now call Recv, and signal the messageReceived waitGroup.
		node.unfreeze()
	}

	t.Log("Total sending time to all 4 nodes took:", time.Since(t0))

	messageReceived.Wait()
}

func TestBlockingSend(t *testing.T) {
	// Scenario: Basic test that spawns 2 nodes and sends from the first node
	// to the second node, three SubmitRequests, or three consensus requests.
	// SubmitRequests should block, but consensus requests should not.

	for _, testCase := range []struct {
		description        string
		messageToSend      *orderer.StepRequest
		streamUnblocks     bool
		elapsedGreaterThan time.Duration
		overflowErr        string
	}{
		{
			description:        "SubmitRequest",
			messageToSend:      wrapSubmitReq(testReq),
			streamUnblocks:     true,
			elapsedGreaterThan: time.Second / 2,
		},
		{
			description:   "ConsensusRequest",
			messageToSend: testConsensusReq,
			overflowErr:   "send queue overflown",
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			node1 := newTestNode(t)
			node2 := newTestNode(t)

			node1.c.SendBufferSize = 1
			node2.c.SendBufferSize = 1

			defer node1.stop()
			defer node2.stop()

			config := []cluster.RemoteNode{node1.nodeInfo, node2.nodeInfo}
			node1.c.Configure(testChannel, config)
			node2.c.Configure(testChannel, config)
			node2.handler.On("OnSubmit", testChannel, node1.nodeInfo.ID, mock.Anything).Return(errors.Errorf("whoops")).Once()

			rm, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
			require.NoError(t, err)

			fakeStream := &mocks.StepClientStream{}
			rm.GetStreamFunc = func(ctx context.Context) (cluster.StepClientStream, error) {
				return fakeStream, nil
			}

			rm.ProbeConn = func(_ *grpc.ClientConn) error {
				return nil
			}

			fakeStream.On("Context", mock.Anything).Return(context.Background())
			fakeStream.On("Auth").Return(nil).Once()

			unBlock := make(chan struct{})
			var sendInvoked sync.WaitGroup
			sendInvoked.Add(1)
			var once sync.Once
			fakeStream.On("Send", mock.Anything).Run(func(_ mock.Arguments) {
				once.Do(sendInvoked.Done)
				<-unBlock
			}).Return(errors.New("oops"))

			stream, err := rm.NewStream(time.Hour)
			require.NoError(t, err)

			// The first send doesn't block, even though the Send operation blocks.
			err = stream.Send(testCase.messageToSend)
			require.NoError(t, err)

			// The second once doesn't either.
			// After this point, we have 1 goroutine which is blocked on Send(),
			// and one message in the buffer.
			sendInvoked.Wait()
			err = stream.Send(testCase.messageToSend)
			require.NoError(t, err)

			// The third blocks, so we need to unblock it ourselves
			// in order for it to go through, unless the operation
			// is non blocking.
			go func() {
				time.Sleep(time.Second)
				if testCase.streamUnblocks {
					close(unBlock)
				}
			}()

			t1 := time.Now()
			err = stream.Send(testCase.messageToSend)
			// The third send always overflows or blocks.
			// If we expect to receive an overflow error - assert it.
			if testCase.overflowErr != "" {
				require.EqualError(t, err, testCase.overflowErr)
			}
			elapsed := time.Since(t1)
			t.Log("Elapsed time:", elapsed)
			require.True(t, elapsed > testCase.elapsedGreaterThan)

			if !testCase.streamUnblocks {
				close(unBlock)
			}
		})
	}
}

func TestEmptyRequest(t *testing.T) {
	// Scenario: Ensures empty messages are discarded and an error is returned
	// back to the sender.

	node1 := newTestNode(t)
	node2 := newTestNode(t)

	node2.srv.Stop()
	svc := &cluster.Service{
		StepLogger: flogging.MustGetLogger("test"),
		Logger:     flogging.MustGetLogger("test"),
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: cluster.NewMetrics(&disabled.Provider{}),
		},
		Dispatcher: node2.c,
	}
	node2.dispatcher = svc

	// Sleep to let the gRPC service be closed
	time.Sleep(time.Second)

	// Resurrect the node with the new dispatcher
	node2.resurrect()

	defer node1.stop()
	defer node2.stop()

	config := []cluster.RemoteNode{node1.nodeInfo, node2.nodeInfo}
	node1.c.Configure(testChannel, config)
	node2.c.Configure(testChannel, config)

	assertBiDiCommunication(t, node1, node2, testReq)

	rm, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
	require.NoError(t, err)

	stream, err := rm.NewStream(time.Second * 10)
	require.NoError(t, err)

	err = stream.Send(&orderer.StepRequest{})
	require.NoError(t, err)

	_, err = stream.Recv()
	require.Error(t, err, "message is neither a Submit nor a Consensus request")
}

func TestBasic(t *testing.T) {
	// Scenario: Basic test that spawns 2 nodes and sends each other
	// messages that are expected to be echoed back

	node1 := newTestNode(t)
	node2 := newTestNode(t)

	defer node1.stop()
	defer node2.stop()

	config := []cluster.RemoteNode{node1.nodeInfo, node2.nodeInfo}
	node1.c.Configure(testChannel, config)
	node2.c.Configure(testChannel, config)

	assertBiDiCommunication(t, node1, node2, testReq)
}

func TestUnavailableHosts(t *testing.T) {
	// Scenario: A node is configured to connect
	// to a host that is down
	node1 := newTestNode(t)

	clientConfig := node1.dialer.Config
	// The below timeout makes sure that connection establishment is done
	// asynchronously. Had it been synchronous, the Remote() call would be
	// blocked for an hour.
	clientConfig.DialTimeout = time.Hour
	defer node1.stop()

	node2 := newTestNode(t)
	node2.stop()

	node1.c.Configure(testChannel, []cluster.RemoteNode{node2.nodeInfo})
	remote, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
	require.NoError(t, err)
	require.NotNil(t, remote)

	_, err = remote.NewStream(time.Millisecond * 100)
	require.Contains(t, err.Error(), "connection")
}

func TestStreamAbortReportCorrectError(t *testing.T) {
	// Scenario: node 1 acquires a stream to node 2 and then the stream
	// encounters an error and as a result, the stream is aborted.
	// We ensure the error reported is the first error, even after
	// multiple attempts of using it.

	node1 := newTestNode(t)
	defer node1.stop()

	node2 := newTestNode(t)
	defer node2.stop()

	node1.c.Configure(testChannel, []cluster.RemoteNode{node2.nodeInfo})
	node2.c.Configure(testChannel, []cluster.RemoteNode{node1.nodeInfo})

	node2.handler.On("OnSubmit", testChannel, node1.nodeInfo.ID, mock.Anything).Return(errors.Errorf("whoops")).Once()

	rm1, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
	require.NoError(t, err)
	var streamTerminated sync.WaitGroup
	streamTerminated.Add(1)

	stream := assertEventualEstablishStream(t, rm1)

	l, err := zap.NewDevelopment()
	require.NoError(t, err)
	stream.Logger = flogging.NewFabricLogger(l, zap.Hooks(func(entry zapcore.Entry) error {
		if strings.Contains(entry.Message, "Stream 1 to") && strings.Contains(entry.Message, "terminated") {
			streamTerminated.Done()
		}
		return nil
	}))

	// Probe the stream for the first time
	err = stream.Send(wrapSubmitReq(testReq))
	require.NoError(t, err)

	// We should receive back the crafted error
	_, err = stream.Recv()
	require.Contains(t, err.Error(), "whoops")

	// Wait for the stream to be terminated from within the communication infrastructure
	streamTerminated.Wait()

	// We should still receive the original crafted error despite the stream being terminated
	err = stream.Send(wrapSubmitReq(testReq))
	require.Contains(t, err.Error(), "whoops")
}

func TestStreamAbort(t *testing.T) {
	// Scenarios: node 1 is connected to node 2 in 2 channels,
	// and the consumer of the communication calls receive.
	// The two sub-scenarios happen:
	// 1) The server certificate of node 2 changes in the first channel
	// 2) Node 2 is evicted from the membership of the first channel
	// In both of the scenarios, the Recv() call should be aborted

	node2 := newTestNode(t)
	defer node2.stop()

	invalidNodeInfo := cluster.RemoteNode{
		NodeAddress: cluster.NodeAddress{ID: node2.nodeInfo.ID},
		NodeCerts: cluster.NodeCerts{
			ServerTLSCert: []byte{1, 2, 3},
			ClientTLSCert: []byte{1, 2, 3},
		},
	}

	for _, tst := range []struct {
		testName      string
		membership    []cluster.RemoteNode
		expectedError string
	}{
		{
			testName:      "Evicted from membership",
			membership:    nil,
			expectedError: "rpc error: code = Canceled desc = context canceled",
		},
		{
			testName:      "Changed TLS certificate",
			membership:    []cluster.RemoteNode{invalidNodeInfo},
			expectedError: "rpc error: code = Canceled desc = context canceled",
		},
	} {
		t.Run(tst.testName, func(t *testing.T) {
			testStreamAbort(t, node2, tst.membership, tst.expectedError)
		})
	}
	node2.handler.AssertNumberOfCalls(t, "OnSubmit", 2)
}

func testStreamAbort(t *testing.T, node2 *clusterNode, newMembership []cluster.RemoteNode, expectedError string) {
	node1 := newTestNode(t)
	defer node1.stop()

	node1.c.Configure(testChannel, []cluster.RemoteNode{node2.nodeInfo})
	node2.c.Configure(testChannel, []cluster.RemoteNode{node1.nodeInfo})
	node1.c.Configure(testChannel2, []cluster.RemoteNode{node2.nodeInfo})
	node2.c.Configure(testChannel2, []cluster.RemoteNode{node1.nodeInfo})

	var streamCreated sync.WaitGroup
	streamCreated.Add(1)

	stopChan := make(chan struct{})

	node2.handler.On("OnSubmit", testChannel, node1.nodeInfo.ID, mock.Anything).Once().Run(func(_ mock.Arguments) {
		// Notify the stream was created
		streamCreated.Done()
		// Wait for the test to finish
		<-stopChan
	}).Return(nil).Once()

	rm1, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
	require.NoError(t, err)

	go func() {
		stream := assertEventualEstablishStream(t, rm1)
		// Signal the reconfiguration
		err = stream.Send(wrapSubmitReq(testReq))
		require.NoError(t, err)
		_, err := stream.Recv()
		require.Contains(t, err.Error(), expectedError)
		close(stopChan)
	}()

	go func() {
		// Wait for the stream reference to be obtained
		streamCreated.Wait()
		// Reconfigure the channel membership
		node1.c.Configure(testChannel, newMembership)
	}()

	<-stopChan
}

func TestDoubleReconfigure(t *testing.T) {
	// Scenario: Basic test that spawns 2 nodes
	// and configures node 1 twice, and checks that
	// the remote stub for node 1 wasn't re-created in the second
	// configuration since it already existed

	node1 := newTestNode(t)
	node2 := newTestNode(t)

	defer node1.stop()
	defer node2.stop()

	node1.c.Configure(testChannel, []cluster.RemoteNode{node2.nodeInfo})
	rm1, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
	require.NoError(t, err)

	node1.c.Configure(testChannel, []cluster.RemoteNode{node2.nodeInfo})
	rm2, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
	require.NoError(t, err)
	// Ensure the references are equal
	require.True(t, rm1 == rm2)
}

func TestInvalidChannel(t *testing.T) {
	// Scenario: node 1 it ordered to send a message on a channel
	// that doesn't exist, and also receives a message, but
	// the channel cannot be extracted from the message.

	t.Run("channel doesn't exist", func(t *testing.T) {
		node1 := newTestNode(t)
		defer node1.stop()

		_, err := node1.c.Remote(testChannel, 0)
		require.EqualError(t, err, "channel test doesn't exist")
	})

	t.Run("channel cannot be extracted", func(t *testing.T) {
		node1 := newTestNode(t)
		defer node1.stop()

		node1.c.Configure(testChannel, []cluster.RemoteNode{node1.nodeInfo})
		gt := gomega.NewGomegaWithT(t)
		gt.Eventually(func() (bool, error) {
			_, err := node1.c.Remote(testChannel, node1.nodeInfo.ID)
			return true, err
		}, time.Minute).Should(gomega.BeTrue())

		stub, err := node1.c.Remote(testChannel, node1.nodeInfo.ID)
		require.NoError(t, err)

		stream := assertEventualEstablishStream(t, stub)

		// An empty SubmitRequest has an empty channel which is invalid
		err = stream.Send(wrapSubmitReq(&orderer.SubmitRequest{}))
		require.NoError(t, err)

		_, err = stream.Recv()
		require.EqualError(t, err, "rpc error: code = Unknown desc = badly formatted message, cannot extract channel")

		// Test directly without going through the gRPC stream
		err = node1.c.DispatchSubmit(context.Background(), &orderer.SubmitRequest{})
		require.EqualError(t, err, "badly formatted message, cannot extract channel")
	})
}

func TestAbortRPC(t *testing.T) {
	// Scenarios:
	// (I) The node calls an RPC, and calls Abort() on the remote context
	//  in parallel. The RPC should return even though the server-side call hasn't finished.
	// (II) The node calls an RPC, but the server-side processing takes too long,
	// and the RPC invocation returns prematurely.

	testCases := []struct {
		name        string
		abortFunc   func(*cluster.RemoteContext)
		rpcTimeout  time.Duration
		expectedErr string
	}{
		{
			name:        "Abort() called",
			expectedErr: "rpc error: code = Canceled desc = context canceled",
			rpcTimeout:  time.Hour,
			abortFunc: func(rc *cluster.RemoteContext) {
				rc.Abort()
			},
		},
		{
			name:        "RPC timeout",
			expectedErr: "rpc timeout expired",
			rpcTimeout:  time.Second,
			abortFunc:   func(*cluster.RemoteContext) {},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			testAbort(t, testCase.abortFunc, testCase.rpcTimeout, testCase.expectedErr)
		})
	}
}

func testAbort(t *testing.T, abortFunc func(*cluster.RemoteContext), rpcTimeout time.Duration, expectedErr string) {
	node1 := newTestNode(t)
	defer node1.stop()

	node2 := newTestNode(t)
	defer node2.stop()

	config := []cluster.RemoteNode{node1.nodeInfo, node2.nodeInfo}
	node1.c.Configure(testChannel, config)
	node2.c.Configure(testChannel, config)
	var onStepCalled sync.WaitGroup
	onStepCalled.Add(1)

	// stuckCall ensures the OnStep() call is stuck throughout this test
	var stuckCall sync.WaitGroup
	stuckCall.Add(1)
	// At the end of the test, release the server-side resources
	defer stuckCall.Done()

	node2.handler.On("OnSubmit", testChannel, node1.nodeInfo.ID, mock.Anything).Return(nil).Once().Run(func(_ mock.Arguments) {
		onStepCalled.Done()
		stuckCall.Wait()
	}).Once()

	rm, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
	require.NoError(t, err)

	go func() {
		onStepCalled.Wait()
		abortFunc(rm)
	}()

	var stream *cluster.Stream
	gt := gomega.NewGomegaWithT(t)
	gt.Eventually(func() error {
		stream, err = rm.NewStream(rpcTimeout)
		return err
	}, time.Second*10, time.Millisecond*10).Should(gomega.Succeed())

	stream.Send(wrapSubmitReq(testSubReq))
	_, err = stream.Recv()

	require.EqualError(t, err, expectedErr)

	node2.handler.AssertNumberOfCalls(t, "OnSubmit", 1)
}

func TestNoTLSCertificate(t *testing.T) {
	// Scenario: The node is sent a message by another node that doesn't
	// connect with mutual TLS, thus doesn't provide a TLS certificate
	node1 := newTestNode(t)
	defer node1.stop()

	node1.c.Configure(testChannel, []cluster.RemoteNode{node1.nodeInfo})

	clientConfig := comm_utils.ClientConfig{
		AsyncConnect: true,
		DialTimeout:  time.Millisecond * 100,
		SecOpts: comm_utils.SecureOptions{
			ServerRootCAs: [][]byte{ca.CertBytes()},
			UseTLS:        true,
		},
	}

	var conn *grpc.ClientConn
	gt := gomega.NewGomegaWithT(t)
	gt.Eventually(func() (bool, error) {
		var err error
		conn, err = clientConfig.Dial(node1.srv.Address())
		return true, err
	}, time.Minute).Should(gomega.BeTrue())

	echoClient := orderer.NewClusterClient(conn)
	stream, err := echoClient.Step(context.Background())
	require.NoError(t, err)

	err = stream.Send(wrapSubmitReq(testSubReq))
	require.NoError(t, err)
	_, err = stream.Recv()
	require.EqualError(t, err, "rpc error: code = Unknown desc = no TLS certificate sent")
}

func TestReconnect(t *testing.T) {
	// Scenario: node 1 and node 2 are connected,
	// and node 2 is taken offline.
	// Node 1 tries to send a message to node 2 but fails,
	// and afterwards node 2 is brought back, after which
	// node 1 sends more messages, and it should succeed
	// sending a message to node 2 eventually.

	node1 := newTestNode(t)
	defer node1.stop()
	conf := node1.dialer.Config
	conf.DialTimeout = time.Hour

	node2 := newTestNode(t)
	node2.handler.On("OnSubmit", testChannel, node1.nodeInfo.ID, mock.Anything).Return(nil)
	defer node2.stop()

	config := []cluster.RemoteNode{node1.nodeInfo, node2.nodeInfo}
	node1.c.Configure(testChannel, config)
	node2.c.Configure(testChannel, config)

	// Make node 2 be offline by shutting down its gRPC service
	node2.srv.Stop()
	// Obtain the stub for node 2.
	// Should succeed, because the connection was created at time of configuration
	stub, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
	require.NoError(t, err)

	// Try to obtain a stream. Should not Succeed.
	gt := gomega.NewGomegaWithT(t)
	gt.Eventually(func() error {
		_, err = stub.NewStream(time.Hour)
		return err
	}).Should(gomega.Not(gomega.Succeed()))

	// Wait for the port to be released
	for {
		lsnr, err := net.Listen("tcp", node2.nodeInfo.Endpoint)
		if err == nil {
			lsnr.Close()
			break
		}
	}

	// Resurrect node 2
	node2.resurrect()
	// Send a message from node 1 to node 2.
	// Should succeed eventually
	assertEventualSendMessage(t, stub, testReq)
}

func TestRenewCertificates(t *testing.T) {
	// Scenario: node 1 and node 2 are connected,
	// Node 2's certificate is renewed, and
	// node 1 is reconfigured with the new
	// configuration without being restarted.

	node1 := newTestNode(t)
	defer node1.stop()

	node2 := newTestNode(t)
	defer node2.stop()

	node1.handler.On("OnStep", testChannel, node2.nodeInfo.ID, mock.Anything).Return(testRes, nil)
	node2.handler.On("OnStep", testChannel, node1.nodeInfo.ID, mock.Anything).Return(testRes, nil)

	config := []cluster.RemoteNode{node1.nodeInfo, node2.nodeInfo}
	node1.c.Configure(testChannel, config)
	node2.c.Configure(testChannel, config)

	assertBiDiCommunication(t, node1, node2, testReq)

	// Close outgoing connections from node2 to node1
	node2.c.Configure(testChannel, nil)
	// Stop the gRPC service of node 2 to replace its certificate
	node2.srv.Stop()

	// Wait until node 1 detects this
	gt := gomega.NewGomegaWithT(t)
	gt.Eventually(func() error {
		remote, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
		if err != nil {
			return err
		}
		stream, err := remote.NewStream(time.Hour)
		if err != nil {
			return err
		}
		err = stream.Send(wrapSubmitReq(testSubReq))
		if err != nil {
			return err
		}
		return nil
	}).Should(gomega.Not(gomega.Succeed()))

	// Renew node 2's keys
	node2.renewCertificates()

	// Resurrect node 2 to make it service connections again
	node2.resurrect()

	// W.L.O.G, try to send a message from node1 to node2
	// It should fail, because node2's server certificate has now changed,
	// so it closed the connection to the remote node
	info2 := node2.nodeInfo
	remote, err := node1.c.Remote(testChannel, info2.ID)
	require.NoError(t, err)
	require.NotNil(t, remote)
	_, err = remote.NewStream(time.Hour)
	require.Contains(t, err.Error(), info2.Endpoint)

	// Reconfigure both nodes with the updates keys
	config = []cluster.RemoteNode{node1.nodeInfo, node2.nodeInfo}
	node1.c.Configure(testChannel, config)
	node2.c.Configure(testChannel, config)

	// Finally, check that the nodes can communicate once again
	assertBiDiCommunication(t, node1, node2, testReq)
}

func TestMembershipReconfiguration(t *testing.T) {
	// Scenario: node 1 and node 2 are started up
	// and node 2 is configured to know about node 1,
	// without node1 knowing about node 2.
	// The communication between them should only work
	// after node 1 is configured to know about node 2.

	node1 := newTestNode(t)
	defer node1.stop()

	node2 := newTestNode(t)
	defer node2.stop()

	node1.c.Configure(testChannel, []cluster.RemoteNode{})
	node2.c.Configure(testChannel, []cluster.RemoteNode{node1.nodeInfo})

	// Node 1 can't connect to node 2 because it doesn't know its TLS certificate yet
	_, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
	require.EqualError(t, err, fmt.Sprintf("node %d doesn't exist in channel test's membership", node2.nodeInfo.ID))
	// Node 2 can connect to node 1, but it can't send it messages because node 1 doesn't know node 2 yet.

	gt := gomega.NewGomegaWithT(t)
	gt.Eventually(func() (bool, error) {
		_, err := node2.c.Remote(testChannel, node1.nodeInfo.ID)
		return true, err
	}, time.Minute).Should(gomega.BeTrue())

	stub, err := node2.c.Remote(testChannel, node1.nodeInfo.ID)
	require.NoError(t, err)

	stream := assertEventualEstablishStream(t, stub)
	err = stream.Send(wrapSubmitReq(testSubReq))
	require.NoError(t, err)

	_, err = stream.Recv()
	require.EqualError(t, err, "rpc error: code = Unknown desc = certificate extracted from TLS connection isn't authorized")

	// Next, configure node 1 to know about node 2
	node1.c.Configure(testChannel, []cluster.RemoteNode{node2.nodeInfo})

	// Check that the communication works correctly between both nodes
	assertBiDiCommunication(t, node1, node2, testReq)
	assertBiDiCommunication(t, node2, node1, testReq)

	// Reconfigure node 2 to forget about node 1
	node2.c.Configure(testChannel, []cluster.RemoteNode{})
	// Node 1 can still connect to node 2
	stub, err = node1.c.Remote(testChannel, node2.nodeInfo.ID)
	require.NoError(t, err)
	// But can't send a message because node 2 now doesn't authorized node 1
	stream = assertEventualEstablishStream(t, stub)
	stream.Send(wrapSubmitReq(testSubReq))
	_, err = stream.Recv()
	require.EqualError(t, err, "rpc error: code = Unknown desc = certificate extracted from TLS connection isn't authorized")
}

func TestShutdown(t *testing.T) {
	// Scenario: node 1 is shut down and as a result, can't
	// send messages to anyone, nor can it be reconfigured

	node1 := newTestNode(t)
	defer node1.stop()

	node1.c.Shutdown()

	// Obtaining a RemoteContext cannot succeed because shutdown was called before
	_, err := node1.c.Remote(testChannel, node1.nodeInfo.ID)
	require.EqualError(t, err, "communication has been shut down")

	node2 := newTestNode(t)
	defer node2.stop()

	node2.c.Configure(testChannel, []cluster.RemoteNode{node1.nodeInfo})
	// Configuration of node doesn't take place
	node1.c.Configure(testChannel, []cluster.RemoteNode{node2.nodeInfo})

	gt := gomega.NewGomegaWithT(t)
	gt.Eventually(func() error {
		_, err := node2.c.Remote(testChannel, node1.nodeInfo.ID)
		return err
	}, time.Minute).Should(gomega.Succeed())

	stub, err := node2.c.Remote(testChannel, node1.nodeInfo.ID)
	require.NoError(t, err)

	// Therefore, sending a message doesn't succeed because node 1 rejected the configuration change
	gt.Eventually(func() string {
		stream, err := stub.NewStream(time.Hour)
		if err != nil {
			return err.Error()
		}
		err = stream.Send(wrapSubmitReq(testSubReq))
		require.NoError(t, err)

		_, err = stream.Recv()
		return err.Error()
	}, timeout).Should(gomega.ContainSubstring("channel test doesn't exist"))
}

func TestMultiChannelConfig(t *testing.T) {
	// Scenario: node 1 is knows node 2 only in channel "foo"
	// and knows node 3 only in channel "bar".
	// Messages that are received, are routed according to their corresponding channels
	// and when node 2 sends a message for channel "bar" to node 1, it is rejected.
	// Same thing applies for node 3 that sends a message to node 1 in channel "foo".

	node1 := newTestNode(t)
	defer node1.stop()

	node2 := newTestNode(t)
	defer node2.stop()

	node3 := newTestNode(t)
	defer node3.stop()

	node1.c.Configure("foo", []cluster.RemoteNode{node2.nodeInfo})
	node1.c.Configure("bar", []cluster.RemoteNode{node3.nodeInfo})
	node2.c.Configure("foo", []cluster.RemoteNode{node1.nodeInfo})
	node3.c.Configure("bar", []cluster.RemoteNode{node1.nodeInfo})

	t.Run("Correct channel", func(t *testing.T) {
		var fromNode2 sync.WaitGroup
		fromNode2.Add(1)
		node1.handler.On("OnSubmit", "foo", node2.nodeInfo.ID, mock.Anything).Return(nil).Run(func(_ mock.Arguments) {
			fromNode2.Done()
		}).Once()

		var fromNode3 sync.WaitGroup
		fromNode3.Add(1)
		node1.handler.On("OnSubmit", "bar", node3.nodeInfo.ID, mock.Anything).Return(nil).Run(func(_ mock.Arguments) {
			fromNode3.Done()
		}).Once()

		node2toNode1, err := node2.c.Remote("foo", node1.nodeInfo.ID)
		require.NoError(t, err)
		node3toNode1, err := node3.c.Remote("bar", node1.nodeInfo.ID)
		require.NoError(t, err)

		stream := assertEventualEstablishStream(t, node2toNode1)
		stream.Send(fooReq)

		fromNode2.Wait()
		node1.handler.AssertNumberOfCalls(t, "OnSubmit", 1)

		stream = assertEventualEstablishStream(t, node3toNode1)
		stream.Send(barReq)

		fromNode3.Wait()
		node1.handler.AssertNumberOfCalls(t, "OnSubmit", 2)
	})

	t.Run("Incorrect channel", func(t *testing.T) {
		node1.handler.On("OnSubmit", "foo", node2.nodeInfo.ID, mock.Anything).Return(nil)
		node1.handler.On("OnSubmit", "bar", node3.nodeInfo.ID, mock.Anything).Return(nil)

		node2toNode1, err := node2.c.Remote("foo", node1.nodeInfo.ID)
		require.NoError(t, err)
		node3toNode1, err := node3.c.Remote("bar", node1.nodeInfo.ID)
		require.NoError(t, err)

		assertEventualSendMessage(t, node2toNode1, &orderer.SubmitRequest{Channel: "foo"})
		require.NoError(t, err)
		stream, err := node2toNode1.NewStream(time.Hour)
		require.NoError(t, err)
		err = stream.Send(barReq)
		require.NoError(t, err)
		_, err = stream.Recv()
		require.EqualError(t, err, "rpc error: code = Unknown desc = certificate extracted from TLS connection isn't authorized")

		assertEventualSendMessage(t, node3toNode1, &orderer.SubmitRequest{Channel: "bar"})
		stream, err = node3toNode1.NewStream(time.Hour)
		require.NoError(t, err)
		err = stream.Send(fooReq)
		require.NoError(t, err)
		_, err = stream.Recv()
		require.EqualError(t, err, "rpc error: code = Unknown desc = certificate extracted from TLS connection isn't authorized")
	})
}

func TestConnectionFailure(t *testing.T) {
	// Scenario: node 1 fails to connect to node 2.

	node1 := newTestNode(t)
	defer node1.stop()

	node2 := newTestNode(t)
	defer node2.stop()

	dialer := &mocks.SecureDialer{}
	dialer.On("Dial", mock.Anything, mock.Anything).Return(nil, errors.New("oops"))
	node1.c.Connections = cluster.NewConnectionStore(dialer, &disabled.Gauge{})
	node1.c.Configure(testChannel, []cluster.RemoteNode{node2.nodeInfo})

	_, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
	require.EqualError(t, err, "oops")
}

type testMetrics struct {
	fakeProvider        *mocks.MetricsProvider
	egressQueueLength   metricsfakes.Gauge
	egressQueueCapacity metricsfakes.Gauge
	egressStreamCount   metricsfakes.Gauge
	egressTLSConnCount  metricsfakes.Gauge
	egressWorkerSize    metricsfakes.Gauge
	ingressStreamsCount metricsfakes.Gauge
	msgSendTime         metricsfakes.Histogram
	msgDropCount        metricsfakes.Counter
}

func (tm *testMetrics) initialize() {
	tm.egressQueueLength.WithReturns(&tm.egressQueueLength)
	tm.egressQueueCapacity.WithReturns(&tm.egressQueueCapacity)
	tm.egressStreamCount.WithReturns(&tm.egressStreamCount)
	tm.egressTLSConnCount.WithReturns(&tm.egressTLSConnCount)
	tm.egressWorkerSize.WithReturns(&tm.egressWorkerSize)
	tm.ingressStreamsCount.WithReturns(&tm.ingressStreamsCount)
	tm.msgSendTime.WithReturns(&tm.msgSendTime)
	tm.msgDropCount.WithReturns(&tm.msgDropCount)

	fakeProvider := tm.fakeProvider
	fakeProvider.On("NewGauge", cluster.IngressStreamsCountOpts).Return(&tm.ingressStreamsCount)
	fakeProvider.On("NewGauge", cluster.EgressQueueLengthOpts).Return(&tm.egressQueueLength)
	fakeProvider.On("NewGauge", cluster.EgressQueueCapacityOpts).Return(&tm.egressQueueCapacity)
	fakeProvider.On("NewGauge", cluster.EgressStreamsCountOpts).Return(&tm.egressStreamCount)
	fakeProvider.On("NewGauge", cluster.EgressTLSConnectionCountOpts).Return(&tm.egressTLSConnCount)
	fakeProvider.On("NewGauge", cluster.EgressWorkersOpts).Return(&tm.egressWorkerSize)
	fakeProvider.On("NewCounter", cluster.MessagesDroppedCountOpts).Return(&tm.msgDropCount)
	fakeProvider.On("NewHistogram", cluster.MessageSendTimeOpts).Return(&tm.msgSendTime)
}

func TestMetrics(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		runTest     func(t *testing.T, node1, node2 *clusterNode, testMetrics *testMetrics)
		testMetrics *testMetrics
	}{
		{
			name: "EgressQueueOccupancy",
			runTest: func(t *testing.T, node1, node2 *clusterNode, testMetrics *testMetrics) {
				assertBiDiCommunication(t, node1, node2, testReq)
				require.Equal(t, []string{"host", node2.nodeInfo.Endpoint, "msg_type", "transaction", "channel", testChannel},
					testMetrics.egressQueueLength.WithArgsForCall(0))
				require.Equal(t, float64(0), testMetrics.egressQueueLength.SetArgsForCall(0))
				require.Equal(t, float64(1), testMetrics.egressQueueCapacity.SetArgsForCall(0))

				var messageReceived sync.WaitGroup
				messageReceived.Add(1)
				node2.handler.On("OnConsensus", testChannel, node1.nodeInfo.ID, mock.Anything).Run(func(args mock.Arguments) {
					messageReceived.Done()
				}).Return(nil)

				rm, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
				require.NoError(t, err)

				stream := assertEventualEstablishStream(t, rm)
				stream.Send(testConsensusReq)
				messageReceived.Wait()

				require.Equal(t, []string{"host", node2.nodeInfo.Endpoint, "msg_type", "consensus", "channel", testChannel},
					testMetrics.egressQueueLength.WithArgsForCall(1))
				require.Equal(t, float64(0), testMetrics.egressQueueLength.SetArgsForCall(1))
				require.Equal(t, float64(1), testMetrics.egressQueueCapacity.SetArgsForCall(1))
			},
		},
		{
			name: "EgressStreamsCount",
			runTest: func(t *testing.T, node1, node2 *clusterNode, testMetrics *testMetrics) {
				assertBiDiCommunication(t, node1, node2, testReq)
				require.Equal(t, 1, testMetrics.egressStreamCount.SetCallCount())
				require.Equal(t, 1, testMetrics.egressStreamCount.WithCallCount())
				require.Equal(t, []string{"channel", testChannel}, testMetrics.egressStreamCount.WithArgsForCall(0))

				assertBiDiCommunicationForChannel(t, node1, node2, testReq2, testChannel2)
				require.Equal(t, 2, testMetrics.egressStreamCount.SetCallCount())
				require.Equal(t, 2, testMetrics.egressStreamCount.WithCallCount())
				require.Equal(t, []string{"channel", testChannel2}, testMetrics.egressStreamCount.WithArgsForCall(1))
			},
		},
		{
			name: "EgressTLSConnCount",
			runTest: func(t *testing.T, node1, node2 *clusterNode, testMetrics *testMetrics) {
				assertBiDiCommunication(t, node1, node2, testReq)
				require.Equal(t, []string{"channel", testChannel}, testMetrics.egressStreamCount.WithArgsForCall(0))

				assertBiDiCommunicationForChannel(t, node1, node2, testReq2, testChannel2)
				require.Equal(t, []string{"channel", testChannel2}, testMetrics.egressStreamCount.WithArgsForCall(1))

				// A single TLS connection despite 2 streams
				require.Equal(t, float64(1), testMetrics.egressTLSConnCount.SetArgsForCall(0))
				require.Equal(t, 1, testMetrics.egressTLSConnCount.SetCallCount())
			},
		},
		{
			name: "EgressWorkerSize",
			runTest: func(t *testing.T, node1, node2 *clusterNode, testMetrics *testMetrics) {
				assertBiDiCommunication(t, node1, node2, testReq)
				require.Equal(t, []string{"channel", testChannel}, testMetrics.egressStreamCount.WithArgsForCall(0))

				assertBiDiCommunicationForChannel(t, node1, node2, testReq2, testChannel2)
				require.Equal(t, []string{"channel", testChannel2}, testMetrics.egressStreamCount.WithArgsForCall(1))

				require.Equal(t, float64(1), testMetrics.egressWorkerSize.SetArgsForCall(0))
				require.Equal(t, float64(1), testMetrics.egressWorkerSize.SetArgsForCall(1))
			},
		},
		{
			name: "MsgSendTime",
			runTest: func(t *testing.T, node1, node2 *clusterNode, testMetrics *testMetrics) {
				assertBiDiCommunication(t, node1, node2, testReq)
				require.Eventually(t, func() bool { return testMetrics.msgSendTime.ObserveCallCount() > 0 }, time.Second, 10*time.Millisecond)
				require.Equal(t, 1, testMetrics.msgSendTime.ObserveCallCount())
				require.Equal(t, []string{"host", node2.nodeInfo.Endpoint, "channel", testChannel}, testMetrics.msgSendTime.WithArgsForCall(0))
			},
		},
		{
			name: "MsgDropCount",
			runTest: func(t *testing.T, node1, node2 *clusterNode, testMetrics *testMetrics) {
				blockRecv := make(chan struct{})
				wasReported := func() bool {
					select {
					case <-blockRecv:
						return true
					default:
						return false
					}
				}
				// When the drop count is reported, release the lock on the server side receive operation.
				testMetrics.msgDropCount.AddStub = func(float642 float64) {
					if !wasReported() {
						close(blockRecv)
					}
				}

				node2.handler.On("OnConsensus", testChannel, node1.nodeInfo.ID, mock.Anything).Run(func(args mock.Arguments) {
					// Block until the message drop is reported
					<-blockRecv
				}).Return(nil)

				rm, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
				require.NoError(t, err)

				stream := assertEventualEstablishStream(t, rm)
				// Send too many messages while the server side is not reading from the stream
				for {
					stream.Send(testConsensusReq)
					if wasReported() {
						break
					}
				}
				require.Equal(t, []string{"host", node2.nodeInfo.Endpoint, "channel", testChannel},
					testMetrics.msgDropCount.WithArgsForCall(0))
				require.Equal(t, 1, testMetrics.msgDropCount.AddCallCount())
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fakeProvider := &mocks.MetricsProvider{}
			testCase.testMetrics = &testMetrics{
				fakeProvider: fakeProvider,
			}

			testCase.testMetrics.initialize()

			node1 := newTestNodeWithMetrics(t, fakeProvider, &testCase.testMetrics.egressTLSConnCount)
			defer node1.stop()

			node2 := newTestNode(t)
			defer node2.stop()

			configForNode1 := []cluster.RemoteNode{node2.nodeInfo}
			configForNode2 := []cluster.RemoteNode{node1.nodeInfo}
			node1.c.Configure(testChannel, configForNode1)
			node2.c.Configure(testChannel, configForNode2)
			node1.c.Configure(testChannel2, configForNode1)
			node2.c.Configure(testChannel2, configForNode2)

			testCase.runTest(t, node1, node2, testCase.testMetrics)
		})
	}
}

func TestCertExpirationWarningEgress(t *testing.T) {
	// Scenario: Ensures that when certificates are due to expire,
	// a warning is logged to the log.

	node1 := newTestNode(t)
	node2 := newTestNode(t)

	cert, err := x509.ParseCertificate(node2.nodeInfo.ServerTLSCert)
	require.NoError(t, err)
	require.NotNil(t, cert)

	// Let the NotAfter time of the certificate be T1, the current time be T0.
	// So time.Until is (T1 - T0), which means we have (T1 - T0) time left.
	// We want to trigger a warning, so we set the warning threshold to be 20 seconds above
	// the time left, so the time left would be smaller than the threshold.
	node1.c.CertExpWarningThreshold = time.Until(cert.NotAfter) + time.Second*20
	// We only alert once in 3 seconds
	node1.c.MinimumExpirationWarningInterval = time.Second * 3

	defer node1.stop()
	defer node2.stop()

	config := []cluster.RemoteNode{node1.nodeInfo, node2.nodeInfo}
	node1.c.Configure(testChannel, config)
	node2.c.Configure(testChannel, config)

	stub, err := node1.c.Remote(testChannel, node2.nodeInfo.ID)
	require.NoError(t, err)

	node2.handler.On("OnSubmit", testChannel, node1.nodeInfo.ID, mock.Anything).Return(nil)

	mockgRPC := &mocks.StepClient{}
	mockgRPC.On("Send", mock.Anything).Return(nil)
	mockgRPC.On("Context").Return(context.Background())
	mockClient := &mocks.ClusterClient{}
	mockClient.On("Step", mock.Anything).Return(mockgRPC, nil)

	stream := assertEventualEstablishStream(t, stub)

	alerts := make(chan struct{}, 100)

	stream.Logger = stream.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if strings.Contains(entry.Message, "expires in less than") {
			alerts <- struct{}{}
		}
		return nil
	}))

	// Send a message to the node and expert an alert to be logged.
	stream.Send(wrapSubmitReq(testReq))
	select {
	case <-alerts:
	case <-time.After(time.Second * 5):
		t.Fatal("Should have logged an alert")
	}
	// Send another message, and ensure we don't log anything to the log, because the
	// alerts should be suppressed before the minimum interval timeout expires.
	stream.Send(wrapSubmitReq(testReq))
	select {
	case <-alerts:
		t.Fatal("Should not have logged an alert")
	case <-time.After(time.Millisecond * 500):
	}
	// Wait enough time for the alert interval to clear.
	time.Sleep(node1.c.MinimumExpirationWarningInterval + time.Second)
	// Send again a message, and this time it should be logged again.
	stream.Send(wrapSubmitReq(testReq))
	select {
	case <-alerts:
	case <-time.After(time.Second * 5):
		t.Fatal("Should have logged an alert")
	}
}

func assertBiDiCommunicationForChannel(t *testing.T, node1, node2 *clusterNode, msgToSend *orderer.SubmitRequest, channel string) {
	establish := []struct {
		label    string
		sender   *clusterNode
		receiver *clusterNode
		target   uint64
	}{
		{label: "1->2", sender: node1, target: node2.nodeInfo.ID, receiver: node2},
		{label: "2->1", sender: node2, target: node1.nodeInfo.ID, receiver: node1},
	}
	for _, estab := range establish {
		stub, err := estab.sender.c.Remote(channel, estab.target)
		require.NoError(t, err)

		stream := assertEventualEstablishStream(t, stub)

		var wg sync.WaitGroup
		wg.Add(1)
		estab.receiver.handler.On("OnSubmit", channel, estab.sender.nodeInfo.ID, mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			req := args.Get(2).(*orderer.SubmitRequest)
			require.True(t, proto.Equal(req, msgToSend))
			t.Log(estab.label)
			wg.Done()
		})

		err = stream.Send(wrapSubmitReq(msgToSend))
		require.NoError(t, err)

		wg.Wait()
	}
}

func assertBiDiCommunication(t *testing.T, node1, node2 *clusterNode, msgToSend *orderer.SubmitRequest) {
	assertBiDiCommunicationForChannel(t, node1, node2, msgToSend, testChannel)
}

func assertEventualEstablishStream(t *testing.T, rpc *cluster.RemoteContext) *cluster.Stream {
	var res *cluster.Stream
	gt := gomega.NewGomegaWithT(t)
	gt.Eventually(func() error {
		stream, err := rpc.NewStream(time.Hour)
		res = stream
		return err
	}, timeout).Should(gomega.Succeed())
	return res
}

func assertEventualSendMessage(t *testing.T, rpc *cluster.RemoteContext, req *orderer.SubmitRequest) *cluster.Stream {
	var res *cluster.Stream
	gt := gomega.NewGomegaWithT(t)
	gt.Eventually(func() error {
		stream, err := rpc.NewStream(time.Hour)
		if err != nil {
			return err
		}
		res = stream
		return stream.Send(wrapSubmitReq(req))
	}, timeout).Should(gomega.Succeed())
	return res
}

func wrapSubmitReq(req *orderer.SubmitRequest) *orderer.StepRequest {
	return &orderer.StepRequest{
		Payload: &orderer.StepRequest_SubmitRequest{
			SubmitRequest: req,
		},
	}
}
