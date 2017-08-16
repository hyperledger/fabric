/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resources

import (
	"testing"

	"github.com/hyperledger/fabric/common/cauthdsl"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"

	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

var dummyPolicy = &cb.ConfigPolicy{
	Policy: &cb.Policy{
		Type:  int32(cb.Policy_SIGNATURE),
		Value: utils.MarshalOrPanic(cauthdsl.AcceptAllPolicy),
	},
}

func TestBundleGreenPath(t *testing.T) {
	env, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, "foo", nil, &cb.ConfigEnvelope{
		Config: &cb.Config{
			ChannelGroup: &cb.ConfigGroup{
				Values: map[string]*cb.ConfigValue{
					"Foo": &cb.ConfigValue{
						Value: utils.MarshalOrPanic(&pb.Resource{
							PolicyRef: "foo",
						}),
					},
					"Bar": &cb.ConfigValue{
						Value: utils.MarshalOrPanic(&pb.Resource{
							PolicyRef: "/Channel/foo",
						}),
					},
				},
				Policies: map[string]*cb.ConfigPolicy{
					"foo": dummyPolicy,
					"bar": dummyPolicy,
				},
				Groups: map[string]*cb.ConfigGroup{
					"subGroup": &cb.ConfigGroup{
						Policies: map[string]*cb.ConfigPolicy{
							"other": dummyPolicy,
						},
					},
				},
			},
		},
	}, 0, 0)
	assert.NoError(t, err)

	b, err := New(env, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, b)
	assert.Equal(t, "/Resources/foo", b.ResourcePolicyMapper().PolicyRefForResource("Foo"))
	assert.Equal(t, "/Channel/foo", b.ResourcePolicyMapper().PolicyRefForResource("Bar"))

	t.Run("Code coverage nits", func(t *testing.T) {
		assert.Equal(t, b.RootGroupKey(), RootGroupKey)
		assert.NotNil(t, b.ConfigtxManager())
		assert.NotNil(t, b.PolicyManager())
	})
}

func TestBundleBadSubGroup(t *testing.T) {
	env, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, "foo", nil, &cb.ConfigEnvelope{
		Config: &cb.Config{
			ChannelGroup: &cb.ConfigGroup{
				Groups: map[string]*cb.ConfigGroup{
					"subGroup": &cb.ConfigGroup{
						Values: map[string]*cb.ConfigValue{
							"Foo": &cb.ConfigValue{
								Value: utils.MarshalOrPanic(&pb.Resource{
									PolicyRef: "foo",
								}),
							},
						},
						Policies: map[string]*cb.ConfigPolicy{
							"other": dummyPolicy,
						},
					},
				},
			},
		},
	}, 0, 0)
	assert.NoError(t, err)

	_, err = New(env, nil, nil)
	assert.Error(t, err)
	assert.Regexp(t, "sub-groups not allowed to have values", err.Error())
}
