package validator

import "github.com/hyperledger/fabric/protos/common"

// MockValidator implements a mock validation useful for testing
type MockValidator struct {
}

// Validate does nothing
func (m *MockValidator) Validate(block *common.Block) {
}

// MockVsccValidator is a mock implementation of the VSCC validation interface
type MockVsccValidator struct {
}

// VSCCValidateTx does nothing
func (v *MockVsccValidator) VSCCValidateTx(payload *common.Payload, envBytes []byte) error {
	return nil
}
