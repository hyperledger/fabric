/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider/fake"
	"github.com/hyperledger/fabric/common/deliverclient/orderers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestDeliveryRequester_Connect_Success(t *testing.T) {
	fakeSigner := &fake.Signer{}
	fakeSigner.SignReturns([]byte("good-sig"), nil)

	fakeDialer := &fake.Dialer{}
	cc := &grpc.ClientConn{}
	fakeDialer.DialReturns(cc, nil)

	fakeDeliverClient := &fake.DeliverClient{}
	fakeDeliverClient.SendReturns(nil)

	fakeDeliverStreamer := &fake.DeliverStreamer{}
	fakeDeliverStreamer.DeliverReturns(fakeDeliverClient, nil)

	dr := blocksprovider.NewDeliveryRequester("channel-id", fakeSigner, []byte("tls-cert-hash"), fakeDialer, fakeDeliverStreamer)
	assert.NotNil(t, dr)

	seekInfoEnv := &common.Envelope{}
	endpoint := &orderers.Endpoint{
		Address:   "orderer-address-1",
		RootCerts: [][]byte{[]byte("root-cert")},
		Refreshed: make(chan struct{}),
	}

	deliverClient, cancelFunc, err := dr.Connect(seekInfoEnv, endpoint)
	assert.NoError(t, err)
	assert.NotNil(t, deliverClient)
	assert.NotNil(t, cancelFunc)
}

func TestDeliveryRequester_Connect_DialerError(t *testing.T) {
	fakeSigner := &fake.Signer{}
	fakeSigner.SignReturns([]byte("good-sig"), nil)

	fakeDialer := &fake.Dialer{}
	dialError := errors.New("dialer-error")
	fakeDialer.DialReturns(nil, dialError)

	fakeDeliverClient := &fake.DeliverClient{}
	fakeDeliverClient.SendReturns(nil)

	fakeDeliverStreamer := &fake.DeliverStreamer{}
	fakeDeliverStreamer.DeliverReturns(fakeDeliverClient, nil)

	dr := blocksprovider.NewDeliveryRequester("channel-id", fakeSigner, []byte("tls-cert-hash"), fakeDialer, fakeDeliverStreamer)
	assert.NotNil(t, dr)

	seekInfoEnv := &common.Envelope{}
	endpoint := &orderers.Endpoint{
		Address:   "orderer-address-1",
		RootCerts: [][]byte{[]byte("root-cert")},
		Refreshed: make(chan struct{}),
	}

	deliverClient, cancelFunc, err := dr.Connect(seekInfoEnv, endpoint)
	assert.Error(t, err)
	assert.Equal(t, dialError, err)
	assert.Nil(t, deliverClient)
	assert.Nil(t, cancelFunc)
}

func TestDeliveryRequester_Connect_DeliverStreamerError(t *testing.T) {
	fakeSigner := &fake.Signer{}
	fakeSigner.SignReturns([]byte("good-sig"), nil)

	fakeDialer := &fake.Dialer{}
	cc := &grpc.ClientConn{}
	fakeDialer.DialReturns(cc, nil)

	fakeDeliverClient := &fake.DeliverClient{}
	fakeDeliverClient.SendReturns(nil)

	fakeDeliverStreamer := &fake.DeliverStreamer{}
	deliverStreamerError := errors.New("deliver-streamer-error")
	fakeDeliverStreamer.DeliverReturns(nil, deliverStreamerError)

	dr := blocksprovider.NewDeliveryRequester("channel-id", fakeSigner, []byte("tls-cert-hash"), fakeDialer, fakeDeliverStreamer)
	assert.NotNil(t, dr)

	seekInfoEnv := &common.Envelope{}
	endpoint := &orderers.Endpoint{
		Address:   "orderer-address-1",
		RootCerts: [][]byte{[]byte("root-cert")},
		Refreshed: make(chan struct{}),
	}

	deliverClient, cancelFunc, err := dr.Connect(seekInfoEnv, endpoint)
	assert.Error(t, err)
	assert.Equal(t, deliverStreamerError, err)
	assert.Nil(t, deliverClient)
	assert.Nil(t, cancelFunc)
}

func TestDeliveryRequester_Connect_DeliverClientError(t *testing.T) {
	fakeSigner := &fake.Signer{}
	fakeSigner.SignReturns([]byte("good-sig"), nil)

	fakeDialer := &fake.Dialer{}
	cc := &grpc.ClientConn{}
	fakeDialer.DialReturns(cc, nil)

	fakeDeliverClient := &fake.DeliverClient{}
	deliverClientError := errors.New("deliver-client-error")
	fakeDeliverClient.SendReturns(deliverClientError)

	fakeDeliverStreamer := &fake.DeliverStreamer{}
	fakeDeliverStreamer.DeliverReturns(fakeDeliverClient, nil)

	dr := blocksprovider.NewDeliveryRequester("channel-id", fakeSigner, []byte("tls-cert-hash"), fakeDialer, fakeDeliverStreamer)
	assert.NotNil(t, dr)

	seekInfoEnv := &common.Envelope{}
	endpoint := &orderers.Endpoint{
		Address:   "orderer-address-1",
		RootCerts: [][]byte{[]byte("root-cert")},
		Refreshed: make(chan struct{}),
	}

	deliverClient, cancelFunc, err := dr.Connect(seekInfoEnv, endpoint)
	assert.Error(t, err)
	assert.Equal(t, deliverClientError, err)
	assert.Nil(t, deliverClient)
	assert.Nil(t, cancelFunc)
}

func TestDeliveryRequester_SeekInfoBlocksFrom(t *testing.T) {
	fakeSigner := &fake.Signer{}
	fakeSigner.SignReturns([]byte("good-sig"), nil)

	fakeDialer := &fake.Dialer{}
	cc := &grpc.ClientConn{}
	fakeDialer.DialReturns(cc, nil)

	fakeDeliverClient := &fake.DeliverClient{}
	fakeDeliverClient.SendReturns(nil)

	fakeDeliverStreamer := &fake.DeliverStreamer{}
	fakeDeliverStreamer.DeliverReturns(fakeDeliverClient, nil)

	dr := blocksprovider.NewDeliveryRequester("channel-id", fakeSigner, []byte("tls-cert-hash"), fakeDialer, fakeDeliverStreamer)
	assert.NotNil(t, dr)

	envelope, err := dr.SeekInfoBlocksFrom(1000)
	assert.NoError(t, err)
	assert.NotNil(t, envelope)
}

func TestDeliveryRequester_SeekInfoHeadersFrom(t *testing.T) {
	fakeSigner := &fake.Signer{}
	fakeSigner.SignReturns([]byte("good-sig"), nil)

	fakeDialer := &fake.Dialer{}
	cc := &grpc.ClientConn{}
	fakeDialer.DialReturns(cc, nil)

	fakeDeliverClient := &fake.DeliverClient{}
	fakeDeliverClient.SendReturns(nil)

	fakeDeliverStreamer := &fake.DeliverStreamer{}
	fakeDeliverStreamer.DeliverReturns(fakeDeliverClient, nil)

	dr := blocksprovider.NewDeliveryRequester("channel-id", fakeSigner, []byte("tls-cert-hash"), fakeDialer, fakeDeliverStreamer)
	assert.NotNil(t, dr)

	envelope, err := dr.SeekInfoHeadersFrom(1000)
	assert.NoError(t, err)
	assert.NotNil(t, envelope)
}
