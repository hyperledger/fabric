/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider_test

import (
	"context"
	"crypto/x509"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider/fake"
	"github.com/hyperledger/fabric/common/deliverclient/orderers"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var ca = createCAOrPanic()

func createCAOrPanic() tlsgen.CA {
	ca, err := tlsgen.NewCA()
	if err != nil {
		panic(fmt.Sprintf("failed creating CA: %+v", err))
	}
	return ca
}

type newDialer struct {
	ClientConfig comm.ClientConfig
}

func (nd *newDialer) Dial(address string, rootCerts [][]byte) (*grpc.ClientConn, error) {
	cc := nd.ClientConfig
	ctx, cancel := context.WithTimeout(context.Background(), cc.DialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address, grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new connection")
	}
	return conn, nil
}

func readSeekEnvelope(stream orderer.AtomicBroadcast_DeliverServer) (*orderer.SeekInfo, string, error) {
	env, err := stream.Recv()
	if err != nil {
		return nil, "", err
	}
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, "", err
	}
	seekInfo := &orderer.SeekInfo{}
	if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
		return nil, "", err
	}
	chdr := &common.ChannelHeader{}
	if err = proto.Unmarshal(payload.Header.ChannelHeader, chdr); err != nil {
		return nil, "", err
	}
	return seekInfo, chdr.ChannelId, nil
}

type deliverServer struct {
	logger  *flogging.FabricLogger
	cert    *x509.Certificate
	rawCert []byte
	t       *testing.T
	sync.Mutex
	err            error
	srv            *comm.GRPCServer
	seekAssertions chan func(*orderer.SeekInfo, string)
	blockResponses chan *orderer.DeliverResponse
	done           chan struct{}
}

func (ds *deliverServer) Deliver(stream orderer.AtomicBroadcast_DeliverServer) error {
	ds.Lock()
	err := ds.err
	ds.Unlock()
	if err != nil {
		return nil
	}
	seekInfo, channel, err := readSeekEnvelope(stream)
	require.NoError(ds.t, err)
	timer := time.NewTimer(1 * time.Minute)
	defer timer.Stop()

	select {
	case <-timer.C:
		ds.t.Fatalf("timed out waiting for seek assertions to receive a value\n")

	case seekAssert := <-ds.seekAssertions:
		ds.logger.Debugf("Received seekInfo: %+v", seekInfo)
		seekAssert(seekInfo, channel)
	case <-ds.done:
		return nil
	}

	if seekInfo.GetStart().GetSpecified() != nil {
		return ds.deliverBlocks(stream)
	}
	if seekInfo.GetStart().GetNewest() != nil {
		select {
		case resp := <-ds.blocks():
			if resp == nil {
				return nil
			}
			return stream.Send(resp)
		case <-ds.done:
		}
	}
	ds.t.Fatalf("expected either specified or newset seek but got %v\n", seekInfo.GetStart())
	seekInfo.GetStart()
	return nil
}

func (ds *deliverServer) deliverBlocks(stream orderer.AtomicBroadcast_DeliverServer) error {
	for {
		blockChan := ds.blocks()
		var response *orderer.DeliverResponse
		select {
		case response = <-blockChan:
		case <-ds.done:
			return nil
		}

		if response == nil {
			return nil
		}
		if err := stream.Send(response); err != nil {
			return err
		}
	}
}

func (ds *deliverServer) blocks() chan *orderer.DeliverResponse {
	ds.Lock()
	defer ds.Unlock()
	blockChan := ds.blockResponses
	return blockChan
}

func (ds *deliverServer) stop() {
	ds.srv.Stop()
	close(ds.blocks())
	close(ds.done)
}

func (ds *deliverServer) Broadcast(orderer.AtomicBroadcast_BroadcastServer) error {
	panic("implement me")
}

