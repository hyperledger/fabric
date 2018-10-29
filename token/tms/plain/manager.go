/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"sync"

	"github.com/hyperledger/fabric/token/identity"
	"github.com/hyperledger/fabric/token/transaction"
	"github.com/pkg/errors"
)

// Manager is used to access TMS components.
type Manager struct {
	mutex            sync.RWMutex
	policyValidators map[string]identity.IssuingValidator
}

// GetTxProcessor returns a TMSTxProcessor that is used to process token transactions.
func (m *Manager) GetTxProcessor(channel string) (transaction.TMSTxProcessor, error) {
	m.mutex.RLock()
	policyValidator := m.policyValidators[channel]
	m.mutex.RUnlock()
	if policyValidator == nil {
		return nil, errors.Errorf("no policy validator found for channel '%s'", channel)
	}
	return &Verifier{IssuingValidator: policyValidator}, nil
}

// SetPolicyValidator sets the policy validator for the specified channel
func (m *Manager) SetPolicyValidator(channel string, validator identity.IssuingValidator) {
	m.mutex.Lock()
	m.policyValidators[channel] = validator
	m.mutex.Unlock()
}
