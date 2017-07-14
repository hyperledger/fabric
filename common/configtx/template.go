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
	"fmt"

	"github.com/hyperledger/fabric/common/config"
	configmsp "github.com/hyperledger/fabric/common/config/msp"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
)

const (
	// CreationPolicyKey defines the config key used in the channel
	// config, under which the creation policy is defined.
	CreationPolicyKey = "CreationPolicy"
	msgVersion        = int32(0)
	epoch             = 0
)

// Template can be used to facilitate creation of config transactions
type Template interface {
	// Envelope returns a ConfigUpdateEnvelope for the given chainID
	Envelope(chainID string) (*cb.ConfigUpdateEnvelope, error)
}

type simpleTemplate struct {
	configGroup *cb.ConfigGroup
}

// NewSimpleTemplate creates a Template using the supplied ConfigGroups
func NewSimpleTemplate(configGroups ...*cb.ConfigGroup) Template {
	sts := make([]Template, len(configGroups))
	for i, group := range configGroups {
		sts[i] = &simpleTemplate{
			configGroup: group,
		}
	}
	return NewCompositeTemplate(sts...)
}

// Envelope returns a ConfigUpdateEnvelope for the given chainID
func (st *simpleTemplate) Envelope(chainID string) (*cb.ConfigUpdateEnvelope, error) {
	config, err := proto.Marshal(&cb.ConfigUpdate{
		ChannelId: chainID,
		WriteSet:  st.configGroup,
	})

	if err != nil {
		return nil, err
	}

	return &cb.ConfigUpdateEnvelope{
		ConfigUpdate: config,
	}, nil
}

type compositeTemplate struct {
	templates []Template
}

// NewCompositeTemplate creates a Template using the source Templates
func NewCompositeTemplate(templates ...Template) Template {
	return &compositeTemplate{templates: templates}
}

func copyGroup(source *cb.ConfigGroup, target *cb.ConfigGroup) error {
	for key, value := range source.Values {
		_, ok := target.Values[key]
		if ok {
			return fmt.Errorf("Duplicate key: %s", key)
		}
		target.Values[key] = value
	}

	for key, policy := range source.Policies {
		_, ok := target.Policies[key]
		if ok {
			return fmt.Errorf("Duplicate policy: %s", key)
		}
		target.Policies[key] = policy
	}

	for key, group := range source.Groups {
		_, ok := target.Groups[key]
		if !ok {
			newGroup := cb.NewConfigGroup()
			newGroup.ModPolicy = group.ModPolicy
			target.Groups[key] = newGroup
		}

		err := copyGroup(group, target.Groups[key])
		if err != nil {
			return fmt.Errorf("Error copying group %s: %s", key, err)
		}
	}
	return nil
}

// Envelope returns a ConfigUpdateEnvelope for the given chainID
func (ct *compositeTemplate) Envelope(chainID string) (*cb.ConfigUpdateEnvelope, error) {
	channel := cb.NewConfigGroup()

	for i := range ct.templates {
		configEnv, err := ct.templates[i].Envelope(chainID)
		if err != nil {
			return nil, err
		}
		config, err := UnmarshalConfigUpdate(configEnv.ConfigUpdate)
		if err != nil {
			return nil, err
		}
		err = copyGroup(config.WriteSet, channel)
		if err != nil {
			return nil, err
		}
	}

	marshaledConfig, err := proto.Marshal(&cb.ConfigUpdate{
		ChannelId: chainID,
		WriteSet:  channel,
	})
	if err != nil {
		return nil, err
	}

	return &cb.ConfigUpdateEnvelope{ConfigUpdate: marshaledConfig}, nil
}

type modPolicySettingTemplate struct {
	modPolicy string
	template  Template
}

// NewModPolicySettingTemplate wraps another template and sets the ModPolicy of
// every ConfigGroup/ConfigValue/ConfigPolicy without a modPolicy to modPolicy
func NewModPolicySettingTemplate(modPolicy string, template Template) Template {
	return &modPolicySettingTemplate{
		modPolicy: modPolicy,
		template:  template,
	}
}