func newClusterNode(t *testing.T) *deliverServer {
	srv, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{})
	require.NoError(t, err)
	ds := &deliverServer{
		logger:         flogging.MustGetLogger("test.debug"),
		t:              t,
		seekAssertions: make(chan func(*orderer.SeekInfo, string), 100),
		blockResponses: make(chan *orderer.DeliverResponse, 100),
		done:           make(chan struct{}),
		srv:            srv,
	}
	orderer.RegisterAtomicBroadcastServer(srv.Server(), ds)
	go srv.Start()
	return ds
}

func newClusterNodeWithTLS(t *testing.T) *deliverServer {
	cert, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)

	srv, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			Key:         cert.Key,
			Certificate: cert.Cert,
			UseTLS:      true,
		},
	})

	require.NoError(t, err)
	ds := &deliverServer{
		rawCert:        cert.Cert,
		cert:           cert.TLSCert,
		logger:         flogging.MustGetLogger("test.debug"),
		t:              t,
		seekAssertions: make(chan func(*orderer.SeekInfo, string), 100),
		blockResponses: make(chan *orderer.DeliverResponse, 100),
		done:           make(chan struct{}),
		srv:            srv,
	}
	orderer.RegisterAtomicBroadcastServer(srv.Server(), ds)
	go srv.Start()
	return ds
}

func TestDeliveryRequester_Connect(t *testing.T) {
	osn := newClusterNode(t)
	defer osn.stop()
	fakeSigner := &fake.Signer{}
	fakeSigner.SignReturns([]byte("good-sig"), nil)

	fakeDialer := newDialer{
		ClientConfig: comm.ClientConfig{
			DialTimeout: 100 * time.Millisecond,
		},
	}
	fakeDeliverStreamer := blocksprovider.DeliverAdapter{}

	dr := blocksprovider.NewDeliveryRequester(
		"channel-id",
		fakeSigner,
		nil,
		&fakeDialer,
		fakeDeliverStreamer,
	)

	seekInfoEnv, err := dr.SeekInfoBlocksFrom(100)
	require.NoError(t, err)
	endpoint := &orderers.Endpoint{
		Address:   osn.srv.Address(),
		RootCerts: nil,
	}

	deliverClient, cancelFunc, err := dr.Connect(seekInfoEnv, endpoint)
	time.Sleep(100 * time.Millisecond)
	assert.Nil(t, err)
	assert.NotNil(t, deliverClient)
	assert.NotNil(t, cancelFunc)
}

func TestDeliveryRequester_SeekInfoBlocksFrom(t *testing.T) {
	fakeSigner := &fake.Signer{}
	fakeSigner.SignReturns([]byte("good-sig"), nil)

	fakeDialer := newDialer{
		ClientConfig: comm.ClientConfig{
			DialTimeout: 1 * time.Second,
		},
	}

	fakeDeliverStreamer := &blocksprovider.DeliverAdapter{}

	dr := blocksprovider.NewDeliveryRequester(
		"channel-id",
		fakeSigner,
		[]byte("tls-cert-hash"),
		&fakeDialer,
		fakeDeliverStreamer)
	assert.NotNil(t, dr)

	envelope, err := dr.SeekInfoBlocksFrom(1000)
	assert.NoError(t, err)
	assert.NotNil(t, envelope)
}

func TestDeliveryRequester_SeekInfoHeadersFrom(t *testing.T) {
	fakeSigner := &fake.Signer{}
	fakeSigner.SignReturns([]byte("good-sig"), nil)

	fakeDialer := newDialer{
		ClientConfig: comm.ClientConfig{
			DialTimeout: 1 * time.Second,
		},
	}

	fakeDeliverStreamer := &blocksprovider.DeliverAdapter{}

	dr := blocksprovider.NewDeliveryRequester(
		"channel-id",
		fakeSigner,
		[]byte("tls-cert-hash"),
		&fakeDialer,
		fakeDeliverStreamer)
	assert.NotNil(t, dr)

	envelope, err := dr.SeekInfoHeadersFrom(1000)
	assert.NoError(t, err)
	assert.NotNil(t, envelope)
}
