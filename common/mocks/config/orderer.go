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

package config

import (
	"time"

	"github.com/hyperledger/fabric/common/config"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

// Orderer is a mock implementation of config.Orderer
type Orderer struct {
	// ConsensusTypeVal is returned as the result of ConsensusType()
	ConsensusTypeVal string
	// BatchSizeVal is returned as the result of BatchSize()
	BatchSizeVal *ab.BatchSize
	// BatchTimeoutVal is returned as the result of BatchTimeout()
	BatchTimeoutVal time.Duration
	// KafkaBrokersVal is returned as the result of KafkaBrokers()
	KafkaBrokersVal []string
	// MaxChannelsCountVal is returns as the result of MaxChannelsCount()
	MaxChannelsCountVal uint64
	// OrganizationsVal is returned as the result of Organizations()
	OrganizationsVal map[string]config.Org
}

// ConsensusType returns the ConsensusTypeVal
func (scm *Orderer) ConsensusType() string {
	return scm.ConsensusTypeVal
}

// BatchSize returns the BatchSizeVal
func (scm *Orderer) BatchSize() *ab.BatchSize {
	return scm.BatchSizeVal
}

// BatchTimeout returns the BatchTimeoutVal
func (scm *Orderer) BatchTimeout() time.Duration {
	return scm.BatchTimeoutVal
}

// KafkaBrokers returns the KafkaBrokersVal
func (scm *Orderer) KafkaBrokers() []string {
	return scm.KafkaBrokersVal
}

// MaxChannelsCount returns the MaxChannelsCountVal
func (scm *Orderer) MaxChannelsCount() uint64 {
	return scm.MaxChannelsCountVal
}

// Organizations returns OrganizationsVal
func (scm *Orderer) Organizations() map[string]config.Org {
	return scm.OrganizationsVal
}
