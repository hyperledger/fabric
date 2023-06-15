/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"google.golang.org/grpc"
)

type DialerAdapter struct {
	ClientConfig comm.ClientConfig
}

func (da DialerAdapter) Dial(address string, rootCerts [][]byte) (*grpc.ClientConn, error) {
	cc := da.ClientConfig
	cc.SecOpts.ServerRootCAs = rootCerts
	return cc.Dial(address)
}

type DeliverAdapter struct{}

func (DeliverAdapter) Deliver(ctx context.Context, clientConn *grpc.ClientConn) (orderer.AtomicBroadcast_DeliverClient, error) {
	return orderer.NewAtomicBroadcastClient(clientConn).Deliver(ctx)
}

type GossipServiceSender struct {
	gs                  GossipServiceAdapter
	blockGossipDisabled bool
	yieldLeadership     bool
}

// AddPayload adds payload to the local state sync buffer
func (gss *GossipServiceSender) AddPayload(chainID string, payload *gossip.Payload) error {
	return gss.gs.AddPayload(chainID, payload)
}

// Gossip the message across the peers
func (gss *GossipServiceSender) Gossip(msg *gossip.GossipMessage) {
	gss.gs.Gossip(msg)
}

func (gss *GossipServiceSender) BlockGossipDisabled() bool {
	return gss.blockGossipDisabled
}

func (gss *GossipServiceSender) YieldLeadership() bool {
	return gss.yieldLeadership
}
