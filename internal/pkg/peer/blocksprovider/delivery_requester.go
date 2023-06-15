/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"context"
	"math"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// DeliveryRequester is used to connect to an orderer and request the delivery of various types of block delivery
// streams. The type of stream requested depends upon the orderer.SeekInfo created.
type DeliveryRequester struct {
	channelID       string
	signer          identity.SignerSerializer
	tlsCertHash     []byte
	dialer          Dialer
	deliverStreamer DeliverStreamer
}

func NewDeliveryRequester(
	channelID string,
	signer identity.SignerSerializer,
	tlsCertHash []byte,
	dialer Dialer,
	deliverStreamer DeliverStreamer,
) *DeliveryRequester {
	return &DeliveryRequester{
		channelID:       channelID,
		signer:          signer,
		tlsCertHash:     tlsCertHash,
		dialer:          dialer,
		deliverStreamer: deliverStreamer,
	}
}

func (dr *DeliveryRequester) SeekInfoBlocksFrom(ledgerHeight uint64) (*common.Envelope, error) {
	return protoutil.CreateSignedEnvelopeWithTLSBinding(
		common.HeaderType_DELIVER_SEEK_INFO,
		dr.channelID,
		dr.signer,
		&orderer.SeekInfo{
			Start: &orderer.SeekPosition{
				Type: &orderer.SeekPosition_Specified{
					Specified: &orderer.SeekSpecified{
						Number: ledgerHeight,
					},
				},
			},
			Stop: &orderer.SeekPosition{
				Type: &orderer.SeekPosition_Specified{
					Specified: &orderer.SeekSpecified{
						Number: math.MaxUint64,
					},
				},
			},
			Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
		},
		int32(0),
		uint64(0),
		dr.tlsCertHash,
	)
}

func (dr *DeliveryRequester) Connect(seekInfoEnv *common.Envelope, endpoint *orderers.Endpoint) (orderer.AtomicBroadcast_DeliverClient, func(), error) {
	conn, err := dr.dialer.Dial(endpoint.Address, endpoint.RootCerts)
	if err != nil {
		return nil, nil, errors.WithMessagef(err, "could not dial endpoint '%s'", endpoint.Address)
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	deliverClient, err := dr.deliverStreamer.Deliver(ctx, conn)
	if err != nil {
		_ = conn.Close()
		ctxCancel()
		return nil, nil, errors.WithMessagef(err, "could not create deliver client to endpoints '%s'", endpoint.Address)
	}

	err = deliverClient.Send(seekInfoEnv)
	if err != nil {
		_ = deliverClient.CloseSend()
		_ = conn.Close()
		ctxCancel()
		return nil, nil, errors.WithMessagef(err, "could not send deliver seek info handshake to '%s'", endpoint.Address)
	}

	cancelFunc := func() {
		_ = deliverClient.CloseSend()
		ctxCancel()
		_ = conn.Close()
	}

	return deliverClient, cancelFunc, nil
}
