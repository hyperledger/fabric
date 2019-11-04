/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cauthdsl

import (
	"fmt"
	"time"

	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/msp"
	"go.uber.org/zap/zapcore"
)

var cauthdslLogger = flogging.MustGetLogger("cauthdsl")

// compile recursively builds a go evaluatable function corresponding to the policy specified, remember to call deduplicate on identities before
// passing them to this function for evaluation
func compile(policy *cb.SignaturePolicy, identities []*mb.MSPPrincipal) (func([]msp.Identity, []bool) bool, error) {
	if policy == nil {
		return nil, fmt.Errorf("Empty policy element")
	}

	switch t := policy.Type.(type) {
	case *cb.SignaturePolicy_NOutOf_:
		policies := make([]func([]msp.Identity, []bool) bool, len(t.NOutOf.Rules))
		for i, policy := range t.NOutOf.Rules {
			compiledPolicy, err := compile(policy, identities)
			if err != nil {
				return nil, err
			}
			policies[i] = compiledPolicy

		}
		return func(signedData []msp.Identity, used []bool) bool {
			grepKey := time.Now().UnixNano()
			cauthdslLogger.Debugf("%p gate %d evaluation starts", signedData, grepKey)
			verified := int32(0)
			_used := make([]bool, len(used))
			for _, policy := range policies {
				copy(_used, used)
				if policy(signedData, _used) {
					verified++
					copy(used, _used)
				}
			}

			if verified >= t.NOutOf.N {
				cauthdslLogger.Debugf("%p gate %d evaluation succeeds", signedData, grepKey)
			} else {
				cauthdslLogger.Debugf("%p gate %d evaluation fails", signedData, grepKey)
			}

			return verified >= t.NOutOf.N
		}, nil
	case *cb.SignaturePolicy_SignedBy:
		if t.SignedBy < 0 || t.SignedBy >= int32(len(identities)) {
			return nil, fmt.Errorf("identity index out of range, requested %v, but identities length is %d", t.SignedBy, len(identities))
		}
		signedByID := identities[t.SignedBy]
		return func(signedData []msp.Identity, used []bool) bool {
			cauthdslLogger.Debugf("%p signed by %d principal evaluation starts (used %v)", signedData, t.SignedBy, used)
			for i, sd := range signedData {
				if used[i] {
					cauthdslLogger.Debugf("%p skipping identity %d because it has already been used", signedData, i)
					continue
				}
				if cauthdslLogger.IsEnabledFor(zapcore.DebugLevel) {
					// Unlike most places, this is a huge print statement, and worth checking log level before create garbage
					cauthdslLogger.Debugf("%p processing identity %d - %v", signedData, i, sd.GetIdentifier())
				}
				err := sd.SatisfiesPrincipal(signedByID)
				if err != nil {
					cauthdslLogger.Debugf("%p identity %d does not satisfy principal: %s", signedData, i, err)
					continue
				}
				cauthdslLogger.Debugf("%p principal evaluation succeeds for identity %d", signedData, i)
				used[i] = true
				return true
			}
			cauthdslLogger.Debugf("%p principal evaluation fails", signedData)
			return false
		}, nil
	default:
		return nil, fmt.Errorf("Unknown type: %T:%v", t, t)
	}
}
