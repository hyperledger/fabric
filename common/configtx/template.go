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
	"github.com/hyperledger/fabric/common/util"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/msp"
)

const (
	CreationPolicyKey = "CreationPolicy"
	msgVersion        = int32(0)
	epoch             = 0

	ApplicationGroup = "Application"
	OrdererGroup     = "Orderer"
	MSPKey           = "MSP"
)

// Template can be used to faciliate creation of config transactions
type Template interface {
	// Items returns a set of ConfigEnvelopes for the given chainID
	Envelope(chainID string) (*cb.ConfigEnvelope, error)
}

type simpleTemplate struct {
	items []*cb.ConfigItem
}

// NewSimpleTemplate creates a Template using the supplied items
// XXX This signature will change soon, leaving as is for backwards compatibility
func NewSimpleTemplate(items ...*cb.ConfigItem) Template {
	return &simpleTemplate{items: items}
}

// Items returns a set of ConfigEnvelopes for the given chainID, and errors only on marshaling errors
func (st *simpleTemplate) Envelope(chainID string) (*cb.ConfigEnvelope, error) {
	channel := cb.NewConfigGroup()
	channel.Groups[ApplicationGroup] = cb.NewConfigGroup()
	channel.Groups[OrdererGroup] = cb.NewConfigGroup()

	for _, item := range st.items {
		var values map[string]*cb.ConfigValue
		switch item.Type {
		case cb.ConfigItem_Peer:
			values = channel.Groups[ApplicationGroup].Values
		case cb.ConfigItem_Orderer:
			values = channel.Groups[OrdererGroup].Values
		case cb.ConfigItem_Chain:
			values = channel.Values
		case cb.ConfigItem_Policy:
			logger.Debugf("Templating about policy %s", item.Key)
			policy := &cb.Policy{}
			err := proto.Unmarshal(item.Value, policy)
			if err != nil {
				return nil, err
			}
			channel.Policies[item.Key] = &cb.ConfigPolicy{
				Policy: policy,
			}
			continue
		case cb.ConfigItem_MSP:
			group := cb.NewConfigGroup()
			channel.Groups[ApplicationGroup].Groups[item.Key] = group
			channel.Groups[OrdererGroup].Groups[item.Key] = group
			group.Values[MSPKey] = &cb.ConfigValue{
				Value: item.Value,
			}
			continue
		}

		// For Peer, Orderer, Chain, types
		values[item.Key] = &cb.ConfigValue{
			Value: item.Value,
		}
	}

	marshaledConfig, err := proto.Marshal(&cb.ConfigNext{
		Header: &cb.ChainHeader{
			ChainID: chainID,
			Type:    int32(cb.HeaderType_CONFIGURATION_ITEM),
		},
		Channel: channel,
	})
	if err != nil {
		return nil, err
	}

	return &cb.ConfigEnvelope{Config: marshaledConfig}, nil
}

type compositeTemplate struct {
	templates []Template
}

// NewSimpleTemplate creates a Template using the source Templates
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
			target.Groups[key] = cb.NewConfigGroup()
		}

		err := copyGroup(group, target.Groups[key])
		if err != nil {
			return fmt.Errorf("Error copying group %s: %s", key, err)
		}
	}
	return nil
}

// Items returns a set of ConfigEnvelopes for the given chainID, and errors only on marshaling errors
func (ct *compositeTemplate) Envelope(chainID string) (*cb.ConfigEnvelope, error) {
	channel := cb.NewConfigGroup()
	channel.Groups[ApplicationGroup] = cb.NewConfigGroup()
	channel.Groups[OrdererGroup] = cb.NewConfigGroup()

	for i := range ct.templates {
		configEnv, err := ct.templates[i].Envelope(chainID)
		if err != nil {
			return nil, err
		}
		config, err := UnmarshalConfigNext(configEnv.Config)
		if err != nil {
			return nil, err
		}
		err = copyGroup(config.Channel, channel)
		if err != nil {
			return nil, err
		}
	}

	marshaledConfig, err := proto.Marshal(&cb.ConfigNext{
		Header: &cb.ChainHeader{
			ChainID: chainID,
			Type:    int32(cb.HeaderType_CONFIGURATION_ITEM),
		},
		Channel: channel,
	})
	if err != nil {
		return nil, err
	}

	return &cb.ConfigEnvelope{Config: marshaledConfig}, nil
}

// NewChainCreationTemplate takes a CreationPolicy and a Template to produce a Template which outputs an appropriately
// constructed list of ConfigEnvelope.  Note, using this Template in
// a CompositeTemplate will invalidate the CreationPolicy
func NewChainCreationTemplate(creationPolicy string, template Template) Template {
	creationPolicyTemplate := NewSimpleTemplate(&cb.ConfigItem{
		Type: cb.ConfigItem_Orderer,
		Key:  CreationPolicyKey,
		Value: utils.MarshalOrPanic(&ab.CreationPolicy{
			Policy: creationPolicy,
		}),
	})

	return NewCompositeTemplate(creationPolicyTemplate, template)
}

// join takes a number of []*cb.ConfigItems and produces their concatenation
func join(sets ...[]*cb.ConfigItem) []*cb.ConfigItem {
	total := 0
	for _, set := range sets {
		total += len(set)
	}
	result := make([]*cb.ConfigItem, total)
	last := 0
	for _, set := range sets {
		for i := range set {
			result[i+last] = set[i]
		}
		last += len(set)
	}

	return result
}

// MakeChainCreationTransaction is a handy utility function for creating new chain transactions using the underlying Template framework
func MakeChainCreationTransaction(creationPolicy string, chainID string, signer msp.SigningIdentity, templates ...Template) (*cb.Envelope, error) {
	sSigner, err := signer.Serialize()
	if err != nil {
		return nil, fmt.Errorf("Serialization of identity failed, err %s", err)
	}

	newChainTemplate := NewChainCreationTemplate(creationPolicy, NewCompositeTemplate(templates...))
	newConfigEnv, err := newChainTemplate.Envelope(chainID)
	if err != nil {
		return nil, err
	}

	newConfigEnv.Signatures = []*cb.ConfigSignature{&cb.ConfigSignature{
		SignatureHeader: utils.MarshalOrPanic(utils.MakeSignatureHeader(sSigner, utils.CreateNonceOrPanic())),
	}}
	newConfigEnv.Signatures[0].Signature, err = signer.Sign(util.ConcatenateBytes(newConfigEnv.Signatures[0].SignatureHeader, newConfigEnv.Config))
	if err != nil {
		return nil, err
	}

	payloadChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_TRANSACTION, msgVersion, chainID, epoch)
	payloadSignatureHeader := utils.MakeSignatureHeader(sSigner, utils.CreateNonceOrPanic())
	payloadHeader := utils.MakePayloadHeader(payloadChainHeader, payloadSignatureHeader)
	payload := &cb.Payload{Header: payloadHeader, Data: utils.MarshalOrPanic(newConfigEnv)}
	paylBytes := utils.MarshalOrPanic(payload)

	// sign the payload
	sig, err := signer.Sign(paylBytes)
	if err != nil {
		return nil, err
	}

	return &cb.Envelope{Payload: paylBytes, Signature: sig}, nil
}
