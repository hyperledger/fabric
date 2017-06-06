/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package configtx

import (
	"bytes"

	cb "github.com/hyperledger/fabric/protos/common"
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
	if lhs.Version != rhs.Version ||
		lhs.ModPolicy != rhs.ModPolicy {
		return false
	}

	if !subsetOfGroups(lhs.Groups, rhs.Groups) ||
		!subsetOfGroups(rhs.Groups, lhs.Groups) ||
		!subsetOfPolicies(lhs.Policies, rhs.Policies) ||
		!subsetOfPolicies(rhs.Policies, lhs.Policies) ||
		!subsetOfValues(lhs.Values, rhs.Values) ||
		!subsetOfValues(rhs.Values, lhs.Values) {
		return false
	}

	return true
}
