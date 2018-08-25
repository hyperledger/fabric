/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cauthdsl

import (
	"fmt"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	mb "github.com/hyperledger/fabric/protos/msp"
	"go.uber.org/zap/zapcore"
)

var cauthdslLogger = flogging.MustGetLogger("cauthdsl")

// deduplicate removes any duplicated identities while otherwise preserving identity order
func deduplicate(sds []*cb.SignedData, deserializer msp.IdentityDeserializer) []*cb.SignedData {
	ids := make(map[string]struct{})
	result := make([]*cb.SignedData, 0, len(sds))
	for i, sd := range sds {
		identity, err := deserializer.DeserializeIdentity(sd.Identity)
		if err != nil {
			cauthdslLogger.Errorf("Principal deserialization failure (%s) for identity %x", err, sd.Identity)
			continue
		}
		key := identity.GetIdentifier().Mspid + identity.GetIdentifier().Id

		if _, ok := ids[key]; ok {
			cauthdslLogger.Warningf("De-duplicating identity %x at index %d in signature set", sd.Identity, i)
		} else {
			result = append(result, sd)
			ids[key] = struct{}{}
		}
	}
	return result
}

// compile recursively builds a go evaluatable function corresponding to the policy specified, remember to call deduplicate on identities before
// passing them to this function for evaluation
func compile(policy *cb.SignaturePolicy, identities []*mb.MSPPrincipal, deserializer msp.IdentityDeserializer) (func([]*cb.SignedData, []bool) bool, error) {
	if policy == nil {
		return nil, fmt.Errorf("Empty policy element")
	}

	switch t := policy.Type.(type) {
	case *cb.SignaturePolicy_NOutOf_:
		policies := make([]func([]*cb.SignedData, []bool) bool, len(t.NOutOf.Rules))
		for i, policy := range t.NOutOf.Rules {
			compiledPolicy, err := compile(policy, identities, deserializer)
			if err != nil {
				return nil, err
			}
			policies[i] = compiledPolicy

		}
		return func(signedData []*cb.SignedData, used []bool) bool {
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
			return nil, fmt.Errorf("identity index out of range, requested %v, but identies length is %d", t.SignedBy, len(identities))
		}
		signedByID := identities[t.SignedBy]
		return func(signedData []*cb.SignedData, used []bool) bool {
			cauthdslLogger.Debugf("%p signed by %d principal evaluation starts (used %v)", signedData, t.SignedBy, used)
			for i, sd := range signedData {
				if used[i] {
					cauthdslLogger.Debugf("%p skipping identity %d because it has already been used", signedData, i)
					continue
				}
				if cauthdslLogger.IsEnabledFor(zapcore.DebugLevel) {
					// Unlike most places, this is a huge print statement, and worth checking log level before create garbage
					cauthdslLogger.Debugf("%p processing identity %d with bytes of %x", signedData, i, sd.Identity)
				}
				identity, err := deserializer.DeserializeIdentity(sd.Identity)
				if err != nil {
					cauthdslLogger.Errorf("Principal deserialization failure (%s) for identity %x", err, sd.Identity)
					continue
				}
				err = identity.SatisfiesPrincipal(signedByID)
				if err != nil {
					cauthdslLogger.Debugf("%p identity %d does not satisfy principal: %s", signedData, i, err)
					continue
				}
				cauthdslLogger.Debugf("%p principal matched by identity %d", signedData, i)
				err = identity.Verify(sd.Data, sd.Signature)
				if err != nil {
					cauthdslLogger.Debugf("%p signature for identity %d is invalid: %s", signedData, i, err)
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
