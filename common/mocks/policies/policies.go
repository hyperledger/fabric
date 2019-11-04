/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
)

// Policy is a mock implementation of the policies.Policy interface
type Policy struct {
	// Err is the error returned by Evaluate
	Err error
}

// EvaluateSignedData returns the Err set in Policy
func (p *Policy) EvaluateSignedData(signatureSet []*protoutil.SignedData) error {
	return p.Err
}

// EvaluateIdentities returns nil
func (p *Policy) EvaluateIdentities(ids []msp.Identity) error {
	return p.Err
}

// Manager is a mock implementation of the policies.Manager interface
type Manager struct {
	// Policy is returned as the output to GetPolicy if a Policy
	// for id is not in PolicyMap
	Policy *Policy

	// PolicyMap is returned is used to look up Policies in
	PolicyMap map[string]policies.Policy

	// SubManagers is used for the return value of Manager
	SubManagersMap map[string]*Manager
}

// Manager returns the Manager from SubManagers for the last component of the path
func (m *Manager) Manager(path []string) (policies.Manager, bool) {
	if len(path) == 0 {
		return m, true
	}
	manager, ok := m.SubManagersMap[path[len(path)-1]]
	return manager, ok
}

// GetPolicy returns the value of Manager.Policy and whether it was nil or not
func (m *Manager) GetPolicy(id string) (policies.Policy, bool) {
	if m.PolicyMap != nil {
		policy, ok := m.PolicyMap[id]
		if ok {
			return policy, true
		}
	}
	return m.Policy, m.Policy != nil
}
