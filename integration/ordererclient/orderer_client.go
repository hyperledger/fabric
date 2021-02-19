/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordererclient

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/pkg/errors"
)

// Broadcast sends given env to Broadcast API of specified orderer.
func Broadcast(n *nwo.Network, o *nwo.Orderer, env *common.Envelope) (*orderer.BroadcastResponse, error) {
	conn := n.OrdererClientConn(o)
	defer conn.Close()

	broadcaster, err := orderer.NewAtomicBroadcastClient(conn).Broadcast(context.Background())
	if err != nil {
		return nil, err
	}

	err = broadcaster.Send(env)
	if err != nil {
		return nil, err
	}

	resp, err := broadcaster.Recv()
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// Deliver sends given env to Deliver API of specified orderer.
func Deliver(n *nwo.Network, o *nwo.Orderer, env *common.Envelope) (*common.Block, error) {
	conn := n.OrdererClientConn(o)
	defer conn.Close()

	deliverer, err := orderer.NewAtomicBroadcastClient(conn).Deliver(context.Background())
	if err != nil {
		return nil, err
	}

	err = deliverer.Send(env)
	if err != nil {
		return nil, err
	}

	resp, err := deliverer.Recv()
	if err != nil {
		return nil, err
	}

	blk := resp.GetBlock()
	if blk == nil {
		return nil, errors.Errorf("block not found")
	}

	return blk, nil
}
