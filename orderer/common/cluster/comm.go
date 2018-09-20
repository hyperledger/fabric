/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
)

const (
	// DefaultRPCTimeout is the default RPC timeout
	// that RPCs use
	DefaultRPCTimeout = time.Second * 5
)

// ChannelExtractor extracts the channel of a given message,
// or returns an empty string if that's not possible
type ChannelExtractor interface {
	TargetChannel(message proto.Message) string
}

//go:generate mockery -dir . -name Handler -case underscore -output ./mocks/

// Handler handles Step() and Submit() requests and returns a corresponding response
type Handler interface {
	OnStep(channel string, sender uint64, req *orderer.StepRequest) (*orderer.StepResponse, error)
	OnSubmit(channel string, sender uint64, req *orderer.SubmitRequest) (*orderer.SubmitResponse, error)
}

// RemoteNode represents a cluster member
type RemoteNode struct {
	// ID is unique among all members, and cannot be 0.
	ID uint64
	// Endpoint is the endpoint of the node, denoted in %s:%d format
	Endpoint string
	// ServerTLSCert is the DER encoded TLS server certificate of the node
	ServerTLSCert []byte
	// ClientTLSCert is the DER encoded TLS client certificate of the node
	ClientTLSCert []byte
}

// String returns a string representation of this RemoteNode
func (rm RemoteNode) String() string {
	return fmt.Sprintf("ID: %d\nEndpoint: %s\nServerTLSCert:%s ClientTLSCert:%s",
		rm.ID, rm.Endpoint, DERtoPEM(rm.ServerTLSCert), DERtoPEM(rm.ClientTLSCert))
}

// Communicator defines communication for a consenter
type Communicator interface {
	// Remote returns a RemoteContext for the given RemoteNode ID in the context
	// of the given channel, or error if connection cannot be established, or
	// the channel wasn't configured
	Remote(channel string, id uint64) (*RemoteContext, error)
	// Configure configures the communication to connect to all
	// given members, and disconnect from any members not among the given
	// members.
	Configure(channel string, members []*RemoteNode)
	// Shutdown shuts down the communicator
	Shutdown()
}

// MembersByChannel is a mapping from channel name
// to MemberMapping
type MembersByChannel map[string]MemberMapping

// Comm implements Communicator
type Comm struct {
	shutdown     bool
	Lock         sync.RWMutex
	Logger       *logging.Logger
	ChanExt      ChannelExtractor
	H            Handler
	Connections  *ConnectionStore
	Chan2Members MembersByChannel
	RPCTimeout   time.Duration
}

type requestContext struct {
	channel string
	sender  uint64
}

// DispatchSubmit identifies the channel and sender of the submit request and passes it
// to the underlying Handler
func (c *Comm) DispatchSubmit(ctx context.Context, request *orderer.SubmitRequest) (*orderer.SubmitResponse, error) {
	c.Logger.Debug(request.Channel)
	reqCtx, err := c.requestContext(ctx, request)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return c.H.OnSubmit(reqCtx.channel, reqCtx.sender, request)
}

// DispatchStep identifies the channel and sender of the step request and passes it
// to the underlying Handler
func (c *Comm) DispatchStep(ctx context.Context, request *orderer.StepRequest) (*orderer.StepResponse, error) {
	reqCtx, err := c.requestContext(ctx, request)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return c.H.OnStep(reqCtx.channel, reqCtx.sender, request)
}

// classifyRequest identifies the sender and channel of the request and returns
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

	cert := comm.ExtractCertificateFromContext(ctx)
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

	err := stub.Activate(c.createRemoteContext(stub))
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

	if c.shutdown {
		return
	}

	beforeConfigChange := c.serverCertsInUse()
	// Update the channel-scoped mapping with the new nodes
	c.applyMembershipConfig(channel, newNodes)
	// Close connections to nodes that are not present in the new membership
	c.cleanUnusedConnections(beforeConfigChange)
}

// Shutdown shuts down the instance
func (c *Comm) Shutdown() {
	c.Lock.Lock()
	defer c.Lock.Unlock()

	c.shutdown = true
	for _, members := range c.Chan2Members {
		for _, member := range members {
			c.Connections.Disconnect(member.ServerTLSCert)
		}
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
		c.updateStubInMapping(mapping, node)
	}

	// Remove all stubs without a corresponding node
	// in the new nodes
	for id, stub := range mapping {
		if _, exists := newNodeIDs[id]; exists {
			c.Logger.Info(id, "exists in both old and new membership, skipping its deactivation")
			continue
		}
		c.Logger.Info("Deactivated node", id, "who's endpoint is", stub.Endpoint, "as it's removed from membership")
		delete(mapping, id)
		stub.Deactivate()
	}
}

