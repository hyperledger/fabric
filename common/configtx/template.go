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
)

// Template can be used to faciliate creation of configuration transactions
type Template interface {
	// Items returns a set of ConfigurationEnvelopes for the given chainID
	Envelope(chainID string) (*cb.ConfigurationEnvelope, error)
}

type simpleTemplate struct {
	items []*cb.ConfigurationItem
}

// NewSimpleTemplate creates a Template using the supplied items
func NewSimpleTemplate(items ...*cb.ConfigurationItem) Template {
	return &simpleTemplate{items: items}
}

// Items returns a set of ConfigurationEnvelopes for the given chainID, and errors only on marshaling errors
func (st *simpleTemplate) Envelope(chainID string) (*cb.ConfigurationEnvelope, error) {
	marshaledConfig, err := proto.Marshal(&cb.Config{
		Header: &cb.ChainHeader{
			ChainID: chainID,
			Type:    int32(cb.HeaderType_CONFIGURATION_ITEM),
		},
		Items: st.items,
	})
	if err != nil {
		return nil, err
	}

	return &cb.ConfigurationEnvelope{Config: marshaledConfig}, nil
}

type compositeTemplate struct {
	templates []Template
}

// NewSimpleTemplate creates a Template using the source Templates
func NewCompositeTemplate(templates ...Template) Template {
	return &compositeTemplate{templates: templates}
}

// Items returns a set of ConfigurationEnvelopes for the given chainID, and errors only on marshaling errors
func (ct *compositeTemplate) Envelope(chainID string) (*cb.ConfigurationEnvelope, error) {
	items := make([][]*cb.ConfigurationItem, len(ct.templates))
	for i := range ct.templates {
		configEnv, err := ct.templates[i].Envelope(chainID)
		if err != nil {
			return nil, err
		}
		config, err := UnmarshalConfig(configEnv.Config)
		if err != nil {
			return nil, err
		}
		items[i] = config.Items
	}

	return NewSimpleTemplate(join(items...)...).Envelope(chainID)
}

// NewChainCreationTemplate takes a CreationPolicy and a Template to produce a Template which outputs an appropriately
// constructed list of ConfigurationEnvelope.  Note, using this Template in
// a CompositeTemplate will invalidate the CreationPolicy
func NewChainCreationTemplate(creationPolicy string, template Template) Template {
	creationPolicyTemplate := NewSimpleTemplate(&cb.ConfigurationItem{
		Type: cb.ConfigurationItem_Orderer,
		Key:  CreationPolicyKey,
		Value: utils.MarshalOrPanic(&ab.CreationPolicy{
			Policy: creationPolicy,
		}),
	})

	return NewCompositeTemplate(creationPolicyTemplate, template)
}

// join takes a number of []*cb.ConfigurationItems and produces their concatenation
func join(sets ...[]*cb.ConfigurationItem) []*cb.ConfigurationItem {
	total := 0
	for _, set := range sets {
		total += len(set)
	}
	result := make([]*cb.ConfigurationItem, total)
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

	newConfigEnv.Signatures = []*cb.ConfigurationSignature{&cb.ConfigurationSignature{
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
