/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

const (
	// MinimumExpirationWarningInterval is the default minimum time interval
	// between consecutive warnings about certificate expiration.
	MinimumExpirationWarningInterval = time.Minute * 5
)

var (
	errOverflow = errors.New("send queue overflown")
	errAborted  = errors.New("aborted")
	errTimeout  = errors.New("rpc timeout expired")
)

// MembersByChannel is a mapping from channel name
// to MemberMapping
type MembersByChannel map[string]MemberMapping

// Comm implements Communicator
type Comm struct {
	MinimumExpirationWarningInterval time.Duration
	CertExpWarningThreshold          time.Duration
	shutdownSignal                   chan struct{}
	shutdown                         bool
	SendBufferSize                   int
	Lock                             sync.RWMutex
	Logger                           *flogging.FabricLogger
	ChanExt                          ChannelExtractor
	H                                Handler
	Connections                      *ConnectionStore
	Chan2Members                     MembersByChannel
	Metrics                          *Metrics
	CompareCertificate               CertificateComparator
}

type requestContext struct {
	channel string
	sender  uint64
}

// DispatchSubmit identifies the channel and sender of the submit request and passes it
// to the underlying Handler
func (c *Comm) DispatchSubmit(ctx context.Context, request *orderer.SubmitRequest) error {
	reqCtx, err := c.requestContext(ctx, request)
	if err != nil {
		return err
	}
	return c.H.OnSubmit(reqCtx.channel, reqCtx.sender, request)
}

// DispatchConsensus identifies the channel and sender of the step request and passes it
// to the underlying Handler
func (c *Comm) DispatchConsensus(ctx context.Context, request *orderer.ConsensusRequest) error {
	reqCtx, err := c.requestContext(ctx, request)
	if err != nil {
		return err
	}
	return c.H.OnConsensus(reqCtx.channel, reqCtx.sender, request)
}

// requestContext identifies the sender and channel of the request and returns
// it wrapped in a requestContext
func (c *Comm) requestContext(ctx context.Context, msg proto.Message) (*requestContext, error) {
	channel := c.ChanExt.TargetChannel(msg)
	if channel == "" {
		return nil, errors.Errorf("badly formatted message, cannot extract channel")
	}

	c.Lock.RLock()
	mapping, exists := c.Chan2Members[channel]
	c.Lock.RUnlock()

	if !exists {
		return nil, errors.Errorf("channel %s doesn't exist", channel)
	}

	cert := util.ExtractRawCertificateFromContext(ctx)
	if len(cert) == 0 {
		return nil, errors.Errorf("no TLS certificate sent")
	}

	stub := mapping.LookupByClientCert(cert)
	if stub == nil {
		return nil, errors.Errorf("certificate extracted from TLS connection isn't authorized")
	}
	return &requestContext{
		channel: channel,
		sender:  stub.ID,
	}, nil
}

// Remote obtains a RemoteContext linked to the destination node on the context
// of a given channel
func (c *Comm) Remote(channel string, id uint64) (*RemoteContext, error) {
	c.Lock.RLock()
	defer c.Lock.RUnlock()

	if c.shutdown {
		return nil, errors.New("communication has been shut down")
	}

	mapping, exists := c.Chan2Members[channel]
	if !exists {
		return nil, errors.Errorf("channel %s doesn't exist", channel)
	}
	stub := mapping.ByID(id)
	if stub == nil {
		return nil, errors.Errorf("node %d doesn't exist in channel %s's membership", id, channel)
	}

	if stub.Active() {
		return stub.RemoteContext, nil
	}

	err := stub.Activate(c.createRemoteContext(stub, channel))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return stub.RemoteContext, nil
}

// Configure configures the channel with the given RemoteNodes
func (c *Comm) Configure(channel string, newNodes []RemoteNode) {
	c.Logger.Infof("Entering, channel: %s, nodes: %v", channel, newNodes)
	defer c.Logger.Infof("Exiting")

	c.Lock.Lock()
	defer c.Lock.Unlock()

	c.createShutdownSignalIfNeeded()

	if c.shutdown {
		return
	}

	beforeConfigChange := c.serverCertsInUse()
	// Update the channel-scoped mapping with the new nodes
	c.applyMembershipConfig(channel, newNodes)
	// Close connections to nodes that are not present in the new membership
	c.cleanUnusedConnections(beforeConfigChange)
}

