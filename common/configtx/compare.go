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
		return cg.ConfigGroup.GetVersion()
	case cg.ConfigValue != nil:
		return cg.ConfigValue.GetVersion()
	case cg.ConfigPolicy != nil:
		return cg.ConfigPolicy.GetVersion()
	}

	// Unreachable
	return 0
}

func (cg comparable) modPolicy() string {
	switch {
	case cg.ConfigGroup != nil:
		return cg.ConfigGroup.GetModPolicy()
	case cg.ConfigValue != nil:
		return cg.ConfigValue.GetModPolicy()
	case cg.ConfigPolicy != nil:
		return cg.ConfigPolicy.GetModPolicy()
	}

	// Unreachable
	return ""
}

func equalConfigValues(lhs, rhs *cb.ConfigValue) bool {
	return lhs.GetVersion() == rhs.GetVersion() &&
		lhs.GetModPolicy() == rhs.GetModPolicy() &&
		bytes.Equal(lhs.GetValue(), rhs.GetValue())
}

func equalConfigPolicies(lhs, rhs *cb.ConfigPolicy) bool {
	if lhs.GetVersion() != rhs.GetVersion() ||
		lhs.GetModPolicy() != rhs.GetModPolicy() {
		return false
	}

	if lhs.GetPolicy() == nil || rhs.GetPolicy() == nil {
		return lhs.GetPolicy() == rhs.GetPolicy()
	}

	return lhs.GetPolicy().GetType() == rhs.GetPolicy().GetType() &&
		bytes.Equal(lhs.GetPolicy().GetValue(), rhs.GetPolicy().GetValue())
}

// The subset functions check if inner is a subset of outer
// TODO, try to consolidate these three methods into one, as the code
// contents are the same, but the function signatures need to be different
func subsetOfGroups(inner, outer map[string]*cb.ConfigGroup) bool {
	// The empty set is a subset of all sets
	if len(inner) == 0 {
		return true
	}

	// If inner has more elements than outer, it cannot be a subset
	if len(inner) > len(outer) {
		return false
	}

	// If any element in inner is not in outer, it is not a subset
	for key := range inner {
		if _, ok := outer[key]; !ok {
			return false
		}
	}

	return true
}

func subsetOfPolicies(inner, outer map[string]*cb.ConfigPolicy) bool {
	// The empty set is a subset of all sets
	if len(inner) == 0 {
		return true
	}

	// If inner has more elements than outer, it cannot be a subset
	if len(inner) > len(outer) {
		return false
	}

	// If any element in inner is not in outer, it is not a subset
	for key := range inner {
		if _, ok := outer[key]; !ok {
			return false
		}
	}

	return true
}

func subsetOfValues(inner, outer map[string]*cb.ConfigValue) bool {
	// The empty set is a subset of all sets
	if len(inner) == 0 {
		return true
	}

	// If inner has more elements than outer, it cannot be a subset
	if len(inner) > len(outer) {
		return false
	}

	// If any element in inner is not in outer, it is not a subset
	for key := range inner {
		if _, ok := outer[key]; !ok {
			return false
		}
	}

	return true
}

func equalConfigGroup(lhs, rhs *cb.ConfigGroup) bool {
	if lhs.GetVersion() != rhs.GetVersion() ||
		lhs.GetModPolicy() != rhs.GetModPolicy() {
		return false
	}

	if !subsetOfGroups(lhs.GetGroups(), rhs.GetGroups()) ||
		!subsetOfGroups(rhs.GetGroups(), lhs.GetGroups()) ||
		!subsetOfPolicies(lhs.GetPolicies(), rhs.GetPolicies()) ||
		!subsetOfPolicies(rhs.GetPolicies(), lhs.GetPolicies()) ||
		!subsetOfValues(lhs.GetValues(), rhs.GetValues()) ||
		!subsetOfValues(rhs.GetValues(), lhs.GetValues()) {
		return false
	}

	return true
}
