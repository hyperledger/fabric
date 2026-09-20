/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package snapshotgrpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt/ledgermgmttest"
	"github.com/hyperledger/fabric/core/ledger/snapshotgrpc/mock"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mock/ledger_getter.go -fake-name LedgerGetter . ledgerGetter
//go:generate counterfeiter -o mock/acl_provider.go -fake-name ACLProvider . aclProvider

type ledgerGetter interface {
	LedgerGetter
}

type aclProvider interface {
	ACLProvider
}

func TestSnapshot(t *testing.T) {
	testDir := t.TempDir()

	ledgerID := "testsnapshot"
	ledgermgmtInitializer := ledgermgmttest.NewInitializer(testDir)
	ledgerMgr := ledgermgmt.NewLedgerMgr(ledgermgmtInitializer)
	gb, err := test.MakeGenesisBlock(ledgerID)
	require.NoError(t, err)
	lgr, err := ledgerMgr.CreateLedger(ledgerID, gb)
	require.NoError(t, err)
	defer lgr.Close()

	fakeLedgerGetter := &mock.LedgerGetter{}
	fakeLedgerGetter.GetLedgerReturns(lgr)
	fakeACLProvider := &mock.ACLProvider{}
	fakeACLProvider.CheckACLNoChannelReturns(nil)
	snapshotSvc := &SnapshotService{LedgerGetter: fakeLedgerGetter, ACLProvider: fakeACLProvider}

	// test generate, cancel and query bindings
	var signedRequest *pb.SignedSnapshotRequest
	testData := []uint64{100, 50, 200}
	for _, blockNumber := range testData {
		signedRequest = createSignedRequest(ledgerID, blockNumber)
		_, err = snapshotSvc.Generate(context.Background(), signedRequest)
		require.NoError(t, err)
	}

	signedRequest = createSignedRequest(ledgerID, 100)
	_, err = snapshotSvc.Cancel(context.Background(), signedRequest)
	require.NoError(t, err)

	expectedResponse := &pb.QueryPendingSnapshotsResponse{BlockNumbers: []uint64{50, 200}}
	signedQuery := createSignedQuery(ledgerID)
	resp, err := snapshotSvc.QueryPendings(context.Background(), signedQuery)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, resp)

	// test error propagation from ledger, generate blockNumber=50 should return an error
	signedRequest = createSignedRequest(ledgerID, 50)
	_, err = snapshotSvc.Generate(context.Background(), signedRequest)
	require.EqualError(t, err, "duplicate snapshot request for block number 50")

	// test error propagation from ledger, cancel blockNumber=100 again should return an error
	signedRequest = createSignedRequest(ledgerID, 100)
	_, err = snapshotSvc.Cancel(context.Background(), signedRequest)
	require.EqualError(t, err, "no snapshot request exists for block number 100")

	// common error tests for all requests
	tests := []struct {
		name          string
		channelID     string
		signedRequest *pb.SignedSnapshotRequest
		errMsg        string
	}{
		{
			name:          "unmarshal error",
			signedRequest: &pb.SignedSnapshotRequest{Request: []byte("dummy")},
			errMsg:        "failed to unmarshal snapshot request",
		},
		{
			name:          "missing channel ID",
			channelID:     "",
			signedRequest: createSignedQuery(""),
			errMsg:        "missing channel ID",
		},
		{
			name:          "missing signature header",
			channelID:     "",
			signedRequest: &pb.SignedSnapshotRequest{Request: protoutil.MarshalOrPanic(&pb.SnapshotQuery{})},
			errMsg:        "missing signature header",
		},
		{
			name:          "cannot find ledger",
			channelID:     ledgerID,
			signedRequest: createSignedQuery(ledgerID),
			errMsg:        "cannot find ledger for channel " + ledgerID,
		},
	}

	fakeLedgerGetter.GetLedgerReturns(nil)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := snapshotSvc.Generate(context.Background(), test.signedRequest)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.errMsg)
			_, err = snapshotSvc.Cancel(context.Background(), test.signedRequest)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.errMsg)
			_, err = snapshotSvc.QueryPendings(context.Background(), test.signedRequest)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.errMsg)
		})
	}

	// test error propagation of CheckACLNoChannel
	fakeACLProvider.CheckACLNoChannelReturns(fmt.Errorf("fake-check-acl-error"))
	signedRequest = createSignedQuery(ledgerID)
	_, err = snapshotSvc.Generate(context.Background(), signedRequest)
	require.EqualError(t, err, "fake-check-acl-error")
	_, err = snapshotSvc.Cancel(context.Background(), signedRequest)
	require.EqualError(t, err, "fake-check-acl-error")
	_, err = snapshotSvc.QueryPendings(context.Background(), signedRequest)
	require.EqualError(t, err, "fake-check-acl-error")

	// verify panic from generate/cancel after ledger is closed
	lgr.Close()
	fakeLedgerGetter.GetLedgerReturns(lgr)
	fakeACLProvider.CheckACLNoChannelReturns(nil)

	require.PanicsWithError(
		t,
		"send on closed channel",
		func() { snapshotSvc.Generate(context.Background(), signedRequest) },
	)
	require.PanicsWithError(
		t,
		"send on closed channel",
		func() { snapshotSvc.Cancel(context.Background(), signedRequest) },
	)
}

func createSignedRequest(channelID string, blockNumber uint64) *pb.SignedSnapshotRequest {
	sigHeader := &common.SignatureHeader{
		Creator: []byte("creator"),
		Nonce:   []byte("nonce-ignored"),
	}
	request := &pb.SnapshotRequest{
		SignatureHeader: sigHeader,
		ChannelId:       channelID,
		BlockNumber:     blockNumber,
	}
	return &pb.SignedSnapshotRequest{
		Request:   protoutil.MarshalOrPanic(request),
		Signature: []byte("dummy-signatures"),
	}
}

func createSignedQuery(channelID string) *pb.SignedSnapshotRequest {
	sigHeader := &common.SignatureHeader{
		Creator: []byte("creator"),
		Nonce:   []byte("nonce-ignored"),
	}
	query := &pb.SnapshotQuery{
		SignatureHeader: sigHeader,
		ChannelId:       channelID,
	}
	return &pb.SignedSnapshotRequest{
		Request:   protoutil.MarshalOrPanic(query),
		Signature: []byte("dummy-signatures"),
	}
}
