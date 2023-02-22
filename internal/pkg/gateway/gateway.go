/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"

	peerproto "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	gdiscovery "github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit"
	"github.com/hyperledger/fabric/internal/pkg/gateway/config"
	"github.com/hyperledger/fabric/internal/pkg/gateway/ledger"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	"google.golang.org/grpc"
)

var logger = flogging.MustGetLogger("gateway")

// Server represents the GRPC server for the Gateway.
type Server struct {
	registry         *registry
	commitFinder     CommitFinder
	policy           ACLChecker
	options          config.Options
	logger           *flogging.FabricLogger
	ledgerProvider   ledger.Provider
	getChannelConfig channelConfigGetter
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

type channelConfigGetter func(cid string) channelconfig.Resources

// CreateServer creates an embedded instance of the Gateway.
func CreateServer(
	localEndorser peerproto.EndorserServer,
	discovery Discovery,
	peerInstance *peer.Peer,
	secureOptions *comm.SecureOptions,
	policy ACLChecker,
	localMSPID string,
	options config.Options,
	systemChaincodes scc.BuiltinSCCs,
) *Server {
	adapter := &ledger.PeerAdapter{
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
		peerInstance.GossipService.SelfMembershipInfo(),
		localMSPID,
		secureOptions,
		options,
		systemChaincodes,
		peerInstance.OrdererEndpointOverrides,
		peerInstance.GetChannelConfig,
	)

	peerInstance.AddConfigCallbacks(server.registry.configUpdate)

	return server
}

func newServer(localEndorser peerproto.EndorserClient,
	discovery Discovery,
	finder CommitFinder,
	policy ACLChecker,
	ledgerProvider ledger.Provider,
	localInfo gdiscovery.NetworkMember,
	localMSPID string,
	secureOptions *comm.SecureOptions,
	options config.Options,
	systemChaincodes scc.BuiltinSCCs,
	ordererEndpointOverrides map[string]*orderers.Endpoint,
	getChannelConfig channelConfigGetter,
) *Server {
	return &Server{
		registry: &registry{
			localEndorser: &endorser{
				client:         localEndorser,
				endpointConfig: &endpointConfig{pkiid: localInfo.PKIid, address: localInfo.Endpoint, logAddress: localInfo.Endpoint, mspid: localMSPID},
			},
			discovery: discovery,
			logger:    logger,
			endpointFactory: &endpointFactory{
				timeout:                  options.DialTimeout,
				clientCert:               secureOptions.Certificate,
				clientKey:                secureOptions.Key,
				ordererEndpointOverrides: ordererEndpointOverrides,
			},
			remoteEndorsers:    map[string]*endorser{},
			channelInitialized: map[string]bool{},
			systemChaincodes:   systemChaincodes,
			localProvider:      ledgerProvider,
		},
		commitFinder:     finder,
		policy:           policy,
		options:          options,
		logger:           logger,
		ledgerProvider:   ledgerProvider,
		getChannelConfig: getChannelConfig,
	}
}
