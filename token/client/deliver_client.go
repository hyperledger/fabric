/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

//go:generate counterfeiter -o mock/deliver_filtered.go -fake-name DeliverFiltered . DeliverFiltered

// DeliverFiltered defines the interface that abstracts deliver filtered grpc calls to commit peer
type DeliverFiltered interface {
	Send(*common.Envelope) error
	Recv() (*pb.DeliverResponse, error)
	CloseSend() error
}

//go:generate counterfeiter -o mock/deliver_client.go -fake-name DeliverClient . DeliverClient

// DeliverClient defines the interface to create a DeliverFiltered client
type DeliverClient interface {
	// NewDeliverFilterd returns a DeliverFiltered
	NewDeliverFiltered(ctx context.Context, opts ...grpc.CallOption) (DeliverFiltered, error)

	// Certificate returns tls certificate for the deliver client to commit peer
	Certificate() *tls.Certificate
}

// deliverClient implements DeliverClient interface
type deliverClient struct {
	peerAddr           string
	serverNameOverride string
	grpcClient         *comm.GRPCClient
	conn               *grpc.ClientConn
}

func NewDeliverClient(config *ClientConfig) (DeliverClient, error) {
	grpcClient, err := createGrpcClient(&config.CommitPeerCfg, config.TlsEnabled)
	if err != nil {
		err = errors.WithMessage(err, fmt.Sprintf("failed to create a GRPCClient to peer %s", config.CommitPeerCfg.Address))
		logger.Errorf("%s", err)
		return nil, err
	}
	conn, err := grpcClient.NewConnection(config.CommitPeerCfg.Address, config.CommitPeerCfg.ServerNameOverride)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("failed to connect to commit peer %s", config.CommitPeerCfg.Address))
	}

	return &deliverClient{
		peerAddr:           config.CommitPeerCfg.Address,
		serverNameOverride: config.CommitPeerCfg.ServerNameOverride,
		grpcClient:         grpcClient,
		conn:               conn,
	}, nil
}

// NewDeliverFilterd creates a DeliverFiltered client
func (d *deliverClient) NewDeliverFiltered(ctx context.Context, opts ...grpc.CallOption) (DeliverFiltered, error) {
	if d.conn != nil {
		// close the old connection because new connection will restart its timeout
		d.conn.Close()
	}

	// create a new connection to the peer
	var err error
	d.conn, err = d.grpcClient.NewConnection(d.peerAddr, d.serverNameOverride)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("failed to connect to commit peer %s", d.peerAddr))
	}

	// create a new DeliverFiltered
	df, err := pb.NewDeliverClient(d.conn).DeliverFiltered(ctx)
	if err != nil {
		rpcStatus, _ := status.FromError(err)
		return nil, errors.Wrapf(err, "failed to new a deliver filtered, rpcStatus=%+v", rpcStatus)
	}
	return df, nil
}

func (d *deliverClient) Certificate() *tls.Certificate {
	cert := d.grpcClient.Certificate()
	return &cert
}

// create a signed envelope with SeekPosition_Newest for block
func CreateDeliverEnvelope(channelId string, creator []byte, signer SignerIdentity, cert *tls.Certificate) (*common.Envelope, error) {
	var tlsCertHash []byte
	var err error
	// check for client certificate and compute SHA2-256 on certificate if present
	if cert != nil && len(cert.Certificate) > 0 {
		tlsCertHash, err = factory.GetDefault().Hash(cert.Certificate[0], &bccsp.SHA256Opts{})
		if err != nil {
			err = errors.New("failed to compute SHA256 on client certificate")
			logger.Errorf("%s", err)
			return nil, err
		}
	}

	_, header, err := CreateHeader(common.HeaderType_DELIVER_SEEK_INFO, channelId, creator, tlsCertHash)
	if err != nil {
		return nil, err
	}

	start := &ab.SeekPosition{
		Type: &ab.SeekPosition_Newest{
			Newest: &ab.SeekNewest{},
		},
	}

	stop := &ab.SeekPosition{
		Type: &ab.SeekPosition_Specified{
			Specified: &ab.SeekSpecified{
				Number: math.MaxUint64,
			},
		},
	}

	seekInfo := &ab.SeekInfo{
		Start:    start,
		Stop:     stop,
		Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
	}

	raw, err := proto.Marshal(seekInfo)
	if err != nil {
		return nil, errors.Wrap(err, "error marshaling SeekInfo")
	}

	envelope, err := CreateEnvelope(raw, header, signer)
	if err != nil {
		return nil, err
	}

	return envelope, nil
}

func DeliverSend(df DeliverFiltered, address string, envelope *common.Envelope) error {
	err := df.Send(envelope)
	df.CloseSend()
	if err != nil {
		return errors.Wrapf(err, "failed to send deliver envelope to peer %s", address)
	}
	return nil
}

func DeliverReceive(df DeliverFiltered, address string, txid string, eventCh chan TxEvent) error {
	event := TxEvent{
		Txid:       txid,
		Committed:  false,
		CommitPeer: address,
	}

read:
	for {
		resp, err := df.Recv()
		if err != nil {
			event.Err = errors.WithMessage(err, fmt.Sprintf("error receiving deliver response from peer %s", address))
			break read
		}
		switch r := resp.Type.(type) {
		case *pb.DeliverResponse_FilteredBlock:
			filteredTransactions := r.FilteredBlock.FilteredTransactions
			for _, tx := range filteredTransactions {
				logger.Debugf("deliverReceive got filteredTransaction for transaction [%s], status [%s]", tx.Txid, tx.TxValidationCode)
				if tx.Txid == txid {
					if tx.TxValidationCode == pb.TxValidationCode_VALID {
						event.Committed = true
					} else {
						event.Err = errors.Errorf("transaction [%s] status is not valid: %s", tx.Txid, tx.TxValidationCode)
					}
					break read
				}
			}
		case *pb.DeliverResponse_Status:
			event.Err = errors.Errorf("deliver completed with status (%s) before txid %s received from peer %s", r.Status, txid, address)
			break read
		default:
			event.Err = errors.Errorf("received unexpected response type (%T) from peer %s", r, address)
			break read
		}
	}

	if event.Err != nil {
		logger.Errorf("Error: %s", event.Err)
	}
	select {
	case eventCh <- event:
		logger.Debugf("Received transaction deliver event %+v", event)
	default:
		logger.Errorf("Event channel full. Discarding event %+v", event)
	}

	return event.Err
}

// DeliverWaitForResponse waits for either eventChan has value (i.e., response has been received) or ctx is timed out
// This function assumes that the eventCh is only for the specified txid
// If an eventCh is shared by multiple transactions, a loop should be used to listen to events from multiple transactions
func DeliverWaitForResponse(ctx context.Context, eventCh chan TxEvent, txid string) (bool, error) {
	select {
	case event, _ := <-eventCh:
		if txid == event.Txid {
			return event.Committed, event.Err
		} else {
			// should never get here
			return false, errors.Errorf("no event received for txid %s", txid)
		}
	case <-ctx.Done():
		err := errors.Errorf("timed out waiting for committing txid %s", txid)
		logger.Errorf("%s", err)
		return false, err
	}
}
