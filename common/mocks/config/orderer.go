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
	// ConsensusMetadataVal is returned as the result of ConsensusMetadata()
	ConsensusMetadataVal []byte
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
func (o *Orderer) ConsensusType() string {
	return o.ConsensusTypeVal
}

// ConsensusMetadata returns the ConsensusMetadataVal
func (o *Orderer) ConsensusMetadata() []byte {
	return o.ConsensusMetadataVal
}

// BatchSize returns the BatchSizeVal
func (o *Orderer) BatchSize() *ab.BatchSize {
	return o.BatchSizeVal
}

// BatchTimeout returns the BatchTimeoutVal
func (o *Orderer) BatchTimeout() time.Duration {
	return o.BatchTimeoutVal
}

// KafkaBrokers returns the KafkaBrokersVal
func (o *Orderer) KafkaBrokers() []string {
	return o.KafkaBrokersVal
}

// MaxChannelsCount returns the MaxChannelsCountVal
func (o *Orderer) MaxChannelsCount() uint64 {
	return o.MaxChannelsCountVal
}

// Organizations returns OrganizationsVal
func (o *Orderer) Organizations() map[string]channelconfig.Org {
	return o.OrganizationsVal
}

// Capabilities returns CapabilitiesVal
func (o *Orderer) Capabilities() channelconfig.OrdererCapabilities {
	return o.CapabilitiesVal
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
