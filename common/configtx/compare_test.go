/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/stretchr/testify/require"
)

func TestCompareConfigValue(t *testing.T) {
	// Normal equality
	require.True(t, comparable{
		ConfigValue: &cb.ConfigValue{
			Version:   0,
			ModPolicy: "foo",
			Value:     []byte("bar"),
		},
	}.equals(comparable{
		ConfigValue: &cb.ConfigValue{
			Version:   0,
			ModPolicy: "foo",
			Value:     []byte("bar"),
		},
	}), "Should have found identical config values to be identical")

	// Different Mod Policy
	require.False(t, comparable{
		ConfigValue: &cb.ConfigValue{
			Version:   0,
			ModPolicy: "foo",
			Value:     []byte("bar"),
		},
	}.equals(comparable{
		ConfigValue: &cb.ConfigValue{
			Version:   0,
			ModPolicy: "bar",
			Value:     []byte("bar"),
		},
	}), "Should have detected different mod policy")

	// Different Value
	require.False(t, comparable{
		ConfigValue: &cb.ConfigValue{
			Version:   0,
			ModPolicy: "foo",
			Value:     []byte("bar"),
		},
	}.equals(comparable{
		ConfigValue: &cb.ConfigValue{
			Version:   0,
			ModPolicy: "foo",
			Value:     []byte("foo"),
		},
	}), "Should have detected different value")

	// Different Version
	require.False(t, comparable{
		ConfigValue: &cb.ConfigValue{
			Version:   0,
			ModPolicy: "foo",
			Value:     []byte("bar"),
		},
	}.equals(comparable{
		ConfigValue: &cb.ConfigValue{
			Version:   1,
			ModPolicy: "foo",
			Value:     []byte("bar"),
		},
	}), "Should have detected different version")

	// One nil value
	require.False(t, comparable{
		ConfigValue: &cb.ConfigValue{
			Version:   0,
			ModPolicy: "foo",
			Value:     []byte("bar"),
		},
	}.equals(comparable{}), "Should have detected nil other value")
}

func TestCompareConfigPolicy(t *testing.T) {
	// Normal equality
	require.True(t, comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   0,
			ModPolicy: "foo",
			Policy: &cb.Policy{
				Type:  1,
				Value: []byte("foo"),
			},
		},
	}.equals(comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   0,
			ModPolicy: "foo",
			Policy: &cb.Policy{
				Type:  1,
				Value: []byte("foo"),
			},
		},
	}), "Should have found identical config policies to be identical")

	// Different mod policy
	require.False(t, comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   0,
			ModPolicy: "foo",
			Policy: &cb.Policy{
				Type:  1,
				Value: []byte("foo"),
			},
		},
	}.equals(comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   0,
			ModPolicy: "bar",
			Policy: &cb.Policy{
				Type:  1,
				Value: []byte("foo"),
			},
		},
	}), "Should have detected different mod policy")

	// Different version
	require.False(t, comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   0,
			ModPolicy: "foo",
			Policy: &cb.Policy{
				Type:  1,
				Value: []byte("foo"),
			},
		},
	}.equals(comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   1,
			ModPolicy: "foo",
			Policy: &cb.Policy{
				Type:  1,
				Value: []byte("foo"),
			},
		},
	}), "Should have detected different version")

	// Different policy type
	require.False(t, comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   0,
			ModPolicy: "foo",
			Policy: &cb.Policy{
				Type:  1,
				Value: []byte("foo"),
			},
		},
	}.equals(comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   0,
			ModPolicy: "foo",
			Policy: &cb.Policy{
				Type:  2,
				Value: []byte("foo"),
			},
		},
	}), "Should have detected different policy type")

	// Different policy value
	require.False(t, comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   0,
			ModPolicy: "foo",
			Policy: &cb.Policy{
				Type:  1,
				Value: []byte("foo"),
			},
		},
	}.equals(comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   0,
			ModPolicy: "foo",
			Policy: &cb.Policy{
				Type:  1,
				Value: []byte("bar"),
			},
		},
	}), "Should have detected different policy value")

	// One nil value
	require.False(t, comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   0,
			ModPolicy: "foo",
			Policy: &cb.Policy{
				Type:  1,
				Value: []byte("foo"),
			},
		},
	}.equals(comparable{}), "Should have detected one nil value")

	// One nil policy
	require.False(t, comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   0,
			ModPolicy: "foo",
			Policy: &cb.Policy{
				Type:  1,
				Value: []byte("foo"),
			},
		},
	}.equals(comparable{
		ConfigPolicy: &cb.ConfigPolicy{
			Version:   0,
			ModPolicy: "foo",
			Policy: &cb.Policy{
				Type: 1,
			},
		},
	}), "Should have detected one nil policy")
}

