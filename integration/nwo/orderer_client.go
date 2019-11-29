/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"context"
	"io/ioutil"
	"path"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/pkg/errors"
)

// Broadcast sends given env to Broadcast API of specified orderer.
func Broadcast(n *Network, o *Orderer, env *common.Envelope) (*orderer.BroadcastResponse, error) {
	gRPCclient, err := createOrdererGRPCClient(n, o)
	if err != nil {
		return nil, err
	}

	addr := n.OrdererAddress(o, ListenPort)
	conn, err := gRPCclient.NewConnection(addr)
	if err != nil {
		return nil, err
	}
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
func Deliver(n *Network, o *Orderer, env *common.Envelope) (*common.Block, error) {
	gRPCclient, err := createOrdererGRPCClient(n, o)
	if err != nil {
		return nil, err
	}

	addr := n.OrdererAddress(o, ListenPort)
	conn, err := gRPCclient.NewConnection(addr)
	if err != nil {
		return nil, err
	}
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

func createOrdererGRPCClient(n *Network, o *Orderer) (*comm.GRPCClient, error) {
	config := comm.ClientConfig{}
	config.Timeout = 5 * time.Second

	secOpts := comm.SecureOptions{
		UseTLS:            true,
		RequireClientCert: false,
	}

	caPEM, err := ioutil.ReadFile(path.Join(n.OrdererLocalTLSDir(o), "ca.crt"))
	if err != nil {
		return nil, err
	}

	secOpts.ServerRootCAs = [][]byte{caPEM}
	config.SecOpts = secOpts

	grpcClient, err := comm.NewGRPCClient(config)
	if err != nil {
		return nil, err
	}

	return grpcClient, nil
}
