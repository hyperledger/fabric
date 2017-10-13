/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

const (
	msgVersion = int32(0)
	epoch      = 0
)

type channelCreationTemplate struct {
	consortiumName string
	orgs           []string
}

// NewChainCreationTemplate takes a consortium name and a Template to produce a
// Template which outputs an appropriately constructed list of ConfigUpdateEnvelopes.
func NewChainCreationTemplate(consortiumName string, orgs []string) configtx.Template {
	return &channelCreationTemplate{
		consortiumName: consortiumName,
		orgs:           orgs,
	}
}

func (cct *channelCreationTemplate) Envelope(channelID string) (*cb.ConfigUpdateEnvelope, error) {
	rSet := TemplateConsortium(cct.consortiumName)
	wSet := TemplateConsortium(cct.consortiumName)

	rSet.Groups[ApplicationGroupKey] = cb.NewConfigGroup()
	wSet.Groups[ApplicationGroupKey] = cb.NewConfigGroup()

	for _, org := range cct.orgs {
		rSet.Groups[ApplicationGroupKey].Groups[org] = cb.NewConfigGroup()
		wSet.Groups[ApplicationGroupKey].Groups[org] = cb.NewConfigGroup()
	}

	wSet.Groups[ApplicationGroupKey].ModPolicy = AdminsPolicyKey
	wSet.Groups[ApplicationGroupKey].Policies[AdminsPolicyKey] = policies.ImplicitMetaPolicyWithSubPolicy(AdminsPolicyKey, cb.ImplicitMetaPolicy_MAJORITY)
	wSet.Groups[ApplicationGroupKey].Policies[AdminsPolicyKey].ModPolicy = AdminsPolicyKey
	wSet.Groups[ApplicationGroupKey].Policies[WritersPolicyKey] = policies.ImplicitMetaPolicyWithSubPolicy(WritersPolicyKey, cb.ImplicitMetaPolicy_ANY)
	wSet.Groups[ApplicationGroupKey].Policies[WritersPolicyKey].ModPolicy = AdminsPolicyKey
	wSet.Groups[ApplicationGroupKey].Policies[ReadersPolicyKey] = policies.ImplicitMetaPolicyWithSubPolicy(ReadersPolicyKey, cb.ImplicitMetaPolicy_ANY)
	wSet.Groups[ApplicationGroupKey].Policies[ReadersPolicyKey].ModPolicy = AdminsPolicyKey
	wSet.Groups[ApplicationGroupKey].Version = 1

	return &cb.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(&cb.ConfigUpdate{
			ChannelId: channelID,
			ReadSet:   rSet,
			WriteSet:  wSet,
			IsolatedData: map[string][]byte{
				pb.RSCCSeedDataKey: utils.MarshalOrPanic(&cb.Config{
					Type: int32(cb.ConfigType_RESOURCE),
					ChannelGroup: &cb.ConfigGroup{
						// All of the default seed data values would inside this ConfigGroup
						Values: map[string]*cb.ConfigValue{
							"QSCC.Example1": &cb.ConfigValue{
								Value: utils.MarshalOrPanic(&pb.Resource{
									PolicyRef: policies.ChannelApplicationAdmins,
								}),
								ModPolicy: policies.ChannelApplicationAdmins,
							},
							"QSCC.Example2": &cb.ConfigValue{
								Value: utils.MarshalOrPanic(&pb.Resource{
									PolicyRef: "Example",
								}),
								ModPolicy: policies.ChannelApplicationAdmins,
							},
						},
						Policies: map[string]*cb.ConfigPolicy{
							"Example": &cb.ConfigPolicy{
								Policy: &cb.Policy{
									Type:  int32(cb.Policy_SIGNATURE),
									Value: utils.MarshalOrPanic(cauthdsl.AcceptAllPolicy),
								},
								ModPolicy: "Example",
							},
						},
						ModPolicy: policies.ChannelApplicationAdmins,
					},
				}),
			},
		}),
	}, nil
}
