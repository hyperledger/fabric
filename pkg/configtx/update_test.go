/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	. "github.com/onsi/gomega"
)

func TestNoUpdate(t *testing.T) {
	gt := NewGomegaWithT(t)
	original := &cb.ConfigGroup{
		Version: 7,
	}
	updated := &cb.ConfigGroup{}

	_, err := computeConfigUpdate(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	gt.Expect(err).To(HaveOccurred())
}

func TestMissingGroup(t *testing.T) {
	gt := NewGomegaWithT(t)
	group := &cb.ConfigGroup{}
	t.Run("MissingOriginal", func(t *testing.T) {
		_, err := computeConfigUpdate(&cb.Config{}, &cb.Config{ChannelGroup: group})

		gt.Expect(err).To(HaveOccurred())
		gt.Expect(err).To(MatchError("no channel group included for original config"))
	})
	t.Run("MissingOriginal", func(t *testing.T) {
		_, err := computeConfigUpdate(&cb.Config{ChannelGroup: group}, &cb.Config{})

		gt.Expect(err).To(HaveOccurred())
		gt.Expect(err).To(MatchError("no channel group included for updated config"))
	})
}

func TestGroupModPolicyUpdate(t *testing.T) {
	gt := NewGomegaWithT(t)
	original := &cb.ConfigGroup{
		Version:   7,
		ModPolicy: "foo",
	}
	updated := &cb.ConfigGroup{
		ModPolicy: "bar",
	}

	cu, err := computeConfigUpdate(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	gt.Expect(err).NotTo(HaveOccurred())

	expectedReadSet := &cb.ConfigGroup{
		Version:  original.Version,
		Groups:   map[string]*cb.ConfigGroup{},
		Policies: map[string]*cb.ConfigPolicy{},
		Values:   map[string]*cb.ConfigValue{},
	}

	gt.Expect(expectedReadSet).To(Equal(cu.ReadSet), "Mismatched read set")

	expectedWriteSet := &cb.ConfigGroup{
		Version:   original.Version + 1,
		Groups:    map[string]*cb.ConfigGroup{},
		Policies:  map[string]*cb.ConfigPolicy{},
		Values:    map[string]*cb.ConfigValue{},
		ModPolicy: updated.ModPolicy,
	}

	gt.Expect(expectedWriteSet).To(Equal(cu.WriteSet), "Mismatched write set")
}

func TestGroupPolicyModification(t *testing.T) {
	gt := NewGomegaWithT(t)
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

	cu, err := computeConfigUpdate(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	gt.Expect(err).NotTo(HaveOccurred())

	expectedReadSet := &cb.ConfigGroup{
		Version:  original.Version,
		Policies: map[string]*cb.ConfigPolicy{},
		Values:   map[string]*cb.ConfigValue{},
		Groups:   map[string]*cb.ConfigGroup{},
	}

	gt.Expect(expectedReadSet).To(Equal(cu.ReadSet), "Mismatched read set")

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

	gt.Expect(expectedWriteSet).To(Equal(cu.WriteSet), "Mismatched write set")
}

func TestGroupValueModification(t *testing.T) {
	gt := NewGomegaWithT(t)
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

	cu, err := computeConfigUpdate(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	gt.Expect(err).NotTo(HaveOccurred())

	expectedReadSet := &cb.ConfigGroup{
		Version:  original.Version,
		Values:   map[string]*cb.ConfigValue{},
		Policies: map[string]*cb.ConfigPolicy{},
		Groups:   map[string]*cb.ConfigGroup{},
	}

	gt.Expect(expectedReadSet).To(Equal(cu.ReadSet), "Mismatched read set")

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

	gt.Expect(expectedWriteSet).To(Equal(cu.WriteSet), "Mismatched write set")
}

func TestGroupGroupsModification(t *testing.T) {
	gt := NewGomegaWithT(t)
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

	cu, err := computeConfigUpdate(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	gt.Expect(err).NotTo(HaveOccurred())

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

	gt.Expect(expectedReadSet).To(Equal(cu.ReadSet), "Mismatched read set")

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

	gt.Expect(expectedWriteSet).To(Equal(cu.WriteSet), "Mismatched write set")
}

func TestGroupValueAddition(t *testing.T) {
	gt := NewGomegaWithT(t)
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

	cu, err := computeConfigUpdate(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	gt.Expect(err).NotTo(HaveOccurred())

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

	gt.Expect(expectedReadSet).To(Equal(cu.ReadSet), "Mismatched read set")

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

	gt.Expect(expectedWriteSet).To(Equal(cu.WriteSet), "Mismatched write set")
}

func TestGroupPolicySwap(t *testing.T) {
	gt := NewGomegaWithT(t)
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

	cu, err := computeConfigUpdate(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	gt.Expect(err).NotTo(HaveOccurred())

	expectedReadSet := &cb.ConfigGroup{
		Version:  original.Version,
		Policies: map[string]*cb.ConfigPolicy{},
		Values:   map[string]*cb.ConfigValue{},
		Groups:   map[string]*cb.ConfigGroup{},
	}

	gt.Expect(expectedReadSet).To(Equal(cu.ReadSet), "Mismatched read set")

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

	gt.Expect(expectedWriteSet).To(Equal(cu.WriteSet), "Mismatched write set")
}

func TestComplex(t *testing.T) {
	gt := NewGomegaWithT(t)
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

	cu, err := computeConfigUpdate(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	gt.Expect(err).NotTo(HaveOccurred())

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

	gt.Expect(expectedReadSet).To(Equal(cu.ReadSet), "Mismatched read set")

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

	gt.Expect(expectedWriteSet).To(Equal(cu.WriteSet), "Mismatched write set")
}

func TestTwiceNestedModification(t *testing.T) {
	gt := NewGomegaWithT(t)
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

	cu, err := computeConfigUpdate(&cb.Config{
		ChannelGroup: original,
	}, &cb.Config{
		ChannelGroup: updated,
	})

	gt.Expect(err).NotTo(HaveOccurred())

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

	gt.Expect(expectedReadSet).To(Equal(cu.ReadSet), "Mismatched read set")

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

	gt.Expect(expectedWriteSet).To(Equal(cu.WriteSet), "Mismatched write set")
}
