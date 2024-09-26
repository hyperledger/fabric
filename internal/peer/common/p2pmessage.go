package common

import (
	"context"
	"fmt"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/peer/protos"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/pkg/errors"
)

// P2PMessageClient holds the necessary information to connect a client
// to an orderer/peer deliver service
type P2PMessageClient struct {
	Signer      identity.SignerSerializer
	Service     protos.ReconcileServiceClient
	ChannelID   string
	TLSCertHash []byte
}

func (p *P2PMessageClient) reconcileSpecified(blockNumber uint64) (*protos.ReconcileResponse, error) {
	reconcileRequest := &protos.ReconcileRequest{
		BlockNumber: blockNumber,
		ChannelId:   p.ChannelID,
	}
	//env := reconHelper(p.ChannelID, seekPosition, p.TLSCertHash, p.Signer)
	fmt.Println(reconcileRequest)
	return p.Service.SendReconcileRequest(context.Background(), reconcileRequest)
}

func (p *P2PMessageClient) ReconcileSpecifiedBlock(num uint64) (*protos.ReconcileResponse, error) {
	logger.Debugf("Reconciling block %d", num)
	response, err := p.reconcileSpecified(num)
	if err != nil {
		return nil, errors.WithMessage(err, "error getting specified block")
	}

	return response, nil
}

func (p *P2PMessageClient) Close() error {
	logger.Debugf("Reconciliation client close")
	return nil
}

// NewP2PMessageClientForOrderer creates a new DeliverClient from an OrdererClient
func NewP2PMessageClientForOrderer(channelID string, signer identity.SignerSerializer) (*P2PMessageClient, error) {
	oc, err := NewOrdererClientFromEnv()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create deliver client for orderer")
	}

	dc, err := oc.P2PMessage()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create deliver client for orderer")
	}
	// check for client certificate and create hash if present
	var tlsCertHash []byte
	if len(oc.Certificate().Certificate) > 0 {
		tlsCertHash = util.ComputeSHA256(oc.Certificate().Certificate[0])
	}
	ds := &ordererP2PDeliverService{dc}
	o := &P2PMessageClient{
		Signer:      signer,
		Service:     ds,
		ChannelID:   channelID,
		TLSCertHash: tlsCertHash,
	}
	return o, nil
}

// NewP2PMessageClientForPeer creates a new DeliverClient from a PeerClient
func NewP2PMessageClientForPeer(channelID string, signer identity.SignerSerializer) (*P2PMessageClient, error) {
	var tlsCertHash []byte
	pc, err := NewPeerClientFromEnv()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create deliver client for peer")
	}

	d, err := pc.P2PMessage()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create deliver client for peer")
	}

	// check for client certificate and create hash if present
	if len(pc.Certificate().Certificate) > 0 {
		tlsCertHash = util.ComputeSHA256(pc.Certificate().Certificate[0])
	}
	ds := &peerP2PDeliverService{d}
	p := &P2PMessageClient{
		Signer:      signer,
		Service:     ds,
		ChannelID:   channelID,
		TLSCertHash: tlsCertHash,
	}
	return p, nil
}

//func reconHelper(
//	channelID string,
//	position *ab.SeekPosition,
//	tlsCertHash []byte,
//	signer identity.SignerSerializer,
//) *cb.Envelope {
//	seekInfo := &ab.SeekInfo{
//		Start:    position,
//		Stop:     position,
//		Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
//	}
//
//	env, err := protoutil.CreateSignedEnvelopeWithTLSBinding(
//		cb.HeaderType_DELIVER_SEEK_INFO,
//		channelID,
//		signer,
//		seekInfo,
//		int32(0),
//		uint64(0),
//		tlsCertHash,
//	)
//	if err != nil {
//		logger.Errorf("Error signing envelope:  %s", err)
//		return nil
//	}
//
//	return env
//}
