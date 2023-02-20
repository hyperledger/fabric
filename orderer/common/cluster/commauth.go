/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"context"
	"encoding/asn1"
	"strconv"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// AuthCommMgr implements the Communicator
// It manages the client side connections and streams established with
// the Cluster GRPC server and new Cluster service
type AuthCommMgr struct {
	Logger  *flogging.FabricLogger
	Metrics *Metrics

	Lock           sync.RWMutex
	shutdown       bool
	shutdownSignal chan struct{}

	Chan2Members MembersByChannel
	Connections  *ConnectionsMgr

	SendBufferSize int
	NodeIdentity   []byte
	Signer         identity.Signer
}

func (ac *AuthCommMgr) Remote(channel string, id uint64) (*RemoteContext, error) {
	ac.Lock.RLock()
	defer ac.Lock.RUnlock()

	if ac.shutdown {
		return nil, errors.New("communication has been shut down")
	}

	mapping, exists := ac.Chan2Members[channel]
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

	err := stub.Activate(ac.createRemoteContext(stub, channel))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return stub.RemoteContext, nil
}

func (ac *AuthCommMgr) Configure(channel string, members []RemoteNode) {
	ac.Logger.Infof("Configuring communication module for Channel: %s with nodes:%v", channel, members)

	ac.Lock.Lock()
	defer ac.Lock.Unlock()

	if ac.shutdown {
		return
	}

	if ac.shutdownSignal == nil {
		ac.shutdownSignal = make(chan struct{})
	}

	mapping := ac.getOrCreateMapping(channel)
	newNodeIDs := make(map[uint64]struct{})

	for _, node := range members {
		newNodeIDs[node.ID] = struct{}{}
		ac.updateStubInMapping(channel, mapping, node)
	}

	// Remove all stubs without a corresponding node
	// in the new nodes
	mapping.Foreach(func(id uint64, stub *Stub) {
		if _, exists := newNodeIDs[id]; exists {
			ac.Logger.Infof("Node with ID %v exists in new membership of channel %v", id, channel)
			return
		}
		ac.Logger.Infof("Deactivated node %v who's endpoint is %v", id, stub.Endpoint)
		mapping.Remove(id)
		stub.Deactivate()
		ac.Connections.Disconnect(stub.Endpoint)
	})
}

func (ac *AuthCommMgr) Shutdown() {
	ac.Lock.Lock()
	defer ac.Lock.Unlock()

	if !ac.shutdown && ac.shutdownSignal != nil {
		close(ac.shutdownSignal)
	}

	ac.shutdown = true
	for _, members := range ac.Chan2Members {
		members.Foreach(func(id uint64, stub *Stub) {
			ac.Connections.Disconnect(stub.endpoint)
		})
	}
}

// getOrCreateMapping creates a MemberMapping for the given channel
// or returns the existing one.
func (ac *AuthCommMgr) getOrCreateMapping(channel string) MemberMapping {
	// Lazily create a mapping if it doesn't already exist
	mapping, exists := ac.Chan2Members[channel]
	if !exists {
		mapping = MemberMapping{
			id2stub: make(map[uint64]*Stub),
		}
		ac.Chan2Members[channel] = mapping
	}
	return mapping
}

