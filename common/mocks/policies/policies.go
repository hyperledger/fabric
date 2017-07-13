/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package policies

import (
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
)

// Policy is a mock implementation of the policies.Policy interface
type Policy struct {
	// Err is the error returned by Evaluate
	Err error
}

// Evaluate returns the Err set in Policy
func (p *Policy) Evaluate(signatureSet []*cb.SignedData) error {
	return p.Err
}

// Manager is a mock implementation of the policies.Manager interface
type Manager struct {
	// Policy is returned as the output to GetPolicy if a Policy
	// for id is not in PolicyMap
	Policy *Policy

	// BasePathVal is returned as the result of BasePath
	BasePathVal string

	// PolicyMap is returned is used to look up Policies in
	PolicyMap map[string]policies.Policy

	// SubManagers is used for the return value of Manager
	SubManagersMap map[string]*Manager
}

// PolicyNames panics
func (m *Manager) PolicyNames() []string {
	panic("Unimplimented")
}

// BasePath returns BasePathVal
func (m *Manager) BasePath() string {
	return m.BasePathVal
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
