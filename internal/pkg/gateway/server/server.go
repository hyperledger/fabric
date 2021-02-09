/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"fmt"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/internal/pkg/gateway"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit"
	"github.com/hyperledger/fabric/internal/pkg/gateway/registry"
)

var logger = flogging.MustGetLogger("gateway")

// Server represents the GRPC server for the Gateway
type Server struct {
	registry       gateway.Registry
	commitNotifier *commit.Notifier
}

// CreateGatewayServer creates an embedded instance of the Gateway
func CreateGatewayServer(localEndorser gateway.Endorser, peer *peer.Peer) (*Server, error) {
	registry, err := registry.New(localEndorser)
	if err != nil {
		return nil, err
	}

	commitNotifier := commit.NewNotifier(&peerChannelFactory{peer})

	gwServer := &Server{
		registry:       registry,
		commitNotifier: commitNotifier,
	}

	return gwServer, nil
}

type peerChannelFactory struct {
	peer *peer.Peer
}

func (factory *peerChannelFactory) Channel(channelID string) (commit.Channel, error) {
	if channel := factory.peer.Channel(channelID); channel != nil {
		return channel, nil
	}

	return nil, fmt.Errorf("unknown channel: %s", channelID)
}
