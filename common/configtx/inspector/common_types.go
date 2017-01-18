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

package inspector

import (
	"fmt"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

func viewableConfigurationEnvelope(name string, configEnvelope *cb.ConfigurationEnvelope) Viewable {
	return &field{
		name:   name,
		values: []Viewable{viewableSignedConfigurationItemSlice("Items", configEnvelope.Items)},
	}
}

func viewableSignedConfigurationItemSlice(name string, signedConfigItems []*cb.SignedConfigurationItem) Viewable {
	values := make([]Viewable, len(signedConfigItems))
	for i, item := range signedConfigItems {
		values[i] = viewableSignedConfigurationItem(fmt.Sprintf("Element %d", i), item)
	}
	return &field{
		name:   name,
		values: values,
	}
}

func viewableSignedConfigurationItem(name string, signedConfigItem *cb.SignedConfigurationItem) Viewable {
	var viewableConfigItem Viewable

	configItem := &cb.ConfigurationItem{}

	if err := proto.Unmarshal(signedConfigItem.ConfigurationItem, configItem); err != nil {
		viewableConfigItem = viewableError(name, err)
	} else {
		viewableConfigItem = viewableConfigurationItem("ConfigurationItem", configItem)
	}

	return &field{
		name:   name,
		values: []Viewable{viewableConfigItem, viewableConfigurationSignatureSlice("Signatures", signedConfigItem.Signatures)},
	}
}

func viewableConfigurationSignatureSlice(name string, configSigs []*cb.ConfigurationSignature) Viewable {
	values := make([]Viewable, len(configSigs))
	for i, item := range configSigs {
		values[i] = viewableConfigurationSignature(fmt.Sprintf("Element %d", i), item)
	}
	return &field{
		name:   name,
		values: values,
	}
}

func viewableConfigurationSignature(name string, configSig *cb.ConfigurationSignature) Viewable {
	children := make([]Viewable, 2)

	sigHeader := &cb.SignatureHeader{}
	err := proto.Unmarshal(configSig.SignatureHeader, sigHeader)
	if err == nil {
		children[0] = viewableSignatureHeader("SignatureHeader", sigHeader)
	} else {
		children[0] = viewableError("SignatureHeader", err)
	}

	children[1] = viewableBytes("Signature", configSig.Signature)

	return &field{
		name:   name,
		values: children,
	}
}

func viewableSignatureHeader(name string, sh *cb.SignatureHeader) Viewable {
	return &field{
		name:   name,
		values: []Viewable{viewableBytes("Creator", sh.Creator), viewableBytes("Nonce", sh.Nonce)},
	}
}

func viewableConfigurationItem(name string, ci *cb.ConfigurationItem) Viewable {

	values := make([]Viewable, 6) // Type, Key, Header, LastModified, ModificationPolicy, Value
	values[0] = viewableString("Type", fmt.Sprintf("%v", ci.Type))
	values[1] = viewableString("Key", ci.Key)
	values[2] = viewableString("Header", "TODO")
	values[3] = viewableString("LastModified", fmt.Sprintf("%d", ci.LastModified))
	values[4] = viewableString("ModificationPolicy", ci.ModificationPolicy)

	typeFactory, ok := typeMap[ci.Type]
	if ok {
		values[5] = typeFactory.Value(ci)
	} else {
		values[5] = viewableError("Value", fmt.Errorf("Unknown message type: %v", ci.Type))
	}
	return &field{
		name:   name,
		values: values,
	}
}