func (c *Comm) createShutdownSignalIfNeeded() {
	if c.shutdownSignal == nil {
		c.shutdownSignal = make(chan struct{})
	}
}

// Shutdown shuts down the instance
func (c *Comm) Shutdown() {
	c.Lock.Lock()
	defer c.Lock.Unlock()

	c.createShutdownSignalIfNeeded()
	if !c.shutdown {
		close(c.shutdownSignal)
	}

	c.shutdown = true
	for _, members := range c.Chan2Members {
		members.Foreach(func(id uint64, stub *Stub) {
			c.Connections.Disconnect(stub.ServerTLSCert)
		})
	}
}

// cleanUnusedConnections disconnects all connections that are un-used
// at the moment of the invocation
func (c *Comm) cleanUnusedConnections(serverCertsBeforeConfig StringSet) {
	// Scan all nodes after the reconfiguration
	serverCertsAfterConfig := c.serverCertsInUse()
	// Filter out the certificates that remained after the reconfiguration
	serverCertsBeforeConfig.subtract(serverCertsAfterConfig)
	// Close the connections to all these nodes as they shouldn't be in use now
	for serverCertificate := range serverCertsBeforeConfig {
		c.Connections.Disconnect([]byte(serverCertificate))
	}
}

// serverCertsInUse returns the server certificates that are in use
// represented as strings.
func (c *Comm) serverCertsInUse() StringSet {
	endpointsInUse := make(StringSet)
	for _, mapping := range c.Chan2Members {
		endpointsInUse.union(mapping.ServerCertificates())
	}
	return endpointsInUse
}

// applyMembershipConfig sets the given RemoteNodes for the given channel
func (c *Comm) applyMembershipConfig(channel string, newNodes []RemoteNode) {
	mapping := c.getOrCreateMapping(channel)
	newNodeIDs := make(map[uint64]struct{})

	for _, node := range newNodes {
		newNodeIDs[node.ID] = struct{}{}
		c.updateStubInMapping(channel, mapping, node)
	}

	// Remove all stubs without a corresponding node
	// in the new nodes
	mapping.Foreach(func(id uint64, stub *Stub) {
		if _, exists := newNodeIDs[id]; exists {
			c.Logger.Info(id, "exists in both old and new membership for channel", channel, ", skipping its deactivation")
			return
		}
		c.Logger.Info("Deactivated node", id, "who's endpoint is", stub.Endpoint, "as it's removed from membership")
		mapping.Remove(id)
		stub.Deactivate()
	})
}

// updateStubInMapping updates the given RemoteNode and adds it to the MemberMapping
func (c *Comm) updateStubInMapping(channel string, mapping MemberMapping, node RemoteNode) {
	stub := mapping.ByID(node.ID)
	if stub == nil {
		c.Logger.Info("Allocating a new stub for node", node.ID, "with endpoint of", node.Endpoint, "for channel", channel)
		stub = &Stub{}
	}

	// Check if the TLS server certificate of the node is replaced
	// and if so - then deactivate the stub, to trigger
	// a re-creation of its gRPC connection
	if !bytes.Equal(stub.ServerTLSCert, node.ServerTLSCert) {
		c.Logger.Info("Deactivating node", node.ID, "in channel", channel,
			"with endpoint of", node.Endpoint, "due to TLS certificate change")
		stub.Deactivate()
	}

	// Overwrite the stub Node data with the new data
	stub.RemoteNode = node

	// Put the stub into the mapping
	mapping.Put(stub)

	// Check if the stub needs activation.
	if stub.Active() {
		return
	}

	// Activate the stub
	stub.Activate(c.createRemoteContext(stub, channel))
}

