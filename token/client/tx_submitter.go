/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package client

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	peercommon "github.com/hyperledger/fabric/peer/common"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("token.client")

type TxSubmitter struct {
	Config        *ClientConfig
	Signer        SignerIdentity
	Creator       []byte
	OrdererClient OrdererClient
	DeliverClient DeliverClient
}

// TxEvent contains information for token transaction commit
// If application wants to be notified when a token transaction is committed,
// do the following:
// - create a event chan with size 1 or bigger, e.g. txChan := make(chan TxEvent, 1)
// - call client.SubmitTransactionWithChan(txBytes, txChan)
// - implement a function to read TxEvent from txChan so that it will be notified when transaction is committed or failed
type TxEvent struct {
	Txid       string
	Committed  bool
	CommitPeer string
	Err        error
}

// NewTransactionSubmitter creates a new TxSubmitter from token client config
func NewTxSubmitter(config *ClientConfig) (*TxSubmitter, error) {
	err := ValidateClientConfig(config)
	if err != nil {
		return nil, err
	}

	// TODO: make mspType configurable
	mspType := "bccsp"
	peercommon.InitCrypto(config.MspDir, config.MspId, mspType)

	Signer, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		return nil, err
	}
	creator, err := Signer.Serialize()
	if err != nil {
		return nil, err
	}

	ordererClient, err := NewOrdererClient(config)
	if err != nil {
		return nil, err
	}

	deliverClient, err := NewDeliverClient(config)
	if err != nil {
		return nil, err
	}

	return &TxSubmitter{
		Config:        config,
		Signer:        Signer,
		Creator:       creator,
		OrdererClient: ordererClient,
		DeliverClient: deliverClient,
	}, nil
}

// SubmitTransaction submits a token transaction to fabric.
// It takes TokenTransaction bytes and waitTimeInSeconds as input parameters.
// The 'waitTimeInSeconds' indicates how long to wait for transaction commit event.
// If it is 0, the function will not wait for transaction to be committed.
// If it is greater than 0, the function will wait until timeout or transaction is committed, whichever is earlier
func (s *TxSubmitter) SubmitTransaction(txEnvelope *common.Envelope, waitTimeInSeconds int) (committed bool, txId string, err error) {
	if waitTimeInSeconds > 0 {
		waitTime := time.Second * time.Duration(waitTimeInSeconds)
		ctx, cancelFunc := context.WithTimeout(context.Background(), waitTime)
		defer cancelFunc()
		localCh := make(chan TxEvent, 1)
		committed, txId, err = s.sendTransactionInternal(txEnvelope, ctx, localCh, true)
		close(localCh)
		return
	} else {
		committed, txId, err = s.sendTransactionInternal(txEnvelope, context.Background(), nil, false)
		return
	}
}

// SubmitTransactionWithChan submits a token transaction to fabric with an event channel.
// This function does not wait for transaction commit and returns as soon as the orderer client receives the response.
// The application will be notified on transaction completion by reading events from the eventCh.
// When the transaction is committed or failed, an event will be added to eventCh so that the application will be notified.
// If eventCh has buffer size 0 or its buffer is full, an error will be returned.
func (s *TxSubmitter) SubmitTransactionWithChan(txEnvelope *common.Envelope, eventCh chan TxEvent) (committed bool, txId string, err error) {
	committed, txId, err = s.sendTransactionInternal(txEnvelope, context.Background(), eventCh, false)
	return
}

