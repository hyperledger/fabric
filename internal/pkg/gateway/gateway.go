/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"

	peerproto "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit"
	"github.com/hyperledger/fabric/internal/pkg/gateway/config"
	"google.golang.org/grpc"
)

var logger = flogging.MustGetLogger("gateway")

// Server represents the GRPC server for the Gateway.
type Server struct {
	registry     *registry
	commitFinder CommitFinder
	eventer      Eventer
	policy       ACLChecker
	options      config.Options
}

type EndorserServerAdapter struct {
	Server peerproto.EndorserServer
}

func (e *EndorserServerAdapter) ProcessProposal(ctx context.Context, req *peerproto.SignedProposal, _ ...grpc.CallOption) (*peerproto.ProposalResponse, error) {
	return e.Server.ProcessProposal(ctx, req)
}

type CommitFinder interface {
	TransactionStatus(ctx context.Context, channelName string, transactionID string) (*commit.Status, error)
}

type Eventer interface {
	ChaincodeEvents(ctx context.Context, channelName string, chaincodeName string) (<-chan *commit.BlockChaincodeEvents, error)
}

type ACLChecker interface {
	CheckACL(policyName string, channelName string, data interface{}) error
}

// CreateServer creates an embedded instance of the Gateway.
func CreateServer(localEndorser peerproto.EndorserServer, discovery Discovery, peerInstance *peer.Peer, policy ACLChecker, localMSPID string, options config.Options) *Server {
	adapter := &peerAdapter{
		Peer: peerInstance,
	}
	notifier := commit.NewNotifier(adapter)

	return newServer(
		&EndorserServerAdapter{
			Server: localEndorser,
		},
		discovery,
		commit.NewFinder(adapter, notifier),
		commit.NewEventer(notifier),
		policy,
		peerInstance.GossipService.SelfMembershipInfo().Endpoint,
		localMSPID,
		options,
	)
}

func newServer(localEndorser peerproto.EndorserClient, discovery Discovery, finder CommitFinder, eventer Eventer, policy ACLChecker, localEndpoint, localMSPID string, options config.Options) *Server {
	gwServer := &Server{
		registry: &registry{
			localEndorser:       &endorser{client: localEndorser, endpointConfig: &endpointConfig{address: localEndpoint, mspid: localMSPID}},
			discovery:           discovery,
			logger:              logger,
			endpointFactory:     &endpointFactory{timeout: options.EndorsementTimeout},
			remoteEndorsers:     map[string]*endorser{},
			broadcastClients:    map[string]*orderer{},
			tlsRootCerts:        map[string][][]byte{},
			channelsInitialized: map[string]bool{},
		},
		commitFinder: finder,
		eventer:      eventer,
		policy:       policy,
		options:      options,
	}

	return gwServer
}
