/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package snapshotgrpc

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/pkg/errors"
)

// Snapshot Service implements SnapshotServer grpc interface
type SnapshotService struct {
	LedgerGetter LedgerGetter
}

// LedgerGetter gets the PeerLedger associated with a channel.
type LedgerGetter interface {
	GetLedger(cid string) ledger.PeerLedger
}

// Generate generates a snapshot request.
func (s *SnapshotService) Generate(ctx context.Context, signedRequest *pb.SignedSnapshotRequest) (*empty.Empty, error) {
	request := &pb.SnapshotRequest{}
	if err := proto.Unmarshal(signedRequest.Request, request); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal snapshot request")
	}

	lgr, err := s.getLedger(request.ChannelId)
	if err != nil {
		return nil, err
	}

	if err := lgr.SubmitSnapshotRequest(request.BlockNumber); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// Cancel cancels a snapshot request.
func (s *SnapshotService) Cancel(ctx context.Context, signedRequest *pb.SignedSnapshotRequest) (*empty.Empty, error) {
	request := &pb.SnapshotRequest{}
	if err := proto.Unmarshal(signedRequest.Request, request); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal snapshot request")
	}

	lgr, err := s.getLedger(request.ChannelId)
	if err != nil {
		return nil, err
	}

	if err := lgr.CancelSnapshotRequest(request.BlockNumber); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// QueryPendings returns a list of pending snapshot requests.
func (s *SnapshotService) QueryPendings(ctx context.Context, signedRequest *pb.SignedSnapshotRequest) (*pb.QueryPendingSnapshotsResponse, error) {
	query := &pb.SnapshotQuery{}
	if err := proto.Unmarshal(signedRequest.Request, query); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal snapshot request")
	}

	lgr, err := s.getLedger(query.ChannelId)
	if err != nil {
		return nil, err
	}

	result, err := lgr.PendingSnapshotRequests()
	if err != nil {
		return nil, err
	}

	return &pb.QueryPendingSnapshotsResponse{BlockNumbers: result}, nil
}

func (s *SnapshotService) getLedger(channelID string) (ledger.PeerLedger, error) {
	if channelID == "" {
		return nil, errors.New("missing channel ID")
	}

	lgr := s.LedgerGetter.GetLedger(channelID)
	if lgr == nil {
		return nil, errors.Errorf("cannot find ledger for channel %s", channelID)
	}

	return lgr, nil
}
