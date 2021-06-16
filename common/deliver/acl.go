/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver

import (
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// ExpiresAtFunc is used to extract the time at which an identity expires.
type ExpiresAtFunc func(identityBytes []byte) time.Time

// ConfigSequencer provides the sequence number of the current config block.
type ConfigSequencer interface {
	Sequence() uint64
}

// NewSessionAC creates an instance of SessionAccessControl. This constructor will
// return an error if a signature header cannot be extracted from the envelope.
func NewSessionAC(chain ConfigSequencer, env *common.Envelope, policyChecker PolicyChecker, channelID string, expiresAt ExpiresAtFunc) (*SessionAccessControl, error) {
	signedData, err := protoutil.EnvelopeAsSignedData(env)
	if err != nil {
		return nil, err
	}

	return &SessionAccessControl{
		envelope:       env,
		channelID:      channelID,
		sequencer:      chain,
		policyChecker:  policyChecker,
		sessionEndTime: expiresAt(signedData[0].Identity),
	}, nil
}

// SessionAccessControl holds access control related data for a common Envelope
// that is used to determine if a request is allowed for the identity
// associated with the request envelope.
type SessionAccessControl struct {
	sequencer          ConfigSequencer
	policyChecker      PolicyChecker
	channelID          string
	envelope           *common.Envelope
	lastConfigSequence uint64
	sessionEndTime     time.Time
	usedAtLeastOnce    bool
}

// Evaluate uses the PolicyChecker to determine if a request should be allowed.
// The decision is cached until the identity expires or the chain configuration
// changes.
func (ac *SessionAccessControl) Evaluate() error {
	if !ac.sessionEndTime.IsZero() && time.Now().After(ac.sessionEndTime) {
		return errors.Errorf("deliver client identity expired %v before", time.Since(ac.sessionEndTime))
	}

	policyCheckNeeded := !ac.usedAtLeastOnce

	if currentConfigSequence := ac.sequencer.Sequence(); currentConfigSequence > ac.lastConfigSequence {
		ac.lastConfigSequence = currentConfigSequence
		policyCheckNeeded = true
	}

	if !policyCheckNeeded {
		return nil
	}

	ac.usedAtLeastOnce = true
	return ac.policyChecker.CheckPolicy(ac.envelope, ac.channelID)
}
