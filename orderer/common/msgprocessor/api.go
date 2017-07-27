/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package msgprocessor provides the implementations for processing of the assorted message
// types which may arrive in the system through Broadcast.
package msgprocessor

import (
	cb "github.com/hyperledger/fabric/protos/common"
)

// Classification represents the possible message types for the system.
type Classification int

const (
	// NormalMsg is the class of standard (endorser or otherwise non-config) messages.
	// Messages of this type should be processed by ProcessNormalMsg.
	NormalMsg Classification = iota

	// ConfigUpdateMsg is the class of configuration related messages.
	// Messages of this type should be processed by ProcessConfigUpdateMsg.
	ConfigUpdateMsg
)

// Processor provides the methods necessary to classify and process any message which
// arrives through the Broadcast interface.
type Processor interface {
	// ClassifyMsg inspects the message to determine which type of processing is necessary
	ClassifyMsg(env *cb.Envelope) (Classification, error)

	// ProcessNormalMsg will check the validity of a message based on the current configuration.  It returns the current
	// configuration sequence number and nil on success, or an error if the message is not valid
	ProcessNormalMsg(env *cb.Envelope) (configSeq uint64, err error)

	// ProcessConfigUpdateMsg will attempt to apply the config update to the current configuration, and if successful
	// return the resulting config message and the configSeq the config was computed from.  If the config update message
	// is invalid, an error is returned.
	ProcessConfigUpdateMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error)
}
