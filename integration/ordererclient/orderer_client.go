/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordererclient

import (
	"context"
	"reflect"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
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

	if err = broadcaster.Send(env); err != nil {
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

	if err = deliverer.Send(env); err != nil {
		return nil, err
	}

	resp, err := deliverer.Recv()
	if err != nil {
		return nil, err
	}

	switch t := resp.Type.(type) {
	case *orderer.DeliverResponse_Block:
		blk := resp.GetBlock()
		if blk == nil {
			return nil, errors.Errorf("block not found")
		}

		return blk, nil
	case *orderer.DeliverResponse_Status:
		return nil, errors.Errorf("faulty node, received status: %s", common.Status_name[int32(t.Status)])
	default:
		return nil, errors.Errorf("response is of type %v, but expected a block", reflect.TypeOf(resp.Type))
	}
}
