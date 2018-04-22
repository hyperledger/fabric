/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validator

import (
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/mock"
)

// MockValidator implements a mock validation useful for testing
type MockValidator struct {
	mock.Mock
}

// Validate does nothing, returning no error
func (m *MockValidator) Validate(block *common.Block) error {
	if len(m.ExpectedCalls) == 0 {
		return nil
	}
	return m.Called().Error(0)
}

// MockVsccValidator is a mock implementation of the VSCC validation interface
type MockVsccValidator struct {
}

// VSCCValidateTx does nothing
func (v *MockVsccValidator) VSCCValidateTx(seq int, payload *common.Payload, envBytes []byte, block *common.Block) (error, peer.TxValidationCode) {
	return nil, peer.TxValidationCode_VALID
}
