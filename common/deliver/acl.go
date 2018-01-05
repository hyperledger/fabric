/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver

import (
	"time"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
)

type expiresAtFunc func(identityBytes []byte) time.Time

// acSupport provides the backing resources needed to support access control validation
type acSupport interface {
	// Sequence returns the current config sequence number, can be used to detect config changes
	Sequence() uint64
}

func newSessionAC(sup acSupport, env *common.Envelope, poliyChecker PolicyChecker, channel string, expiresAt expiresAtFunc) (*sessionAC, error) {
	signedData, err := env.AsSignedData()
	if err != nil {
		return nil, err
	}

	return &sessionAC{
		env:            env,
		channel:        channel,
		acSupport:      sup,
		checkPolicy:    poliyChecker,
		sessionEndTime: expiresAt(signedData[0].Identity),
	}, nil
}

type sessionAC struct {
	acSupport
	checkPolicy        PolicyChecker
	channel            string
	env                *common.Envelope
	lastConfigSequence uint64
	sessionEndTime     time.Time
	usedAtLeastOnce    bool
}

func (ac *sessionAC) evaluate() error {
	if !ac.sessionEndTime.IsZero() && time.Now().After(ac.sessionEndTime) {
		return errors.Errorf("client identity expired %v before", time.Since(ac.sessionEndTime))
	}

	policyCheckNeeded := !ac.usedAtLeastOnce

	if currentConfigSequence := ac.Sequence(); currentConfigSequence > ac.lastConfigSequence {
		ac.lastConfigSequence = currentConfigSequence
		policyCheckNeeded = true
	}

	if !policyCheckNeeded {
		return nil
	}

	ac.usedAtLeastOnce = true
	return ac.checkPolicy(ac.env, ac.channel)

}
