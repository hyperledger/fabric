/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/hyperledger/fabric/protos/common"
)

// PolicyEvaluator evaluates policies
type PolicyEvaluator interface {
	validation.Dependency

	// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies
	// the policy with the given bytes
	Evaluate(policyBytes []byte, signatureSet []*common.SignedData) error
}

// SerializedPolicy defines a serialized policy
type SerializedPolicy interface {
	validation.ContextDatum

	// Bytes returns the bytes of the SerializedPolicy
	Bytes() []byte
}
