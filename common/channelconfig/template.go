/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"fmt"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp"
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

// MakeChainCreationTransaction is a handy utility function for creating new chain transactions using the underlying Template framework
func MakeChainCreationTransaction(channelID string, consortium string, signer msp.SigningIdentity, orgs ...string) (*cb.Envelope, error) {
	newChainTemplate := NewChainCreationTemplate(consortium, orgs)
	newConfigUpdateEnv, err := newChainTemplate.Envelope(channelID)
	if err != nil {
		return nil, err
	}

	payloadSignatureHeader := &cb.SignatureHeader{}
	if signer != nil {
		sSigner, err := signer.Serialize()
		if err != nil {
			return nil, fmt.Errorf("Serialization of identity failed, err %s", err)
		}

		newConfigUpdateEnv.Signatures = []*cb.ConfigSignature{&cb.ConfigSignature{
			SignatureHeader: utils.MarshalOrPanic(utils.MakeSignatureHeader(sSigner, utils.CreateNonceOrPanic())),
		}}

		newConfigUpdateEnv.Signatures[0].Signature, err = signer.Sign(util.ConcatenateBytes(newConfigUpdateEnv.Signatures[0].SignatureHeader, newConfigUpdateEnv.ConfigUpdate))
		if err != nil {
			return nil, err
		}

		payloadSignatureHeader = utils.MakeSignatureHeader(sSigner, utils.CreateNonceOrPanic())
	}

	payloadChannelHeader := utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, msgVersion, channelID, epoch)
	utils.SetTxID(payloadChannelHeader, payloadSignatureHeader)
	payloadHeader := utils.MakePayloadHeader(payloadChannelHeader, payloadSignatureHeader)
	payload := &cb.Payload{Header: payloadHeader, Data: utils.MarshalOrPanic(newConfigUpdateEnv)}
	paylBytes := utils.MarshalOrPanic(payload)

	var sig []byte
	if signer != nil {
		// sign the payload
		sig, err = signer.Sign(paylBytes)
		if err != nil {
			return nil, err
		}
	}

	return &cb.Envelope{Payload: paylBytes, Signature: sig}, nil
}