// updateStubInMapping updates the given RemoteNode and adds it to the MemberMapping
func (c *Comm) updateStubInMapping(mapping MemberMapping, node RemoteNode) {
	stub := mapping.ByID(node.ID)
	if stub == nil {
		c.Logger.Info("Allocating a new stub for node", node.ID, "with endpoint of", node.Endpoint)
		stub = &Stub{}
	}

	// Check if the TLS server certificate of the node is replaced
	// and if so - then deactivate the stub, to trigger
	// a re-creation of its gRPC connection
	if !bytes.Equal(stub.ServerTLSCert, node.ServerTLSCert) {
		c.Logger.Info("Deactivating node", node.ID, "with endpoint of", node.Endpoint, "due to TLS certificate change")
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
	stub.Activate(c.createRemoteContext(stub))
}

// createRemoteStub returns a function that creates a RemoteContext.
// It is used as a parameter to Stub.Activate() in order to activate
// a stub atomically.
func (c *Comm) createRemoteContext(stub *Stub) func() (*RemoteContext, error) {
	return func() (*RemoteContext, error) {
		timeout := c.RPCTimeout
		if timeout == time.Duration(0) {
			timeout = DefaultRPCTimeout
		}

		c.Logger.Debug("Connecting to", stub.RemoteNode, "with gRPC timeout of", timeout)

		conn, err := c.Connections.Connection(stub.Endpoint, stub.ServerTLSCert)
		if err != nil {
			c.Logger.Warningf("Unable to obtain connection to %d(%s): %v", stub.ID, stub.Endpoint, err)
			return nil, err
		}

		clusterClient := orderer.NewClusterClient(conn)

		rc := &RemoteContext{
			RPCTimeout: timeout,
			Client:     clusterClient,
			onAbort: func() {
				c.Logger.Info("Aborted connection to", stub.ID, stub.Endpoint)
				stub.RemoteContext = nil
			},
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
		mapping = make(MemberMapping)
		c.Chan2Members[channel] = mapping
	}
	return mapping
}

// Stub holds all information about the remote node,
// including the RemoteContext for it, and serializes
// some operations on it.
type Stub struct {
	lock sync.RWMutex
	RemoteNode
	*RemoteContext
}

// Active returns whether the Stub
// is active or not
func (stub *Stub) Active() bool {
	stub.lock.RLock()
	defer stub.lock.RUnlock()
	return stub.isActive()
}

// Active returns whether the Stub
// is active or not.
func (stub *Stub) isActive() bool {
	return stub.RemoteContext != nil
}

// Deactivate deactivates the Stub and
// ceases all communication operations
// invoked on it.
func (stub *Stub) Deactivate() {
	stub.lock.Lock()
	defer stub.lock.Unlock()
	if !stub.isActive() {
		return
	}
	stub.RemoteContext.Abort()
	stub.RemoteContext = nil
}

// Activate creates a remote context with the given function callback
// in an atomic manner - if two parallel invocations are invoked on this Stub,
// only a single invocation of createRemoteStub takes place.
func (stub *Stub) Activate(createRemoteContext func() (*RemoteContext, error)) error {
	stub.lock.Lock()
	defer stub.lock.Unlock()
	// Check if the stub has already been activated while we were waiting for the lock
	if stub.isActive() {
		return nil
	}
	remoteStub, err := createRemoteContext()
	if err != nil {
		return errors.WithStack(err)
	}

	stub.RemoteContext = remoteStub
	return nil
}

// RemoteContext interacts with remote cluster
// nodes. Every call can be aborted via call to Abort()
type RemoteContext struct {
	RPCTimeout         time.Duration
	onAbort            func()
	Client             orderer.ClusterClient
	stepLock           sync.Mutex
	cancelStep         func()
	submitLock         sync.Mutex
	cancelSubmitStream func()
	submitStream       orderer.Cluster_SubmitClient
}

// SubmitStream creates a new Submit stream
func (rc *RemoteContext) SubmitStream() (orderer.Cluster_SubmitClient, error) {
	rc.submitLock.Lock()
	defer rc.submitLock.Unlock()
	// Close previous submit stream to prevent resource leak
	rc.closeSubmitStream()

	ctx, cancel := context.WithCancel(context.TODO())
	submitStream, err := rc.Client.Submit(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rc.submitStream = submitStream
	rc.cancelSubmitStream = cancel
	return rc.submitStream, nil
}

// Step passes an implementation-specific message to another cluster member.
func (rc *RemoteContext) Step(req *orderer.StepRequest) (*orderer.StepResponse, error) {
	ctx, abort := context.WithCancel(context.TODO())
	ctx, cancel := context.WithTimeout(ctx, rc.RPCTimeout)
	defer cancel()

	rc.stepLock.Lock()
	rc.cancelStep = abort
	rc.stepLock.Unlock()

	return rc.Client.Step(ctx, req)
}

// Abort aborts the contexts the RemoteContext uses,
// thus effectively causes all operations on the embedded
// ClusterClient to end.
func (rc *RemoteContext) Abort() {
	rc.stepLock.Lock()
	defer rc.stepLock.Unlock()

	rc.submitLock.Lock()
	defer rc.submitLock.Unlock()

	if rc.cancelStep != nil {
		rc.cancelStep()
		rc.cancelStep = nil
	}

	rc.closeSubmitStream()
	rc.onAbort()
}

// closeSubmitStream closes the Submit stream
// and invokes its cancellation function
func (rc *RemoteContext) closeSubmitStream() {
	if rc.cancelSubmitStream != nil {
		rc.cancelSubmitStream()
		rc.cancelSubmitStream = nil
	}

	if rc.submitStream != nil {
		rc.submitStream.CloseSend()
		rc.submitStream = nil
	}
}
