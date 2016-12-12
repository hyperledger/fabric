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

package mocks

// MockSharedConfigManager is a mock implementation of sharedconfig.Manager
type SharedConfigManager struct {
	// ConsensusTypeVal is returned as the result of ConsensusType()
	ConsensusTypeVal string
	// BatchSizeVal is returned as the result of BatchSize()
	BatchSizeVal int
	// ChainCreatorsVal is returned as the result of ChainCreators()
	ChainCreatorsVal []string
}

// ConsensusType returns the configured consensus type
func (scm *SharedConfigManager) ConsensusType() string {
	return scm.ConsensusTypeVal
}

// BatchSize returns the maximum number of messages to include in a block
func (scm *SharedConfigManager) BatchSize() int {
	return scm.BatchSizeVal
}

// ChainCreators returns the policy names which are allowed for chain creation
// This field is only set for the system ordering chain
func (scm *SharedConfigManager) ChainCreators() []string {
	return scm.ChainCreatorsVal
}
