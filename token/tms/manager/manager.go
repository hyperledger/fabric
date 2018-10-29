/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager

import (
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/token/identity"
	"github.com/hyperledger/fabric/token/tms/plain"
	"github.com/hyperledger/fabric/token/transaction"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mock/identity_deserializer_manager.go -fake-name DeserializerManager . DeserializerManager

// FabricIdentityDeserializerManager implements an DeserializerManager
// by routing the call to the msp/mgmt package
type FabricIdentityDeserializerManager struct {
}

func (*FabricIdentityDeserializerManager) Deserializer(channel string) (identity.Deserializer, error) {
	id, ok := mgmt.GetDeserializers()[channel]
	if !ok {
		return nil, errors.New("channel not found")
	}
	return id, nil
}

// Manager is used to access TMS components.
type Manager struct {
	IdentityDeserializerManager identity.DeserializerManager
}

// GetTxProcessor returns a TMSTxProcessor that is used to process token transactions.
func (m *Manager) GetTxProcessor(channel string) (transaction.TMSTxProcessor, error) {
	identityDeserializerManager, err := m.IdentityDeserializerManager.Deserializer(channel)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting identity deserialiser manager for channel '%s'", channel)
	}

	return &plain.Verifier{IssuingValidator: &AllIssuingValidator{Deserializer: identityDeserializerManager}}, nil
}