// updateStubInMapping updates the given RemoteNode and adds it to the MemberMapping
func (ac *AuthCommMgr) updateStubInMapping(channel string, mapping MemberMapping, node RemoteNode) {
	stub := mapping.ByID(node.ID)
	if stub == nil {
		ac.Logger.Infof("Allocating a new stub for node %v with endpoint %v for channel %s", node.ID, node.Endpoint, channel)
		stub = &Stub{}
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
	stub.Activate(ac.createRemoteContext(stub, channel))
}

func (ac *AuthCommMgr) createRemoteContext(stub *Stub, channel string) func() (*RemoteContext, error) {
	return func() (*RemoteContext, error) {
		ac.Logger.Debugf("Connecting to node: %v for channel: %v", stub.RemoteNode.NodeAddress, channel)

		conn, err := ac.Connections.Connect(stub.Endpoint, stub.RemoteNode.ServerRootCA)
		if err != nil {
			ac.Logger.Warningf("Unable to obtain connection to %d(%s) (channel %s): %v", stub.ID, stub.Endpoint, channel, err)
			return nil, err
		}

		probeConnection := func(conn *grpc.ClientConn) error {
			connState := conn.GetState()
			if connState == connectivity.Connecting {
				return errors.Errorf("connection to %d(%s) is in state %s", stub.ID, stub.Endpoint, connState)
			}
			return nil
		}

		clusterClient := orderer.NewClusterNodeServiceClient(conn)
		getStepClientStream := func(ctx context.Context) (StepClientStream, error) {
			stream, err := clusterClient.Step(ctx)
			if err != nil {
				return nil, err
			}

			membersMapping, exists := ac.Chan2Members[channel]
			if !exists {
				return nil, errors.Errorf("channel members not initialized")
			}
			nodeStub := membersMapping.LookupByIdentity(ac.NodeIdentity)
			if nodeStub == nil {
				return nil, errors.Errorf("node identity is missing in channel")
			}

			stepClientStream := &NodeClientStream{
				Version:           0,
				StepClient:        stream,
				SourceNodeID:      nodeStub.ID,
				DestinationNodeID: stub.ID,
				Signer:            ac.Signer,
				Channel:           channel,
			}
			return stepClientStream, nil
		}

		workerCountReporter := workerCountReporter{
			channel: channel,
		}

		rc := &RemoteContext{
			Metrics:             ac.Metrics,
			workerCountReporter: workerCountReporter,
			Channel:             channel,
			SendBuffSize:        ac.SendBufferSize,
			endpoint:            stub.Endpoint,
			Logger:              ac.Logger,
			ProbeConn:           probeConnection,
			conn:                conn,
			GetStreamFunc:       getStepClientStream,
			shutdownSignal:      ac.shutdownSignal,
		}
		return rc, nil
	}
}

type NodeClientStream struct {
	StepClient        orderer.ClusterNodeService_StepClient
	Version           uint32
	SourceNodeID      uint64
	DestinationNodeID uint64
	Signer            identity.Signer
	Channel           string
}

func (cs *NodeClientStream) Send(request *orderer.StepRequest) error {
	stepRequest, cerr := BuildStepRequest(request)
	if cerr != nil {
		return cerr
	}
	return cs.StepClient.Send(stepRequest)
}

func (cs *NodeClientStream) Recv() (*orderer.StepResponse, error) {
	nodeResponse, err := cs.StepClient.Recv()
	if err != nil {
		return nil, err
	}
	return BuildStepRespone(nodeResponse)
}

func (cs *NodeClientStream) Auth() error {
	if cs.Signer == nil {
		return errors.New("signer is nil")
	}

	timestamp, err := ptypes.TimestampProto(time.Now().UTC())
	if err != nil {
		return errors.Wrap(err, "failed to read timestamp")
	}

	payload := &orderer.NodeAuthRequest{
		Version:   cs.Version,
		Timestamp: timestamp,
		FromId:    cs.SourceNodeID,
		ToId:      cs.DestinationNodeID,
		Channel:   cs.Channel,
	}

	bindingFieldsHash := GetSessionBindingHash(payload)

	tlsBinding, err := GetTLSSessionBinding(cs.StepClient.Context(), bindingFieldsHash)
	if err != nil {
		return errors.Wrap(err, "TLSBinding failed")
	}
	payload.SessionBinding = tlsBinding

	asnSignFields, _ := asn1.Marshal(AuthRequestSignature{
		Version:        int64(payload.Version),
		Timestamp:      payload.Timestamp.String(),
		FromId:         strconv.FormatUint(payload.FromId, 10),
		ToId:           strconv.FormatUint(payload.ToId, 10),
		SessionBinding: payload.SessionBinding,
		Channel:        payload.Channel,
	})
	sig, err := cs.Signer.Sign(asnSignFields)
	if err != nil {
		return errors.Wrap(err, "signing failed")
	}

	payload.Signature = sig
	stepRequest := &orderer.ClusterNodeServiceStepRequest{
		Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
			NodeAuthrequest: payload,
		},
	}

	return cs.StepClient.Send(stepRequest)
}

func (cs *NodeClientStream) Context() context.Context {
	return cs.StepClient.Context()
}

func BuildStepRequest(request *orderer.StepRequest) (*orderer.ClusterNodeServiceStepRequest, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	var stepRequest *orderer.ClusterNodeServiceStepRequest
	if consReq := request.GetConsensusRequest(); consReq != nil {
		stepRequest = &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeConrequest{
				NodeConrequest: &orderer.NodeConsensusRequest{
					Payload:  consReq.Payload,
					Metadata: consReq.Metadata,
				},
			},
		}
		return stepRequest, nil
	} else if subReq := request.GetSubmitRequest(); subReq != nil {
		stepRequest = &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeTranrequest{
				NodeTranrequest: &orderer.NodeTransactionOrderRequest{
					Payload:           subReq.Payload,
					LastValidationSeq: subReq.LastValidationSeq,
				},
			},
		}
		return stepRequest, nil
	}
	return nil, errors.New("service message type not valid")
}

func BuildStepRespone(stepResponse *orderer.ClusterNodeServiceStepResponse) (*orderer.StepResponse, error) {
	if stepResponse == nil {
		return nil, errors.New("input response object is nil")
	}
	if respPayload := stepResponse.GetTranorderRes(); respPayload != nil {
		stepResponse := &orderer.StepResponse{
			Payload: &orderer.StepResponse_SubmitRes{
				SubmitRes: &orderer.SubmitResponse{
					Channel: respPayload.Channel,
					Status:  respPayload.Status,
				},
			},
		}
		return stepResponse, nil
	}
	return nil, errors.New("service stream returned with invalid response type")
}