func TestCompareConfigGroup(t *testing.T) {
	// Normal equality
	require.True(t, comparable{
		ConfigGroup: &cb.ConfigGroup{
			Version:   0,
			ModPolicy: "foo",
			Groups:    map[string]*cb.ConfigGroup{"Foo1": nil, "Bar1": nil},
			Values:    map[string]*cb.ConfigValue{"Foo2": nil, "Bar2": nil},
			Policies:  map[string]*cb.ConfigPolicy{"Foo3": nil, "Bar3": nil},
		},
	}.equals(comparable{
		ConfigGroup: &cb.ConfigGroup{
			Version:   0,
			ModPolicy: "foo",
			Groups:    map[string]*cb.ConfigGroup{"Foo1": nil, "Bar1": nil},
			Values:    map[string]*cb.ConfigValue{"Foo2": nil, "Bar2": nil},
			Policies:  map[string]*cb.ConfigPolicy{"Foo3": nil, "Bar3": nil},
		},
	}), "Should have found identical config groups to be identical")

	// Different mod policy
	require.False(t, comparable{
		ConfigGroup: &cb.ConfigGroup{
			Version:   0,
			ModPolicy: "foo",
		},
	}.equals(comparable{
		ConfigGroup: &cb.ConfigGroup{
			Version:   0,
			ModPolicy: "bar",
		},
	}), "Should have detected different mod policy")

	// Different version
	require.False(t, comparable{
		ConfigGroup: &cb.ConfigGroup{
			Version:   0,
			ModPolicy: "foo",
		},
	}.equals(comparable{
		ConfigGroup: &cb.ConfigGroup{
			Version:   1,
			ModPolicy: "foo",
		},
	}), "Should have detected different version")

	// Different groups
	require.False(t, comparable{
		ConfigGroup: &cb.ConfigGroup{
			Version:   0,
			ModPolicy: "foo",
			Groups:    map[string]*cb.ConfigGroup{"Foo1": nil},
			Values:    map[string]*cb.ConfigValue{"Foo2": nil, "Bar2": nil},
			Policies:  map[string]*cb.ConfigPolicy{"Foo3": nil, "Bar3": nil},
		},
	}.equals(comparable{
		ConfigGroup: &cb.ConfigGroup{
			Version:   0,
			ModPolicy: "foo",
			Groups:    map[string]*cb.ConfigGroup{"Foo1": nil, "Bar1": nil},
			Values:    map[string]*cb.ConfigValue{"Foo2": nil, "Bar2": nil},
			Policies:  map[string]*cb.ConfigPolicy{"Foo3": nil, "Bar3": nil},
		},
	}), "Should have detected different groups entries")

	// Different values
	require.False(t, comparable{
		ConfigGroup: &cb.ConfigGroup{
			Version:   0,
			ModPolicy: "foo",
			Groups:    map[string]*cb.ConfigGroup{"Foo1": nil, "Bar1": nil},
			Values:    map[string]*cb.ConfigValue{"Foo2": nil, "Bar2": nil},
			Policies:  map[string]*cb.ConfigPolicy{"Foo3": nil, "Bar3": nil},
		},
	}.equals(comparable{
		ConfigGroup: &cb.ConfigGroup{
			Version:   0,
			ModPolicy: "foo",
			Groups:    map[string]*cb.ConfigGroup{"Foo1": nil, "Bar1": nil},
			Values:    map[string]*cb.ConfigValue{"Foo2": nil},
			Policies:  map[string]*cb.ConfigPolicy{"Foo3": nil, "Bar3": nil},
		},
	}), "Should have detected fifferent values entries")

	// Different policies
	require.False(t, comparable{
		ConfigGroup: &cb.ConfigGroup{
			Version:   0,
			ModPolicy: "foo",
			Groups:    map[string]*cb.ConfigGroup{"Foo1": nil, "Bar1": nil},
			Values:    map[string]*cb.ConfigValue{"Foo2": nil, "Bar2": nil},
			Policies:  map[string]*cb.ConfigPolicy{"Foo3": nil, "Bar3": nil},
		},
	}.equals(comparable{
		ConfigGroup: &cb.ConfigGroup{
			Version:   0,
			ModPolicy: "foo",
			Groups:    map[string]*cb.ConfigGroup{"Foo1": nil, "Bar1": nil},
			Values:    map[string]*cb.ConfigValue{"Foo2": nil, "Bar2": nil},
			Policies:  map[string]*cb.ConfigPolicy{"Foo3": nil, "Bar4": nil},
		},
	}), "Should have detected fifferent policies entries")
}
