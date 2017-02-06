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
	// Items returns a set of SignedConfigurationItems for the given chainID
	Items(chainID string) ([]*cb.SignedConfigurationItem, error)
}

type simpleTemplate struct {
	items []*cb.ConfigurationItem
}

// NewSimpleTemplate creates a Template using the supplied items
func NewSimpleTemplate(items ...*cb.ConfigurationItem) Template {
	return &simpleTemplate{items: items}
}

// Items returns a set of SignedConfigurationItems for the given chainID, and errors only on marshaling errors
func (st *simpleTemplate) Items(chainID string) ([]*cb.SignedConfigurationItem, error) {
	signedItems := make([]*cb.SignedConfigurationItem, len(st.items))
	for i := range st.items {
		mItem, err := proto.Marshal(st.items[i])
		if err != nil {
			return nil, err
		}
		signedItems[i] = &cb.SignedConfigurationItem{ConfigurationItem: mItem}
	}

	return signedItems, nil
}

type compositeTemplate struct {
	templates []Template
}

// NewSimpleTemplate creates a Template using the source Templates
func NewCompositeTemplate(templates ...Template) Template {
	return &compositeTemplate{templates: templates}
}

// Items returns a set of SignedConfigurationItems for the given chainID, and errors only on marshaling errors
func (ct *compositeTemplate) Items(chainID string) ([]*cb.SignedConfigurationItem, error) {
	items := make([][]*cb.SignedConfigurationItem, len(ct.templates))
	var err error
	for i := range ct.templates {
		items[i], err = ct.templates[i].Items(chainID)
		if err != nil {
			return nil, err
		}
	}

	return join(items...), nil
}

type newChainTemplate struct {
	creationPolicy string
	hash           func([]byte) []byte
	template       Template
}

// NewChainCreationTemplate takes a CreationPolicy and a Template to produce a Template which outputs an appropriately
// constructed list of SignedConfigurationItem including an appropriate digest.  Note, using this Template in
// a CompositeTemplate will invalidate the CreationPolicy
func NewChainCreationTemplate(creationPolicy string, hash func([]byte) []byte, template Template) Template {
	return &newChainTemplate{
		creationPolicy: creationPolicy,
		hash:           hash,
		template:       template,
	}
}

// Items returns a set of SignedConfigurationItems for the given chainID, and errors only on marshaling errors
func (nct *newChainTemplate) Items(chainID string) ([]*cb.SignedConfigurationItem, error) {
	items, err := nct.template.Items(chainID)
	if err != nil {
		return nil, err
	}

	creationPolicy := &cb.SignedConfigurationItem{
		ConfigurationItem: utils.MarshalOrPanic(&cb.ConfigurationItem{
			Type: cb.ConfigurationItem_Orderer,
			Key:  CreationPolicyKey,
			Value: utils.MarshalOrPanic(&ab.CreationPolicy{
				Policy: nct.creationPolicy,
				Digest: HashItems(items, nct.hash),
			}),
		}),
	}

	return join([]*cb.SignedConfigurationItem{creationPolicy}, items), nil
}

// HashItems is a utility method for computing the hash of the concatenation of the marshaled ConfigurationItems
// in a []*cb.SignedConfigurationItem
func HashItems(items []*cb.SignedConfigurationItem, hash func([]byte) []byte) []byte {
	sourceBytes := make([][]byte, len(items))
	for i := range items {
		sourceBytes[i] = items[i].ConfigurationItem
	}
	return hash(util.ConcatenateBytes(sourceBytes...))
}

// join takes a number of SignedConfigurationItem slices and produces a single item
func join(sets ...[]*cb.SignedConfigurationItem) []*cb.SignedConfigurationItem {
	total := 0
	for _, set := range sets {
		total += len(set)
	}
	result := make([]*cb.SignedConfigurationItem, total)
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
	composite := NewCompositeTemplate(templates...)
	items, err := composite.Items(chainID)
	if err != nil {
		return nil, err
	}

	manager, err := NewManagerImpl(&cb.ConfigurationEnvelope{Header: &cb.ChainHeader{ChainID: chainID, Type: int32(cb.HeaderType_CONFIGURATION_ITEM)}, Items: items}, NewInitializer(), nil)
	if err != nil {
		return nil, err
	}

	newChainTemplate := NewChainCreationTemplate(creationPolicy, manager.ChainConfig().HashingAlgorithm(), composite)
	signedConfigItems, err := newChainTemplate.Items(chainID)
	if err != nil {
		return nil, err
	}

	sSigner, err := signer.Serialize()
	if err != nil {
		return nil, fmt.Errorf("Serialization of identity failed, err %s", err)
	}

	payloadChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_TRANSACTION, msgVersion, chainID, epoch)
	payloadSignatureHeader := utils.MakeSignatureHeader(sSigner, utils.CreateNonceOrPanic())
	payloadHeader := utils.MakePayloadHeader(payloadChainHeader, payloadSignatureHeader)
	payload := &cb.Payload{Header: payloadHeader, Data: utils.MarshalOrPanic(&cb.ConfigurationEnvelope{
		Items:  signedConfigItems,
		Header: &cb.ChainHeader{ChainID: chainID, Type: int32(cb.HeaderType_CONFIGURATION_ITEM)},
	})}
	paylBytes := utils.MarshalOrPanic(payload)

	// sign the payload
	sig, err := signer.Sign(paylBytes)
	if err != nil {
		return nil, err
	}

	return &cb.Envelope{Payload: paylBytes, Signature: sig}, nil
}