func setGroupModPolicies(modPolicy string, group *cb.ConfigGroup) {
	if group.ModPolicy == "" {
		group.ModPolicy = modPolicy
	}

	for _, value := range group.Values {
		if value.ModPolicy != "" {
			continue
		}
		value.ModPolicy = modPolicy
	}

	for _, policy := range group.Policies {
		if policy.ModPolicy != "" {
			continue
		}
		policy.ModPolicy = modPolicy
	}

	for _, nextGroup := range group.Groups {
		setGroupModPolicies(modPolicy, nextGroup)
	}
}

func (mpst *modPolicySettingTemplate) Envelope(channelID string) (*cb.ConfigUpdateEnvelope, error) {
	configUpdateEnv, err := mpst.template.Envelope(channelID)
	if err != nil {
		return nil, err
	}

	config, err := UnmarshalConfigUpdate(configUpdateEnv.ConfigUpdate)
	if err != nil {
		return nil, err
	}

	setGroupModPolicies(mpst.modPolicy, config.WriteSet)
	configUpdateEnv.ConfigUpdate = utils.MarshalOrPanic(config)
	return configUpdateEnv, nil
}

type channelCreationTemplate struct {
	consortiumName string
	orgs           []string
}

// NewChainCreationTemplate takes a consortium name and a Template to produce a
// Template which outputs an appropriately constructed list of ConfigUpdateEnvelopes.
func NewChainCreationTemplate(consortiumName string, orgs []string) Template {
	return &channelCreationTemplate{
		consortiumName: consortiumName,
		orgs:           orgs,
	}
}

func (cct *channelCreationTemplate) Envelope(channelID string) (*cb.ConfigUpdateEnvelope, error) {
	rSet := config.TemplateConsortium(cct.consortiumName)
	wSet := config.TemplateConsortium(cct.consortiumName)

	rSet.Groups[config.ApplicationGroupKey] = cb.NewConfigGroup()
	wSet.Groups[config.ApplicationGroupKey] = cb.NewConfigGroup()

	for _, org := range cct.orgs {
		rSet.Groups[config.ApplicationGroupKey].Groups[org] = cb.NewConfigGroup()
		wSet.Groups[config.ApplicationGroupKey].Groups[org] = cb.NewConfigGroup()
	}

	wSet.Groups[config.ApplicationGroupKey].ModPolicy = configmsp.AdminsPolicyKey
	wSet.Groups[config.ApplicationGroupKey].Policies[configmsp.AdminsPolicyKey] = policies.ImplicitMetaPolicyWithSubPolicy(configmsp.AdminsPolicyKey, cb.ImplicitMetaPolicy_MAJORITY)
	wSet.Groups[config.ApplicationGroupKey].Policies[configmsp.AdminsPolicyKey].ModPolicy = configmsp.AdminsPolicyKey
	wSet.Groups[config.ApplicationGroupKey].Policies[configmsp.WritersPolicyKey] = policies.ImplicitMetaPolicyWithSubPolicy(configmsp.WritersPolicyKey, cb.ImplicitMetaPolicy_ANY)
	wSet.Groups[config.ApplicationGroupKey].Policies[configmsp.WritersPolicyKey].ModPolicy = configmsp.AdminsPolicyKey
	wSet.Groups[config.ApplicationGroupKey].Policies[configmsp.ReadersPolicyKey] = policies.ImplicitMetaPolicyWithSubPolicy(configmsp.ReadersPolicyKey, cb.ImplicitMetaPolicy_ANY)
	wSet.Groups[config.ApplicationGroupKey].Policies[configmsp.ReadersPolicyKey].ModPolicy = configmsp.AdminsPolicyKey
	wSet.Groups[config.ApplicationGroupKey].Version = 1

	return &cb.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(&cb.ConfigUpdate{
			ChannelId: channelID,
			ReadSet:   rSet,
			WriteSet:  wSet,
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
