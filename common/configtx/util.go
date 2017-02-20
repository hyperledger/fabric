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

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
)

// UnmarshalConfig attempts to unmarshal bytes to a *cb.Config
func UnmarshalConfig(data []byte) (*cb.Config, error) {
	config := &cb.Config{}
	err := proto.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// UnmarshalConfig attempts to unmarshal bytes to a *cb.Config
func UnmarshalConfigOrPanic(data []byte) *cb.Config {
	result, err := UnmarshalConfig(data)
	if err != nil {
		panic(err)
	}
	return result
}

// UnmarshalConfigUpdate attempts to unmarshal bytes to a *cb.ConfigUpdate
func UnmarshalConfigUpdate(data []byte) (*cb.ConfigUpdate, error) {
	configUpdate := &cb.ConfigUpdate{}
	err := proto.Unmarshal(data, configUpdate)
	if err != nil {
		return nil, err
	}
	return configUpdate, nil
}

// UnmarshalConfigUpdate attempts to unmarshal bytes to a *cb.ConfigUpdate or panics
func UnmarshalConfigUpdateOrPanic(data []byte) *cb.ConfigUpdate {
	result, err := UnmarshalConfigUpdate(data)
	if err != nil {
		panic(err)
	}
	return result
}

// UnmarshalConfigUpdateEnvelope attempts to unmarshal bytes to a *cb.ConfigUpdate
func UnmarshalConfigUpdateEnvelope(data []byte) (*cb.ConfigUpdateEnvelope, error) {
	configUpdateEnvelope := &cb.ConfigUpdateEnvelope{}
	err := proto.Unmarshal(data, configUpdateEnvelope)
	if err != nil {
		return nil, err
	}
	return configUpdateEnvelope, nil
}

// UnmarshalConfigUpdateEnvelope attempts to unmarshal bytes to a *cb.ConfigUpdateEnvelope or panics
func UnmarshalConfigUpdateEnvelopeOrPanic(data []byte) *cb.ConfigUpdateEnvelope {
	result, err := UnmarshalConfigUpdateEnvelope(data)
	if err != nil {
		panic(err)
	}
	return result
}

// UnmarshalConfigEnvelope attempts to unmarshal bytes to a *cb.ConfigEnvelope
func UnmarshalConfigEnvelope(data []byte) (*cb.ConfigEnvelope, error) {
	configEnv := &cb.ConfigEnvelope{}
	err := proto.Unmarshal(data, configEnv)
	if err != nil {
		return nil, err
	}
	return configEnv, nil
}

// UnmarshalConfigEnvelope attempts to unmarshal bytes to a *cb.ConfigEnvelope or panics
func UnmarshalConfigEnvelopeOrPanic(data []byte) *cb.ConfigEnvelope {
	result, err := UnmarshalConfigEnvelope(data)
	if err != nil {
		panic(err)
	}
	return result
}

// ConfigEnvelopeFromBlock extract the config envelope from a config block
func ConfigEnvelopeFromBlock(block *cb.Block) (*cb.ConfigEnvelope, error) {
	if block.Data == nil || len(block.Data.Data) != 1 {
		return nil, fmt.Errorf("Not a config block, must contain exactly one tx")
	}

	envelope, err := utils.UnmarshalEnvelope(block.Data.Data[0])
	if err != nil {
		return nil, err
	}

	payload, err := utils.UnmarshalPayload(envelope.Payload)
	if err != nil {
		return nil, err
	}

	return UnmarshalConfigEnvelope(payload.Data)
}