// createRemoteStub returns a function that creates a RemoteContext.
// It is used as a parameter to Stub.Activate() in order to activate
// a stub atomically.
func (c *Comm) createRemoteContext(stub *Stub, channel string) func() (*RemoteContext, error) {
	return func() (*RemoteContext, error) {
		cert, err := x509.ParseCertificate(stub.ServerTLSCert)
		if err != nil {
			pemString := string(pem.EncodeToMemory(&pem.Block{Bytes: stub.ServerTLSCert}))
			c.Logger.Errorf("Invalid DER for channel %s, endpoint %s, ID %d: %v", channel, stub.Endpoint, stub.ID, pemString)
			return nil, errors.Wrap(err, "invalid certificate DER")
		}

		c.Logger.Debug("Connecting to", stub.RemoteNode, "for channel", channel)

		conn, err := c.Connections.Connection(stub.Endpoint, stub.ServerTLSCert)
		if err != nil {
			c.Logger.Warningf("Unable to obtain connection to %d(%s) (channel %s): %v", stub.ID, stub.Endpoint, channel, err)
			return nil, err
		}

		probeConnection := func(conn *grpc.ClientConn) error {
			connState := conn.GetState()
			if connState == connectivity.Connecting {
				return errors.Errorf("connection to %d(%s) is in state %s", stub.ID, stub.Endpoint, connState)
			}
			return nil
		}

		clusterClient := orderer.NewClusterClient(conn)
		getStream := func(ctx context.Context) (StepClientStream, error) {
			stream, err := clusterClient.Step(ctx)
			if err != nil {
				return nil, err
			}
			stepClientStream := &CommClientStream{
				StepClient: stream,
			}
			return stepClientStream, nil
		}

		workerCountReporter := workerCountReporter{
			channel: channel,
		}

		rc := &RemoteContext{
			expiresAt:                        cert.NotAfter,
			minimumExpirationWarningInterval: c.MinimumExpirationWarningInterval,
			certExpWarningThreshold:          c.CertExpWarningThreshold,
			workerCountReporter:              workerCountReporter,
			Channel:                          channel,
			Metrics:                          c.Metrics,
			SendBuffSize:                     c.SendBufferSize,
			shutdownSignal:                   c.shutdownSignal,
			endpoint:                         stub.Endpoint,
			Logger:                           c.Logger,
			ProbeConn:                        probeConnection,
			conn:                             conn,
			GetStreamFunc:                    getStream,
		}
		return rc, nil
	}
}

// getOrCreateMapping creates a MemberMapping for the given channel
// or returns the existing one.
func (c *Comm) getOrCreateMapping(channel string) MemberMapping {
	// Lazily create a mapping if it doesn't already exist
	mapping, exists := c.Chan2Members[channel]
	if !exists {
		mapping = MemberMapping{
			id2stub:       make(map[uint64]*Stub),
			SamePublicKey: c.CompareCertificate,
		}
		c.Chan2Members[channel] = mapping
	}
	return mapping
}

func commonNameFromContext(ctx context.Context) string {
	cert := util.ExtractCertificateFromContext(ctx)
	if cert == nil {
		return "unidentified node"
	}
	return cert.Subject.CommonName
}

type streamsMapperReporter struct {
	size uint32
	sync.Map
}

func (smr *streamsMapperReporter) Delete(key interface{}) {
	smr.Map.Delete(key)
	atomic.AddUint32(&smr.size, ^uint32(0))
}

func (smr *streamsMapperReporter) Store(key, value interface{}) {
	smr.Map.Store(key, value)
	atomic.AddUint32(&smr.size, 1)
}

type workerCountReporter struct {
	channel     string
	workerCount uint32
}

func (wcr *workerCountReporter) increment(m *Metrics) {
	count := atomic.AddUint32(&wcr.workerCount, 1)
	m.reportWorkerCount(wcr.channel, count)
}

func (wcr *workerCountReporter) decrement(m *Metrics) {
	// ^0 flips all zeros to ones, which means
	// 2^32 - 1, and then we add this number wcr.workerCount.
	// It follows from commutativity of the unsigned integers group
	// that wcr.workerCount + 2^32 - 1 = wcr.workerCount - 1 + 2^32
	// which is just wcr.workerCount - 1.
	count := atomic.AddUint32(&wcr.workerCount, ^uint32(0))
	m.reportWorkerCount(wcr.channel, count)
}

type CommClientStream struct {
	StepClient orderer.Cluster_StepClient
}

func (cs *CommClientStream) Send(request *orderer.StepRequest) error {
	return cs.StepClient.Send(request)
}

func (cs *CommClientStream) Recv() (*orderer.StepResponse, error) {
	return cs.StepClient.Recv()
}

func (cs *CommClientStream) Auth() error {
	return nil
}

func (cs *CommClientStream) Context() context.Context {
	return cs.StepClient.Context()
}
