/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

func TestCommitStatus(t *testing.T) {
	tests := []testDef{
		{
			name:      "error finding transaction status",
			finderErr: errors.New("FINDER_ERROR"),
			errCode:   codes.Aborted,
			errString: "FINDER_ERROR",
		},
		{
			name: "returns transaction status",
			finderStatus: &commit.Status{
				Code:        peer.TxValidationCode_MVCC_READ_CONFLICT,
				BlockNumber: 101,
			},
			expectedResponse: &pb.CommitStatusResponse{
				Result:      peer.TxValidationCode_MVCC_READ_CONFLICT,
				BlockNumber: 101,
			},
		},
		{
			name: "passes channel name to finder",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.finder.TransactionStatusCalls(func(ctx context.Context, channelName string, transactionID string) (*commit.Status, error) {
					require.Equal(t, testChannel, channelName)
					status := &commit.Status{
						Code:        peer.TxValidationCode_MVCC_READ_CONFLICT,
						BlockNumber: 101,
					}
					return status, nil
				})
			},
		},
		{
			name: "passes transaction ID to finder",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.finder.TransactionStatusCalls(func(ctx context.Context, channelName string, transactionID string) (*commit.Status, error) {
					require.Equal(t, "TX_ID", transactionID)
					status := &commit.Status{
						Code:        peer.TxValidationCode_MVCC_READ_CONFLICT,
						BlockNumber: 101,
					}
					return status, nil
				})
			},
		},
		{
			name:      "failed policy or signature check",
			policyErr: errors.New("POLICY_ERROR"),
			errCode:   codes.PermissionDenied,
			errString: "POLICY_ERROR",
		},
		{
			name: "passes channel name to policy checker",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.policy.CheckACLCalls(func(policyName string, channelName string, data interface{}) error {
					require.Equal(t, testChannel, channelName)
					return nil
				})
			},
			finderStatus: &commit.Status{
				Code:        peer.TxValidationCode_MVCC_READ_CONFLICT,
				BlockNumber: 101,
			},
		},
		{
			name:     "passes identity to policy checker",
			identity: []byte("IDENTITY"),
			postSetup: func(t *testing.T, test *preparedTest) {
				test.policy.CheckACLCalls(func(policyName string, channelName string, data interface{}) error {
					require.IsType(t, &protoutil.SignedData{}, data)
					signedData := data.(*protoutil.SignedData)
					require.Equal(t, []byte("IDENTITY"), signedData.Identity)
					return nil
				})
			},
			finderStatus: &commit.Status{
				Code:        peer.TxValidationCode_MVCC_READ_CONFLICT,
				BlockNumber: 101,
			},
		},
		{
			name:      "context timeout",
			finderErr: context.DeadlineExceeded,
			errCode:   codes.DeadlineExceeded,
			errString: "context deadline exceeded",
		},
		{
			name:      "context canceled",
			finderErr: context.Canceled,
			errCode:   codes.Canceled,
			errString: "context canceled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := prepareTest(t, &tt)

			request := &pb.CommitStatusRequest{
				ChannelId:     testChannel,
				Identity:      tt.identity,
				TransactionId: "TX_ID",
			}
			requestBytes, err := proto.Marshal(request)
			require.NoError(t, err)

			signedRequest := &pb.SignedCommitStatusRequest{
				Request:   requestBytes,
				Signature: []byte{},
			}

			response, err := test.server.CommitStatus(test.ctx, signedRequest)

			if checkError(t, &tt, err) {
				require.Nil(t, response, "response on error")
				return
			}

			require.NoError(t, err)
			if tt.expectedResponse != nil {
				require.True(t, proto.Equal(tt.expectedResponse, response), "incorrect response", response)
			}
		})
	}
}
