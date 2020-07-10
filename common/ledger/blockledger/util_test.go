/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blockledger_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/deliver/mock"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/stretchr/testify/require"
)

func TestClose(t *testing.T) {
	for _, testCase := range []struct {
		name               string
		status             common.Status
		isIteratorNil      bool
		expectedCloseCount int
	}{
		{
			name:          "nil iterator",
			isIteratorNil: true,
		},
		{
			name:               "Next() fails",
			status:             common.Status_INTERNAL_SERVER_ERROR,
			expectedCloseCount: 1,
		},
		{
			name:               "Next() succeeds",
			status:             common.Status_SUCCESS,
			expectedCloseCount: 1,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var iterator *mock.BlockIterator
			reader := &mock.BlockReader{}
			if !testCase.isIteratorNil {
				iterator = &mock.BlockIterator{}
				iterator.NextReturns(&common.Block{}, testCase.status)
				reader.IteratorReturns(iterator, 1)
			}

			blockledger.GetBlock(reader, 1)
			if !testCase.isIteratorNil {
				require.Equal(t, testCase.expectedCloseCount, iterator.CloseCallCount())
			}
		})
	}
}
