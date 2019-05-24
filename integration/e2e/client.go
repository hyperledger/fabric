/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"context"
	"io/ioutil"
	"path"
	"time"

	"github.com/hyperledger/fabric/cmd/common/signer"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

// Broadcast sends given env to Broadcast API of specified orderer.
func Broadcast(n *nwo.Network, o *nwo.Orderer, env *common.Envelope) (*orderer.BroadcastResponse, error) {
	gRPCclient, err := CreateGRPCClient(n, o)
	if err != nil {
		return nil, err
	}

	addr := n.OrdererAddress(o, nwo.ListenPort)
	conn, err := gRPCclient.NewConnection(addr, "")
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
func Deliver(n *nwo.Network, o *nwo.Orderer, env *common.Envelope) (*common.Block, error) {
	gRPCclient, err := CreateGRPCClient(n, o)
	if err != nil {
		return nil, err
	}

	addr := n.OrdererAddress(o, nwo.ListenPort)
	conn, err := gRPCclient.NewConnection(addr, "")
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

func FetchBlock(n *nwo.Network, o *nwo.Orderer, seq uint64, channel string) *common.Block {
	denv := CreateDeliverEnvelope(n, o, seq, channel)
	Expect(denv).NotTo(BeNil())

	var blk *common.Block
	var err error
	Eventually(func() error {
		blk, err = Deliver(n, o, denv)
		return err
	}, n.EventuallyTimeout).ShouldNot(HaveOccurred())

	return blk
}

func CreateGRPCClient(n *nwo.Network, o *nwo.Orderer) (*comm.GRPCClient, error) {
	config := comm.ClientConfig{}
	config.Timeout = 5 * time.Second

	secOpts := &comm.SecureOptions{
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

func CreateBroadcastEnvelope(n *nwo.Network, signer interface{}, channel string, data []byte) *common.Envelope {
	env, err := utils.CreateSignedEnvelope(
		common.HeaderType_MESSAGE,
		channel,
		nil,
		&common.Envelope{Payload: data},
		0,
		0,
	)
	Expect(err).NotTo(HaveOccurred())

	return signAsAdmin(n, signer, env)
}

// CreateDeliverEnvelope creates a deliver env to seek for specified block.
func CreateDeliverEnvelope(n *nwo.Network, entity interface{}, blkNum uint64, channel string) *common.Envelope {
	specified := &orderer.SeekPosition{
		Type: &orderer.SeekPosition_Specified{
			Specified: &orderer.SeekSpecified{Number: blkNum},
		},
	}
	env, err := utils.CreateSignedEnvelope(
		common.HeaderType_DELIVER_SEEK_INFO,
		channel,
		nil,
		&orderer.SeekInfo{
			Start:    specified,
			Stop:     specified,
			Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
		},
		0,
		0,
	)
	Expect(err).NotTo(HaveOccurred())

	return signAsAdmin(n, entity, env)
}

func signAsAdmin(n *nwo.Network, entity interface{}, env *common.Envelope) *common.Envelope {
	var conf signer.Config
	switch t := entity.(type) {
	case *nwo.Peer:
		conf = signer.Config{
			MSPID:        n.Organization(t.Organization).MSPID,
			IdentityPath: n.PeerUserCert(t, "Admin"),
			KeyPath:      n.PeerUserKey(t, "Admin"),
		}
	case *nwo.Orderer:
		conf = signer.Config{
			MSPID:        n.Organization(t.Organization).MSPID,
			IdentityPath: n.OrdererUserCert(t, "Admin"),
			KeyPath:      n.OrdererUserKey(t, "Admin"),
		}
	default:
		panic("unsupported signing entity type")
	}

	signer, err := signer.NewSigner(conf)
	Expect(err).NotTo(HaveOccurred())

	payload, err := utils.ExtractPayload(env)
	Expect(err).NotTo(HaveOccurred())
	Expect(payload.Header).NotTo(BeNil())
	Expect(payload.Header.ChannelHeader).NotTo(BeNil())

	nonce, err := utils.CreateNonce()
	Expect(err).NotTo(HaveOccurred())
	sighdr := &common.SignatureHeader{
		Creator: signer.Creator,
		Nonce:   nonce,
	}
	payload.Header.SignatureHeader = utils.MarshalOrPanic(sighdr)
	payloadBytes := utils.MarshalOrPanic(payload)

	sig, err := signer.Sign(payloadBytes)
	Expect(err).NotTo(HaveOccurred())

	return &common.Envelope{Payload: payloadBytes, Signature: sig}
}
