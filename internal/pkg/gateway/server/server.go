/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"fmt"

	ordererProto "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
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

	commitNotifier := commit.NewNotifier(&peerAdapter{peer})

	gwServer := &Server{
		registry:       registry,
		commitNotifier: commitNotifier,
	}

	return gwServer, nil
}

type peerAdapter struct {
	peer *peer.Peer
}

func (pa *peerAdapter) Iterator(channelID string, startType *ordererProto.SeekPosition) (blockledger.Iterator, error) {
	channel := pa.peer.Channel(channelID)
	if channel == nil {
		return nil, fmt.Errorf("unknown channel: %s", channelID)
	}

	blockIterator, _ := channel.Reader().Iterator(startType)

	return blockIterator, nil
}
