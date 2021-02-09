/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"

	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"google.golang.org/grpc"
)

var logger = flogging.MustGetLogger("gateway")

// Server represents the GRPC server for the Gateway.
type Server struct {
	registry *registry
}

type EndorserServerAdapter struct {
	Server peer.EndorserServer
}

func (e *EndorserServerAdapter) ProcessProposal(ctx context.Context, req *peer.SignedProposal, _ ...grpc.CallOption) (*peer.ProposalResponse, error) {
	return e.Server.ProcessProposal(ctx, req)
}

// CreateServer creates an embedded instance of the Gateway.
func CreateServer(localEndorser peer.EndorserClient, discovery Discovery, selfEndpoint string) *Server {
	gwServer := &Server{
		registry: &registry{
			localEndorser:       localEndorser,
			discovery:           discovery,
			selfEndpoint:        selfEndpoint,
			logger:              logger,
			endorserFactory:     newEndorser,
			ordererFactory:      newOrderer,
			remoteEndorsers:     map[string]peer.EndorserClient{},
			broadcastClients:    map[string]ab.AtomicBroadcast_BroadcastClient{},
			tlsRootCerts:        map[string][][]byte{},
			channelsInitialized: map[string]bool{},
		},
	}

	return gwServer
}
