/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"time"

	"github.com/hyperledger/fabric/common/channelconfig"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

// Orderer is a mock implementation of channelconfig.Orderer
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
	OrganizationsVal map[string]channelconfig.Org
	// CapabilitiesVal is returned as the result of Capabilities()
	CapabilitiesVal channelconfig.OrdererCapabilities
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
func (scm *Orderer) Organizations() map[string]channelconfig.Org {
	return scm.OrganizationsVal
}

// Capabilities returns CapabilitiesVal
func (scm *Orderer) Capabilities() channelconfig.OrdererCapabilities {
	return scm.CapabilitiesVal
}

// OrdererCapabilities mocks the channelconfig.OrdererCapabilities interface
type OrdererCapabilities struct {
	// SupportedErr is returned by Supported()
	SupportedErr error

	// PredictableChannelTemplateVal is returned by PredictableChannelTemplate()
	PredictableChannelTemplateVal bool

	// ResubmissionVal is returned by Resubmission()
	ResubmissionVal bool

	// ExpirationVal is returned by ExpirationCheck()
	ExpirationVal bool
}

// Supported returns SupportedErr
func (oc *OrdererCapabilities) Supported() error {
	return oc.SupportedErr
}

// PredictableChannelTemplate returns PredictableChannelTemplateVal
func (oc *OrdererCapabilities) PredictableChannelTemplate() bool {
	return oc.PredictableChannelTemplateVal
}

// Resubmission returns ResubmissionVal
func (oc *OrdererCapabilities) Resubmission() bool {
	return oc.ResubmissionVal
}

// ExpirationCheck specifies whether the orderer checks for identity expiration checks
// when validating messages
func (oc *OrdererCapabilities) ExpirationCheck() bool {
	return oc.ExpirationVal
}
