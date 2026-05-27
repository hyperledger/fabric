/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"bytes"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
)

type comparable struct {
	*cb.ConfigGroup
	*cb.ConfigValue
	*cb.ConfigPolicy
	key  string
	path []string
}

func (cg comparable) equals(other comparable) bool {
	switch {
	case cg.ConfigGroup != nil:
		if other.ConfigGroup == nil {
			return false
		}
		return equalConfigGroup(cg.ConfigGroup, other.ConfigGroup)
	case cg.ConfigValue != nil:
		if other.ConfigValue == nil {
			return false
		}
		return equalConfigValues(cg.ConfigValue, other.ConfigValue)
	case cg.ConfigPolicy != nil:
		if other.ConfigPolicy == nil {
			return false
		}
		return equalConfigPolicies(cg.ConfigPolicy, other.ConfigPolicy)
	}

	// Unreachable
	return false
}

func (cg comparable) version() uint64 {
	switch {
	case cg.ConfigGroup != nil:
		return cg.ConfigGroup.Version
	case cg.ConfigValue != nil:
		return cg.ConfigValue.Version
	case cg.ConfigPolicy != nil:
		return cg.ConfigPolicy.Version
	}

	// Unreachable
	return 0
}

func (cg comparable) modPolicy() string {
	switch {
	case cg.ConfigGroup != nil:
		return cg.ConfigGroup.ModPolicy
	case cg.ConfigValue != nil:
		return cg.ConfigValue.ModPolicy
	case cg.ConfigPolicy != nil:
		return cg.ConfigPolicy.ModPolicy
	}

	// Unreachable
	return ""
}

func equalConfigValues(lhs, rhs *cb.ConfigValue) bool {
	return lhs.Version == rhs.Version &&
		lhs.ModPolicy == rhs.ModPolicy &&
		bytes.Equal(lhs.Value, rhs.Value)
}

func equalConfigPolicies(lhs, rhs *cb.ConfigPolicy) bool {
	if lhs.Version != rhs.Version ||
		lhs.ModPolicy != rhs.ModPolicy {
		return false
	}

	if lhs.Policy == nil || rhs.Policy == nil {
		return lhs.Policy == rhs.Policy
	}

	return lhs.Policy.Type == rhs.Policy.Type &&
		bytes.Equal(lhs.Policy.Value, rhs.Policy.Value)
}

// subsetOf checks if every key in inner is also present in outer.
func subsetOf[V any](inner, outer map[string]V) bool {
	if len(inner) > len(outer) {
		return false
	}
	for key := range inner {
		if _, ok := outer[key]; !ok {
			return false
		}
	}
	return true
}

func equalConfigGroup(lhs, rhs *cb.ConfigGroup) bool {
	if lhs.Version != rhs.Version ||
		lhs.ModPolicy != rhs.ModPolicy {
		return false
	}

	if !subsetOf(lhs.Groups, rhs.Groups) ||
		!subsetOf(rhs.Groups, lhs.Groups) ||
		!subsetOf(lhs.Policies, rhs.Policies) ||
		!subsetOf(rhs.Policies, lhs.Policies) ||
		!subsetOf(lhs.Values, rhs.Values) ||
		!subsetOf(rhs.Values, lhs.Values) {
		return false
	}

	return true
}
