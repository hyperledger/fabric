/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"testing"

	"github.com/hyperledger/SmartBFT/pkg/types"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestValidateTransactionsRejectsConfigTransactionWithOthers(t *testing.T) {
	verifyReq := func([]byte, bool) (types.RequestInfo, error) {
		return types.RequestInfo{}, nil
	}

	_, err := validateTransactions([][]byte{
		makeTestEnvelope(cb.HeaderType_ENDORSER_TRANSACTION),
		makeTestEnvelope(cb.HeaderType_CONFIG),
	}, verifyReq)
	require.EqualError(t, err, "transaction 1 is a config transaction but the block contains 2 transactions")
}

func TestValidateTransactionsAcceptsSoleConfigTransaction(t *testing.T) {
	verifyReq := func([]byte, bool) (types.RequestInfo, error) {
		return types.RequestInfo{}, nil
	}

	reqInfo, err := validateTransactions([][]byte{
		makeTestEnvelope(cb.HeaderType_CONFIG),
	}, verifyReq)
	require.NoError(t, err)
	require.Len(t, reqInfo, 1)
}

func makeTestEnvelope(headerType cb.HeaderType) []byte {
	return protoutil.MarshalOrPanic(&cb.Envelope{
		Signature: []byte{1, 2, 3},
		Payload: protoutil.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
					Type: int32(headerType),
				}),
			},
		}),
	})
}
