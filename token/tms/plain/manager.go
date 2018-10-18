/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"sync"

	"github.com/hyperledger/fabric/token/transaction"
	"github.com/pkg/errors"
)

// Manager is used to access TMS components.
type Manager struct {
	mutex            sync.RWMutex
	PolicyValidators map[string]transaction.PolicyValidator
}

// GetTxProcessor returns a TMSTxProcessor that is used to process token transactions.
func (m *Manager) GetTxProcessor(channel string) (transaction.TMSTxProcessor, error) {
	m.mutex.RLock()
	policyValidator := m.PolicyValidators[channel]
	m.mutex.RUnlock()
	if policyValidator == nil {
		return nil, errors.Errorf("no policy validator found for channel '%s'", channel)
	}
	return &Verifier{PolicyValidator: policyValidator}, nil
}

// SetPolicyValidator sets the policy validator for the specified channel
func (m *Manager) SetPolicyValidator(channel string, validator transaction.PolicyValidator) {
	m.mutex.Lock()
	m.PolicyValidators[channel] = validator
	m.mutex.Unlock()
}
