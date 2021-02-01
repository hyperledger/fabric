/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package update

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/stretchr/testify/require"
)

func TestNoUpdate(t *testing.T) {
	original := &cb.ConfigGroup{
		Version: 7,
	}
	updated := &cb.ConfigGroup{}

	_, err := Compute(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	require.Error(t, err)
}

func TestMissingGroup(t *testing.T) {
	group := &cb.ConfigGroup{}
	t.Run("MissingOriginal", func(t *testing.T) {
		_, err := Compute(&cb.Config{}, &cb.Config{ChannelGroup: group})

		require.Error(t, err)
		require.Regexp(t, "no channel group included for original config", err.Error())
	})
	t.Run("MissingOriginal", func(t *testing.T) {
		_, err := Compute(&cb.Config{ChannelGroup: group}, &cb.Config{})

		require.Error(t, err)
		require.Regexp(t, "no channel group included for updated config", err.Error())
	})
}

func TestGroupModPolicyUpdate(t *testing.T) {
	original := &cb.ConfigGroup{
		Version:   7,
		ModPolicy: "foo",
	}
	updated := &cb.ConfigGroup{
		ModPolicy: "bar",
	}

	cu, err := Compute(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	require.NoError(t, err)

	expectedReadSet := &cb.ConfigGroup{
		Version:  original.Version,
		Groups:   map[string]*cb.ConfigGroup{},
		Policies: map[string]*cb.ConfigPolicy{},
		Values:   map[string]*cb.ConfigValue{},
	}

	require.Equal(t, expectedReadSet, cu.ReadSet, "Mismatched read set")

	expectedWriteSet := &cb.ConfigGroup{
		Version:   original.Version + 1,
		Groups:    map[string]*cb.ConfigGroup{},
		Policies:  map[string]*cb.ConfigPolicy{},
		Values:    map[string]*cb.ConfigValue{},
		ModPolicy: updated.ModPolicy,
	}

	require.Equal(t, expectedWriteSet, cu.WriteSet, "Mismatched write set")
}

func TestGroupPolicyModification(t *testing.T) {
	policy1Name := "foo"
	policy2Name := "bar"
	original := &cb.ConfigGroup{
		Version: 4,
		Policies: map[string]*cb.ConfigPolicy{
			policy1Name: {
				Version: 2,
				Policy: &cb.Policy{
					Type: 3,
				},
			},
			policy2Name: {
				Version: 1,
				Policy: &cb.Policy{
					Type: 5,
				},
			},
		},
	}
	updated := &cb.ConfigGroup{
		Policies: map[string]*cb.ConfigPolicy{
			policy1Name: original.Policies[policy1Name],
			policy2Name: {
				Policy: &cb.Policy{
					Type: 9,
				},
			},
		},
	}

	cu, err := Compute(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	require.NoError(t, err)

	expectedReadSet := &cb.ConfigGroup{
		Version:  original.Version,
		Policies: map[string]*cb.ConfigPolicy{},
		Values:   map[string]*cb.ConfigValue{},
		Groups:   map[string]*cb.ConfigGroup{},
	}

	require.Equal(t, expectedReadSet, cu.ReadSet, "Mismatched read set")

	expectedWriteSet := &cb.ConfigGroup{
		Version: original.Version,
		Policies: map[string]*cb.ConfigPolicy{
			policy2Name: {
				Policy: &cb.Policy{
					Type: updated.Policies[policy2Name].Policy.Type,
				},
				Version: original.Policies[policy2Name].Version + 1,
			},
		},
		Values: map[string]*cb.ConfigValue{},
		Groups: map[string]*cb.ConfigGroup{},
	}

	require.Equal(t, expectedWriteSet, cu.WriteSet, "Mismatched write set")
}

func TestGroupValueModification(t *testing.T) {
	value1Name := "foo"
	value2Name := "bar"
	original := &cb.ConfigGroup{
		Version: 7,
		Values: map[string]*cb.ConfigValue{
			value1Name: {
				Version: 3,
				Value:   []byte("value1value"),
			},
			value2Name: {
				Version: 6,
				Value:   []byte("value2value"),
			},
		},
	}
	updated := &cb.ConfigGroup{
		Values: map[string]*cb.ConfigValue{
			value1Name: original.Values[value1Name],
			value2Name: {
				Value: []byte("updatedValued2Value"),
			},
		},
	}

	cu, err := Compute(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	require.NoError(t, err)

	expectedReadSet := &cb.ConfigGroup{
		Version:  original.Version,
		Values:   map[string]*cb.ConfigValue{},
		Policies: map[string]*cb.ConfigPolicy{},
		Groups:   map[string]*cb.ConfigGroup{},
	}

	require.Equal(t, expectedReadSet, cu.ReadSet, "Mismatched read set")

	expectedWriteSet := &cb.ConfigGroup{
		Version: original.Version,
		Values: map[string]*cb.ConfigValue{
			value2Name: {
				Value:   updated.Values[value2Name].Value,
				Version: original.Values[value2Name].Version + 1,
			},
		},
		Policies: map[string]*cb.ConfigPolicy{},
		Groups:   map[string]*cb.ConfigGroup{},
	}

	require.Equal(t, expectedWriteSet, cu.WriteSet, "Mismatched write set")
}

func TestGroupGroupsModification(t *testing.T) {
	subGroupName := "foo"
	original := &cb.ConfigGroup{
		Version: 7,
		Groups: map[string]*cb.ConfigGroup{
			subGroupName: {
				Version: 3,
				Values: map[string]*cb.ConfigValue{
					"testValue": {
						Version: 3,
					},
				},
			},
		},
	}
	updated := &cb.ConfigGroup{
		Groups: map[string]*cb.ConfigGroup{
			subGroupName: {},
		},
	}

	cu, err := Compute(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	require.NoError(t, err)

	expectedReadSet := &cb.ConfigGroup{
		Version: original.Version,
		Groups: map[string]*cb.ConfigGroup{
			subGroupName: {
				Version:  original.Groups[subGroupName].Version,
				Policies: map[string]*cb.ConfigPolicy{},
				Values:   map[string]*cb.ConfigValue{},
				Groups:   map[string]*cb.ConfigGroup{},
			},
		},
		Policies: map[string]*cb.ConfigPolicy{},
		Values:   map[string]*cb.ConfigValue{},
	}

	require.Equal(t, expectedReadSet, cu.ReadSet, "Mismatched read set")

	expectedWriteSet := &cb.ConfigGroup{
		Version: original.Version,
		Groups: map[string]*cb.ConfigGroup{
			subGroupName: {
				Version:  original.Groups[subGroupName].Version + 1,
				Groups:   map[string]*cb.ConfigGroup{},
				Policies: map[string]*cb.ConfigPolicy{},
				Values:   map[string]*cb.ConfigValue{},
			},
		},
		Policies: map[string]*cb.ConfigPolicy{},
		Values:   map[string]*cb.ConfigValue{},
	}

	require.Equal(t, expectedWriteSet, cu.WriteSet, "Mismatched write set")
}

func TestGroupValueAddition(t *testing.T) {
	value1Name := "foo"
	value2Name := "bar"
	original := &cb.ConfigGroup{
		Version: 7,
		Values: map[string]*cb.ConfigValue{
			value1Name: {
				Version: 3,
				Value:   []byte("value1value"),
			},
		},
	}
	updated := &cb.ConfigGroup{
		Values: map[string]*cb.ConfigValue{
			value1Name: original.Values[value1Name],
			value2Name: {
				Version: 9,
				Value:   []byte("newValue2"),
			},
		},
	}

	cu, err := Compute(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	require.NoError(t, err)

	expectedReadSet := &cb.ConfigGroup{
		Version: original.Version,
		Values: map[string]*cb.ConfigValue{
			value1Name: {
				Version: original.Values[value1Name].Version,
			},
		},
		Policies: map[string]*cb.ConfigPolicy{},
		Groups:   map[string]*cb.ConfigGroup{},
	}

	require.Equal(t, expectedReadSet, cu.ReadSet, "Mismatched read set")

	expectedWriteSet := &cb.ConfigGroup{
		Version: original.Version + 1,
		Values: map[string]*cb.ConfigValue{
			value1Name: {
				Version: original.Values[value1Name].Version,
			},
			value2Name: {
				Value:   updated.Values[value2Name].Value,
				Version: 0,
			},
		},
		Policies: map[string]*cb.ConfigPolicy{},
		Groups:   map[string]*cb.ConfigGroup{},
	}

	require.Equal(t, expectedWriteSet, cu.WriteSet, "Mismatched write set")
}

func TestGroupPolicySwap(t *testing.T) {
	policy1Name := "foo"
	policy2Name := "bar"
	original := &cb.ConfigGroup{
		Version: 4,
		Policies: map[string]*cb.ConfigPolicy{
			policy1Name: {
				Version: 2,
				Policy: &cb.Policy{
					Type: 3,
				},
			},
		},
	}
	updated := &cb.ConfigGroup{
		Policies: map[string]*cb.ConfigPolicy{
			policy2Name: {
				Version: 1,
				Policy: &cb.Policy{
					Type: 5,
				},
			},
		},
	}

	cu, err := Compute(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	require.NoError(t, err)

	expectedReadSet := &cb.ConfigGroup{
		Version:  original.Version,
		Policies: map[string]*cb.ConfigPolicy{},
		Values:   map[string]*cb.ConfigValue{},
		Groups:   map[string]*cb.ConfigGroup{},
	}

	require.Equal(t, expectedReadSet, cu.ReadSet, "Mismatched read set")

	expectedWriteSet := &cb.ConfigGroup{
		Version: original.Version + 1,
		Policies: map[string]*cb.ConfigPolicy{
			policy2Name: {
				Policy: &cb.Policy{
					Type: updated.Policies[policy2Name].Policy.Type,
				},
				Version: 0,
			},
		},
		Values: map[string]*cb.ConfigValue{},
		Groups: map[string]*cb.ConfigGroup{},
	}

	require.Equal(t, expectedWriteSet, cu.WriteSet, "Mismatched write set")
}

func TestComplex(t *testing.T) {
	existingGroup1Name := "existingGroup1"
	existingGroup2Name := "existingGroup2"
	existingPolicyName := "existingPolicy"
	original := &cb.ConfigGroup{
		Version: 4,
		Groups: map[string]*cb.ConfigGroup{
			existingGroup1Name: {
				Version: 2,
			},
			existingGroup2Name: {
				Version: 2,
			},
		},
		Policies: map[string]*cb.ConfigPolicy{
			existingPolicyName: {
				Version: 8,
				Policy: &cb.Policy{
					Type: 5,
				},
			},
		},
	}

	newGroupName := "newGroup"
	newPolicyName := "newPolicy"
	newValueName := "newValue"
	updated := &cb.ConfigGroup{
		Groups: map[string]*cb.ConfigGroup{
			existingGroup1Name: {},
			newGroupName: {
				Values: map[string]*cb.ConfigValue{
					newValueName: {},
				},
			},
		},
		Policies: map[string]*cb.ConfigPolicy{
			existingPolicyName: {
				Policy: &cb.Policy{
					Type: 5,
				},
			},
			newPolicyName: {
				Version: 6,
				Policy: &cb.Policy{
					Type: 5,
				},
			},
		},
	}

	cu, err := Compute(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	require.NoError(t, err)

	expectedReadSet := &cb.ConfigGroup{
		Version: original.Version,
		Policies: map[string]*cb.ConfigPolicy{
			existingPolicyName: {
				Version: original.Policies[existingPolicyName].Version,
			},
		},
		Values: map[string]*cb.ConfigValue{},
		Groups: map[string]*cb.ConfigGroup{
			existingGroup1Name: {
				Version: original.Groups[existingGroup1Name].Version,
			},
		},
	}

	require.Equal(t, expectedReadSet, cu.ReadSet, "Mismatched read set")

	expectedWriteSet := &cb.ConfigGroup{
		Version: original.Version + 1,
		Policies: map[string]*cb.ConfigPolicy{
			existingPolicyName: {
				Version: original.Policies[existingPolicyName].Version,
			},
			newPolicyName: {
				Version: 0,
				Policy: &cb.Policy{
					Type: 5,
				},
			},
		},
		Groups: map[string]*cb.ConfigGroup{
			existingGroup1Name: {
				Version: original.Groups[existingGroup1Name].Version,
			},
			newGroupName: {
				Version: 0,
				Values: map[string]*cb.ConfigValue{
					newValueName: {},
				},
				Policies: map[string]*cb.ConfigPolicy{},
				Groups:   map[string]*cb.ConfigGroup{},
			},
		},
		Values: map[string]*cb.ConfigValue{},
	}

	require.Equal(t, expectedWriteSet, cu.WriteSet, "Mismatched write set")
}

func TestTwiceNestedModification(t *testing.T) {
	subGroupName := "foo"
	subSubGroupName := "bar"
	valueName := "testValue"
	original := &cb.ConfigGroup{
		Groups: map[string]*cb.ConfigGroup{
			subGroupName: {
				Groups: map[string]*cb.ConfigGroup{
					subSubGroupName: {
						Values: map[string]*cb.ConfigValue{
							valueName: {},
						},
					},
				},
			},
		},
	}
	updated := &cb.ConfigGroup{
		Groups: map[string]*cb.ConfigGroup{
			subGroupName: {
				Groups: map[string]*cb.ConfigGroup{
					subSubGroupName: {
						Values: map[string]*cb.ConfigValue{
							valueName: {
								ModPolicy: "new",
							},
						},
					},
				},
			},
		},
	}

	cu, err := Compute(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	require.NoError(t, err)

	expectedReadSet := &cb.ConfigGroup{
		Version: original.Version,
		Groups: map[string]*cb.ConfigGroup{
			subGroupName: {
				Groups: map[string]*cb.ConfigGroup{
					subSubGroupName: {
						Policies: map[string]*cb.ConfigPolicy{},
						Values:   map[string]*cb.ConfigValue{},
						Groups:   map[string]*cb.ConfigGroup{},
					},
				},
				Policies: map[string]*cb.ConfigPolicy{},
				Values:   map[string]*cb.ConfigValue{},
			},
		},
		Policies: map[string]*cb.ConfigPolicy{},
		Values:   map[string]*cb.ConfigValue{},
	}

	require.Equal(t, expectedReadSet, cu.ReadSet, "Mismatched read set")

	expectedWriteSet := &cb.ConfigGroup{
		Version: original.Version,
		Groups: map[string]*cb.ConfigGroup{
			subGroupName: {
				Groups: map[string]*cb.ConfigGroup{
					subSubGroupName: {
						Values: map[string]*cb.ConfigValue{
							valueName: {
								Version:   original.Groups[subGroupName].Groups[subSubGroupName].Values[valueName].Version + 1,
								ModPolicy: updated.Groups[subGroupName].Groups[subSubGroupName].Values[valueName].ModPolicy,
							},
						},
						Policies: map[string]*cb.ConfigPolicy{},
						Groups:   map[string]*cb.ConfigGroup{},
					},
				},
				Policies: map[string]*cb.ConfigPolicy{},
				Values:   map[string]*cb.ConfigValue{},
			},
		},
		Policies: map[string]*cb.ConfigPolicy{},
		Values:   map[string]*cb.ConfigValue{},
	}

	require.Equal(t, expectedWriteSet, cu.WriteSet, "Mismatched write set")
}
