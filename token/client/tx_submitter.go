/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protoutil"
	tk "github.com/hyperledger/fabric/token"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("token.client")

// TxSubmitter will submit token transactions to the orderer
// and create a channel to write/read transaction completion event
type TxSubmitter struct {
	Config          *ClientConfig
	SigningIdentity tk.SigningIdentity
	Creator         []byte
	OrdererClient   OrdererClient
	DeliverClient   DeliverClient
}

// TxEvent contains information for token transaction commit
type TxEvent struct {
	Txid       string
	Committed  bool
	CommitPeer string
	Err        error
}

// NewTxSubmitter creates a new TxSubmitter from token client config
func NewTxSubmitter(config *ClientConfig, signingIdentity tk.SigningIdentity) (*TxSubmitter, error) {
	// get and store serialized identity so that we don't have to call it for every request
	creator, err := signingIdentity.Serialize()
	if err != nil {
		return nil, err
	}

	ordererClient, err := NewOrdererClient(&config.Orderer)
	if err != nil {
		return nil, err
	}

	deliverClient, err := NewDeliverClient(&config.CommitterPeer)
	if err != nil {
		return nil, err
	}

	return &TxSubmitter{
		Config:          config,
		SigningIdentity: signingIdentity,
		Creator:         creator,
		OrdererClient:   ordererClient,
		DeliverClient:   deliverClient,
	}, nil
}

// Submit submits a token transaction to fabric orderer.
// It takes a transaction envelope and waitTimeout as input parameters.
// The 'waitTimeout' indicates how long to wait for transaction commit event.
// If it is 0, the function will not wait for transaction to be committed.
// If it is greater than 0, the function will wait until timeout or transaction is committed, whichever is earlier
// It returns orderer status, committed boolean, and error.
func (s *TxSubmitter) Submit(txEnvelope *common.Envelope, waitTimeout time.Duration) (*common.Status, bool, error) {
	if txEnvelope == nil {
		return nil, false, errors.New("envelope is nil")
	}

	txid, err := GetTransactionID(txEnvelope)
	if err != nil {
		return nil, false, err
	}

	broadcast, err := s.OrdererClient.NewBroadcast(context.Background())
	if err != nil {
		return nil, false, err
	}

	var eventCh chan TxEvent
	var ctx context.Context
	var cancelFunc context.CancelFunc
	if waitTimeout > 0 {
		ctx, cancelFunc = context.WithTimeout(context.Background(), waitTimeout)
		defer cancelFunc()
		deliverFiltered, err := s.DeliverClient.NewDeliverFiltered(ctx)
		if err != nil {
			return nil, false, err
		}
		blockEnvelope, err := CreateDeliverEnvelope(s.Config.ChannelID, s.Creator, s.SigningIdentity, s.DeliverClient.Certificate())
		if err != nil {
			return nil, false, err
		}
		err = DeliverSend(deliverFiltered, s.Config.CommitterPeer.Address, blockEnvelope)
		if err != nil {
			return nil, false, err
		}
		eventCh = make(chan TxEvent, 1)
		go DeliverReceive(deliverFiltered, s.Config.CommitterPeer.Address, txid, eventCh)
	}

	err = BroadcastSend(broadcast, s.Config.Orderer.Address, txEnvelope)
	if err != nil {
		return nil, false, err
	}

	// wait for response from orderer broadcast - it does not wait for commit peer response
	responses := make(chan common.Status)
	errs := make(chan error, 1)
	go BroadcastReceive(broadcast, s.Config.Orderer.Address, responses, errs)
	status, err := BroadcastWaitForResponse(responses, errs)

	// wait for commit event from deliver service in this case
	committed := false
	if err == nil && waitTimeout > 0 {
		committed, err = DeliverWaitForResponse(ctx, eventCh, txid)
	}

	return &status, committed, err
}

// CreateTxEnvelope creates a transaction envelope from the serialized TokenTransaction.
// It returns the transaction envelope, transaction id, and error.
func (s *TxSubmitter) CreateTxEnvelope(txBytes []byte) (*common.Envelope, string, error) {
	// check for client certificate and compute SHA2-256 on certificate if present
	tlsCertHash, err := GetTLSCertHash(s.OrdererClient.Certificate())
	if err != nil {
		return nil, "", err
	}

	txid, header, err := CreateHeader(common.HeaderType_TOKEN_TRANSACTION, s.Config.ChannelID, s.Creator, tlsCertHash)
	if err != nil {
		return nil, txid, err
	}

	txEnvelope, err := CreateEnvelope(txBytes, header, s.SigningIdentity)
	return txEnvelope, txid, err
}

// CreateHeader creates common.Header for a token transaction
// tlsCertHash is for client TLS cert, only applicable when ClientAuthRequired is true
func CreateHeader(txType common.HeaderType, channelId string, creator []byte, tlsCertHash []byte) (string, *common.Header, error) {
	ts, err := ptypes.TimestampProto(time.Now())
	if err != nil {
		return "", nil, err
	}

	nonce, err := crypto.GetRandomNonce()
	if err != nil {
		return "", nil, err
	}

	txId, err := protoutil.ComputeTxID(nonce, creator)
	if err != nil {
		return "", nil, err
	}

	chdr := &common.ChannelHeader{
		Type:        int32(txType),
		ChannelId:   channelId,
		TxId:        txId,
		Epoch:       0,
		Timestamp:   ts,
		TlsCertHash: tlsCertHash,
	}
	chdrBytes, err := proto.Marshal(chdr)
	if err != nil {
		return "", nil, err
	}

	shdr := &common.SignatureHeader{
		Creator: creator,
		Nonce:   nonce,
	}
	shdrBytes, err := proto.Marshal(shdr)
	if err != nil {
		return "", nil, err
	}

	header := &common.Header{
		ChannelHeader:   chdrBytes,
		SignatureHeader: shdrBytes,
	}

	return txId, header, nil
}

// CreateEnvelope creates a common.Envelope with given tx bytes, header, and SigningIdentity
func CreateEnvelope(data []byte, header *common.Header, signingIdentity tk.SigningIdentity) (*common.Envelope, error) {
	payload := &common.Payload{
		Header: header,
		Data:   data,
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal common.Payload")
	}

	signature, err := signingIdentity.Sign(payloadBytes)
	if err != nil {
		return nil, err
	}

	txEnvelope := &common.Envelope{
		Payload:   payloadBytes,
		Signature: signature,
	}

	return txEnvelope, nil
}

func GetTransactionID(txEnvelope *common.Envelope) (string, error) {
	payload := common.Payload{}
	err := proto.Unmarshal(txEnvelope.Payload, &payload)
	if err != nil {
		return "", errors.Wrapf(err, "failed to unmarshal envelope payload")
	}

	channelHeader := common.ChannelHeader{}
	err = proto.Unmarshal(payload.Header.ChannelHeader, &channelHeader)
	if err != nil {
		return "", errors.Wrapf(err, "failed to unmarshal channel header")
	}

	return channelHeader.TxId, nil
}
