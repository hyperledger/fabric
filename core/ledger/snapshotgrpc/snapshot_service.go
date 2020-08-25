/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package snapshotgrpc

import (
	"context"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// Snapshot Service implements SnapshotServer grpc interface
type SnapshotService struct {
	LedgerGetter LedgerGetter
	ACLProvider  ACLProvider
}

// LedgerGetter gets the PeerLedger associated with a channel.
type LedgerGetter interface {
	GetLedger(cid string) ledger.PeerLedger
}

// ACLProvider checks ACL for a channelless resource
type ACLProvider interface {
	CheckACLNoChannel(resName string, idinfo interface{}) error
}

// Generate generates a snapshot request.
func (s *SnapshotService) Generate(ctx context.Context, signedRequest *pb.SignedSnapshotRequest) (*empty.Empty, error) {
	request := &pb.SnapshotRequest{}
	if err := proto.Unmarshal(signedRequest.Request, request); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal snapshot request")
	}

	if err := s.checkACL(resources.Snapshot_submitrequest, request.SignatureHeader, signedRequest); err != nil {
		return nil, err
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

	if err := s.checkACL(resources.Snapshot_cancelrequest, request.SignatureHeader, signedRequest); err != nil {
		return nil, err
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

	if err := s.checkACL(resources.Snapshot_listpending, query.SignatureHeader, signedRequest); err != nil {
		return nil, err
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

func (s *SnapshotService) checkACL(resName string, signatureHdr *cb.SignatureHeader, signedRequest *pb.SignedSnapshotRequest) error {
	if signatureHdr == nil {
		return errors.New("missing signature header")
	}

	expirationTime := crypto.ExpiresAt(signatureHdr.Creator)
	if !expirationTime.IsZero() && time.Now().After(expirationTime) {
		return errors.New("client identity expired")
	}

	if err := s.ACLProvider.CheckACLNoChannel(
		resName,
		[]*protoutil.SignedData{{
			Identity:  signatureHdr.Creator,
			Data:      signedRequest.Request,
			Signature: signedRequest.Signature,
		}},
	); err != nil {
		return err
	}

	return nil
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
