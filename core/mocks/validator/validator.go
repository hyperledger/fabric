/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validator

import (
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
)

// MockValidator implements a mock validation useful for testing
type MockValidator struct {
}

// Validate does nothing, returning no error
func (m *MockValidator) Validate(block *common.Block) error {
	return nil
}

// MockVsccValidator is a mock implementation of the VSCC validation interface
type MockVsccValidator struct {
}

// VSCCValidateTx does nothing
func (v *MockVsccValidator) VSCCValidateTx(payload *common.Payload, envBytes []byte, env *common.Envelope) (error, peer.TxValidationCode) {
	return nil, peer.TxValidationCode_VALID
}
