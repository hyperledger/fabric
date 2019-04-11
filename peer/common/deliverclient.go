/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/peer/common/api"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

var (
	logger = flogging.MustGetLogger("cli.common")

	seekNewest = &ab.SeekPosition{
		Type: &ab.SeekPosition_Newest{
			Newest: &ab.SeekNewest{},
		},
	}
	seekOldest = &ab.SeekPosition{
		Type: &ab.SeekPosition_Oldest{
			Oldest: &ab.SeekOldest{},
		},
	}
)

// DeliverClient holds the necessary information to connect a client
// to an orderer/peer deliver service
type DeliverClient struct {
	Service     api.DeliverService
	ChannelID   string
	TLSCertHash []byte
	BestEffort  bool
}

func (d *DeliverClient) seekSpecified(blockNumber uint64) error {
	seekPosition := &ab.SeekPosition{
		Type: &ab.SeekPosition_Specified{
			Specified: &ab.SeekSpecified{
				Number: blockNumber,
			},
		},
	}
	env := seekHelper(d.ChannelID, seekPosition, d.TLSCertHash, d.BestEffort)
	return d.Service.Send(env)
}

func (d *DeliverClient) seekOldest() error {
	env := seekHelper(d.ChannelID, seekOldest, d.TLSCertHash, d.BestEffort)
	return d.Service.Send(env)
}

func (d *DeliverClient) seekNewest() error {
	env := seekHelper(d.ChannelID, seekNewest, d.TLSCertHash, d.BestEffort)
	return d.Service.Send(env)
}

func (d *DeliverClient) readBlock() (*cb.Block, error) {
	msg, err := d.Service.Recv()
	if err != nil {
		return nil, errors.Wrap(err, "error receiving")
	}
	switch t := msg.Type.(type) {
	case *ab.DeliverResponse_Status:
		logger.Infof("Got status: %v", t)
		return nil, errors.Errorf("can't read the block: %v", t)
	case *ab.DeliverResponse_Block:
		logger.Infof("Received block: %v", t.Block.Header.Number)
		d.Service.Recv() // Flush the success message
		return t.Block, nil
	default:
		return nil, errors.Errorf("response error: unknown type %T", t)
	}
}

// GetSpecifiedBlock gets the specified block from a peer/orderer's deliver
// service
func (d *DeliverClient) GetSpecifiedBlock(num uint64) (*cb.Block, error) {
	err := d.seekSpecified(num)
	if err != nil {
		return nil, errors.WithMessage(err, "error getting specified block")
	}

	return d.readBlock()
}

// GetOldestBlock gets the oldest block from a peer/orderer's deliver service
func (d *DeliverClient) GetOldestBlock() (*cb.Block, error) {
	err := d.seekOldest()
	if err != nil {
		return nil, errors.WithMessage(err, "error getting oldest block")
	}

	return d.readBlock()
}

// GetNewestBlock gets the newest block from a peer/orderer's deliver service
func (d *DeliverClient) GetNewestBlock() (*cb.Block, error) {
	err := d.seekNewest()
	if err != nil {
		return nil, errors.WithMessage(err, "error getting newest block")
	}

	return d.readBlock()
}

// Close closes a deliver client's connection
func (d *DeliverClient) Close() error {
	return d.Service.CloseSend()
}

func seekHelper(
	channelID string,
	position *ab.SeekPosition,
	tlsCertHash []byte,
	bestEffort bool,
) *cb.Envelope {
	seekInfo := &ab.SeekInfo{
		Start:    position,
		Stop:     position,
		Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
	}

	if bestEffort {
		seekInfo.ErrorResponse = ab.SeekInfo_BEST_EFFORT
	}

	env, err := utils.CreateSignedEnvelopeWithTLSBinding(
		cb.HeaderType_DELIVER_SEEK_INFO,
		channelID,
		localmsp.NewSigner(),
		seekInfo,
		int32(0),
		uint64(0),
		tlsCertHash,
	)
	if err != nil {
		logger.Errorf("Error signing envelope:  %s", err)
		return nil
	}

	return env
}

type ordererDeliverService struct {
	ab.AtomicBroadcast_DeliverClient
}

// NewDeliverClientForOrderer creates a new DeliverClient from an OrdererClient
func NewDeliverClientForOrderer(channelID string, bestEffort bool) (*DeliverClient, error) {
	var tlsCertHash []byte
	oc, err := NewOrdererClientFromEnv()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create deliver client")
	}

	dc, err := oc.Deliver()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create deliver client")
	}
	// check for client certificate and create hash if present
	if len(oc.Certificate().Certificate) > 0 {
		tlsCertHash = util.ComputeSHA256(oc.Certificate().Certificate[0])
	}
	ds := &ordererDeliverService{dc}
	o := &DeliverClient{
		Service:     ds,
		ChannelID:   channelID,
		TLSCertHash: tlsCertHash,
		BestEffort:  bestEffort,
	}
	return o, nil
}

type peerDeliverService struct {
	pb.Deliver_DeliverClient
}

// NewDeliverClientForPeer creates a new DeliverClient from a PeerClient
func NewDeliverClientForPeer(channelID string, bestEffort bool) (*DeliverClient, error) {
	var tlsCertHash []byte
	pc, err := NewPeerClientFromEnv()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create deliver client")
	}

	d, err := pc.Deliver()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create deliver client")
	}

	// check for client certificate and create hash if present
	if len(pc.Certificate().Certificate) > 0 {
		tlsCertHash = util.ComputeSHA256(pc.Certificate().Certificate[0])
	}
	ds := &peerDeliverService{d}
	p := &DeliverClient{
		Service:     ds,
		ChannelID:   channelID,
		TLSCertHash: tlsCertHash,
		BestEffort:  bestEffort,
	}
	return p, nil
}

func (p *peerDeliverService) Recv() (*ab.DeliverResponse, error) {
	pbResp, err := p.Deliver_DeliverClient.Recv()
	if err != nil {
		return nil, errors.Wrap(err, "error receiving from peer deliver service")
	}

	abResp := &ab.DeliverResponse{}

	switch t := pbResp.Type.(type) {
	case *pb.DeliverResponse_Status:
		abResp.Type = &ab.DeliverResponse_Status{Status: t.Status}
	case *pb.DeliverResponse_Block:
		abResp.Type = &ab.DeliverResponse_Block{Block: t.Block}
	default:
		return nil, errors.Errorf("response error: unknown type %T", t)
	}

	return abResp, nil
}
