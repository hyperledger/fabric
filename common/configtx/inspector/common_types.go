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

func viewableConfigEnvelope(name string, configEnvelope *cb.ConfigEnvelope) Viewable {
	return &field{
		name:   name,
		values: []Viewable{viewableConfig("Config", configEnvelope.Config), viewableConfigSignatureSlice("Signatures", configEnvelope.Signatures)},
	}
}

func viewableConfig(name string, configBytes []byte) Viewable {
	config := &cb.Config{}
	err := proto.Unmarshal(configBytes, config)
	if err != nil {
		return viewableError(name, err)
	}
	values := make([]Viewable, len(config.Items))
	for i, item := range config.Items {
		values[i] = viewableConfigItem(fmt.Sprintf("Element %d", i), item)
	}
	return &field{
		name:   name,
		values: values,
	}
}

func viewableConfigSignatureSlice(name string, configSigs []*cb.ConfigSignature) Viewable {
	values := make([]Viewable, len(configSigs))
	for i, item := range configSigs {
		values[i] = viewableConfigSignature(fmt.Sprintf("Element %d", i), item)
	}
	return &field{
		name:   name,
		values: values,
	}
}

func viewableConfigSignature(name string, configSig *cb.ConfigSignature) Viewable {
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

func viewableConfigItem(name string, ci *cb.ConfigItem) Viewable {

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
