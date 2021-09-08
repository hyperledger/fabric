/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"

	peerproto "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit"
	"github.com/hyperledger/fabric/internal/pkg/gateway/config"
	"google.golang.org/grpc"
)

var logger = flogging.MustGetLogger("gateway")

// Server represents the GRPC server for the Gateway.
type Server struct {
	registry       *registry
	commitFinder   CommitFinder
	policy         ACLChecker
	options        config.Options
	logger         *flogging.FabricLogger
	ledgerProvider LedgerProvider
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

type ACLChecker interface {
	CheckACL(policyName string, channelName string, data interface{}) error
}

type LedgerProvider interface {
	Ledger(channelName string) (ledger.Ledger, error)
}

// CreateServer creates an embedded instance of the Gateway.
func CreateServer(localEndorser peerproto.EndorserServer, discovery Discovery, peerInstance *peer.Peer, policy ACLChecker, localMSPID string, options config.Options) *Server {
	adapter := &peerAdapter{
		Peer: peerInstance,
	}
	notifier := commit.NewNotifier(adapter)

	server := newServer(
		&EndorserServerAdapter{
			Server: localEndorser,
		},
		discovery,
		commit.NewFinder(adapter, notifier),
		policy,
		adapter,
		peerInstance.GossipService.SelfMembershipInfo().PKIid,
		peerInstance.GossipService.SelfMembershipInfo().Endpoint,
		localMSPID,
		options,
	)

	peerInstance.AddConfigCallbacks(server.registry.configUpdate)

	return server
}

func newServer(localEndorser peerproto.EndorserClient, discovery Discovery, finder CommitFinder, policy ACLChecker, ledgerProvider LedgerProvider, localPKIID common.PKIidType, localEndpoint, localMSPID string, options config.Options) *Server {
	return &Server{
		registry: &registry{
			localEndorser:      &endorser{client: localEndorser, endpointConfig: &endpointConfig{pkiid: localPKIID, address: localEndpoint, mspid: localMSPID}},
			discovery:          discovery,
			logger:             logger,
			endpointFactory:    &endpointFactory{timeout: options.EndorsementTimeout},
			remoteEndorsers:    map[string]*endorser{},
			channelInitialized: map[string]bool{},
		},
		commitFinder:   finder,
		policy:         policy,
		options:        options,
		logger:         logger,
		ledgerProvider: ledgerProvider,
	}
}
