/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package snapshotgrpc

import (
	"context"
	"io/ioutil"
	"testing"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt/ledgermgmttest"
	"github.com/hyperledger/fabric/core/ledger/snapshotgrpc/mock"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mock/ledger_getter.go -fake-name LedgerGetter . ledgerGetter

type ledgerGetter interface {
	LedgerGetter
}

func TestSnapshot(t *testing.T) {
	testDir, err := ioutil.TempDir("", "snapshotgrpc")
	if err != nil {
		t.Fatalf("Failed to create ledger directory: %s", err)
	}

	ledgerID := "testsnapshot"
	ledgermgmtInitializer := ledgermgmttest.NewInitializer(testDir)
	ledgerMgr := ledgermgmt.NewLedgerMgr(ledgermgmtInitializer)
	gb, err := test.MakeGenesisBlock(ledgerID)
	require.NoError(t, err)
	lgr, err := ledgerMgr.CreateLedger(ledgerID, gb)
	defer lgr.Close()
	require.NoError(t, err)

	fakeLedgerGetter := &mock.LedgerGetter{}
	fakeLedgerGetter.GetLedgerReturns(lgr)
	snapshotSvc := &SnapshotService{fakeLedgerGetter}

	// test generate, cancel and query bindings
	var signedRequest *pb.SignedSnapshotRequest
	testData := []uint64{100, 50, 200}
	for _, height := range testData {
		signedRequest = createSignedRequest(ledgerID, height)
		_, err = snapshotSvc.Generate(context.Background(), signedRequest)
		require.NoError(t, err)
	}

	signedRequest = createSignedRequest(ledgerID, 100)
	_, err = snapshotSvc.Cancel(context.Background(), signedRequest)
	require.NoError(t, err)

	expectedResponse := &pb.QueryPendingSnapshotsResponse{Heights: []uint64{50, 200}}
	signedQuery := createSignedQuery(ledgerID)
	resp, err := snapshotSvc.QueryPendings(context.Background(), signedQuery)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, resp)

	// test error propagation from ledger, generate height=50 should return an error
	signedRequest = createSignedRequest(ledgerID, 50)
	_, err = snapshotSvc.Generate(context.Background(), signedRequest)
	require.EqualError(t, err, "duplicate snapshot request for height 50")

	// test error propagation from ledger, cancel height=100 again should return an error
	signedRequest = createSignedRequest(ledgerID, 100)
	_, err = snapshotSvc.Cancel(context.Background(), signedRequest)
	require.EqualError(t, err, "no snapshot request exists for height 100")

	// common error tests for all requests
	var tests = []struct {
		name          string
		channelID     string
		signedRequest *pb.SignedSnapshotRequest
		errMsg        string
	}{
		{
			name:          "unmarshal error",
			signedRequest: &pb.SignedSnapshotRequest{Request: []byte("dummy")},
			errMsg:        "failed to unmarshal snapshot request: proto: can't skip unknown wire type 4",
		},
		{
			name:      "missing channel ID",
			channelID: "",
			errMsg:    "missing channel ID",
		},
		{
			name:      "cannot find ledger",
			channelID: ledgerID,
			errMsg:    "cannot find ledger for channel " + ledgerID,
		},
	}

	fakeLedgerGetter.GetLedgerReturns(nil)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			signedRequest = test.signedRequest
			if signedRequest == nil {
				// create a SignedQuery for all requests because height does not matter in these error tests
				signedRequest = createSignedQuery(test.channelID)
			}
			_, err := snapshotSvc.Generate(context.Background(), signedRequest)
			require.EqualError(t, err, test.errMsg)
			_, err = snapshotSvc.Cancel(context.Background(), signedRequest)
			require.EqualError(t, err, test.errMsg)
			_, err = snapshotSvc.QueryPendings(context.Background(), signedRequest)
			require.EqualError(t, err, test.errMsg)
		})
	}

	// verify panic from generate/cancel after ledger is closed
	lgr.Close()
	fakeLedgerGetter.GetLedgerReturns(lgr)

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

func createSignedRequest(channelID string, height uint64) *pb.SignedSnapshotRequest {
	request := &pb.SnapshotRequest{
		ChannelId: channelID,
		Height:    height,
	}
	return &pb.SignedSnapshotRequest{
		Request: protoutil.MarshalOrPanic(request),
	}
}

func createSignedQuery(channelID string) *pb.SignedSnapshotRequest {
	query := &pb.SnapshotQuery{
		ChannelId: channelID,
	}
	return &pb.SignedSnapshotRequest{
		Request: protoutil.MarshalOrPanic(query),
	}
}