func (s *TxSubmitter) sendTransactionInternal(txEnvelope *common.Envelope, ctx context.Context, eventCh chan TxEvent, waitForCommit bool) (bool, string, error) {
	if eventCh != nil && cap(eventCh) == 0 {
		return false, "", errors.New("eventCh buffer size must be greater than 0")
	}
	if eventCh != nil && len(eventCh) == cap(eventCh) {
		return false, "", errors.New("eventCh buffer is full. Read events and try again")
	}

	txid, err := getTransactionId(txEnvelope)
	if err != nil {
		return false, "", err
	}

	broadcast, err := s.OrdererClient.NewBroadcast(context.Background())
	if err != nil {
		return false, "", err
	}

	committed := false
	if eventCh != nil {
		deliverFiltered, err := s.DeliverClient.NewDeliverFiltered(ctx)
		if err != nil {
			return false, "", err
		}
		blockEnvelope, err := CreateDeliverEnvelope(s.Config.ChannelId, s.Creator, s.Signer, s.DeliverClient.Certificate())
		if err != nil {
			return false, "", err
		}
		err = DeliverSend(deliverFiltered, s.Config.CommitPeerCfg.Address, blockEnvelope)
		if err != nil {
			return false, "", err
		}
		go DeliverReceive(deliverFiltered, s.Config.CommitPeerCfg.Address, txid, eventCh)
	}

	err = BroadcastSend(broadcast, s.Config.OrdererCfg.Address, txEnvelope)
	if err != nil {
		return false, txid, err
	}

	// wait for response from orderer broadcast - it does not wait for commit peer response
	responses := make(chan common.Status)
	errs := make(chan error, 1)
	go BroadcastReceive(broadcast, s.Config.OrdererCfg.Address, responses, errs)
	_, err = BroadcastWaitForResponse(responses, errs)

	// wait for commit event from deliver service in this case
	if eventCh != nil && waitForCommit {
		committed, err = DeliverWaitForResponse(ctx, eventCh, txid)
	}

	return committed, txid, err
}

func (s *TxSubmitter) CreateTxEnvelope(txBytes []byte) (string, *common.Envelope, error) {
	// channelId string, creator []byte, signer SignerIdentity, cert *tls.Certificate
	// , s.Config.ChannelId, s.Creator, s.Signer, s.OrdererClient.Certificate()
	var tlsCertHash []byte
	var err error
	// check for client certificate and compute SHA2-256 on certificate if present
	cert := s.OrdererClient.Certificate()
	if cert != nil && len(cert.Certificate) > 0 {
		tlsCertHash, err = factory.GetDefault().Hash(cert.Certificate[0], &bccsp.SHA256Opts{})
		if err != nil {
			err = errors.New("failed to compute SHA256 on client certificate")
			logger.Errorf("%s", err)
			return "", nil, err
		}
	}

	txid, header, err := CreateHeader(common.HeaderType_TOKEN_TRANSACTION, s.Config.ChannelId, s.Creator, tlsCertHash)
	if err != nil {
		return txid, nil, err
	}

	txEnvelope, err := CreateEnvelope(txBytes, header, s.Signer)
	return txid, txEnvelope, err
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

	txId, err := utils.ComputeTxID(nonce, creator)
	if err != nil {
		return "", nil, err
	}

	chdr := &common.ChannelHeader{
		Type:        int32(txType),
		ChannelId:   channelId,
		TxId:        txId,
		Epoch:       uint64(0),
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

// CreateEnvelope creates a common.Envelope with given tx bytes, header, and Signer
func CreateEnvelope(data []byte, header *common.Header, signer SignerIdentity) (*common.Envelope, error) {
	payload := &common.Payload{
		Header: header,
		Data:   data,
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal common.Payload")
	}

	signature, err := signer.Sign(payloadBytes)
	if err != nil {
		return nil, err
	}

	txEnvelope := &common.Envelope{
		Payload:   payloadBytes,
		Signature: signature,
	}

	return txEnvelope, nil
}

func getTransactionId(txEnvelope *common.Envelope) (string, error) {
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

// createGrpcClient returns a comm.GRPCClient based on toke client config
func createGrpcClient(cfg *ConnectionConfig, tlsEnabled bool) (*comm.GRPCClient, error) {
	clientConfig := comm.ClientConfig{Timeout: time.Second}

	if tlsEnabled {
		if cfg.TlsRootCertFile == "" {
			return nil, errors.New("missing TlsRootCertFile in client config")
		}
		caPEM, err := ioutil.ReadFile(cfg.TlsRootCertFile)
		if err != nil {
			return nil, errors.WithMessage(err, fmt.Sprintf("unable to load TLS cert from %s", cfg.TlsRootCertFile))
		}
		secOpts := &comm.SecureOptions{
			UseTLS:            true,
			ServerRootCAs:     [][]byte{caPEM},
			RequireClientCert: false,
		}
		clientConfig.SecOpts = secOpts
	}

	return comm.NewGRPCClient(clientConfig)
}
