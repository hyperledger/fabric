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

// UnmarshalConfigNext attempts to unmarshal bytes to a *cb.ConfigNext
func UnmarshalConfigNext(data []byte) (*cb.ConfigNext, error) {
	config := &cb.ConfigNext{}
	err := proto.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// ConfigNextToConfig is a XXX temporary method for use in the change series converting the configtx protos
// so error handling and testing is omitted as it will be removed shortly
func ConfigNextToConfig(config *cb.ConfigNext) *cb.Config {
	result := &cb.Config{
		Header: config.Header,
	}

	channel := config.Channel

	for key, value := range channel.Values {
		result.Items = append(result.Items, &cb.ConfigItem{
			Key:   key,
			Type:  cb.ConfigItem_Chain,
			Value: value.Value,
		})
	}

	for key, value := range channel.Groups[OrdererGroup].Values {
		result.Items = append(result.Items, &cb.ConfigItem{
			Key:   key,
			Type:  cb.ConfigItem_Orderer,
			Value: value.Value,
		})
	}

	for key, value := range channel.Groups[ApplicationGroup].Values {
		result.Items = append(result.Items, &cb.ConfigItem{
			Key:   key,
			Type:  cb.ConfigItem_Peer,
			Value: value.Value,
		})
	}

	logger.Debugf("Processing polices %v", channel.Policies)
	for key, value := range channel.Policies {
		logger.Debugf("Reversing policy %s", key)
		result.Items = append(result.Items, &cb.ConfigItem{
			Key:   key,
			Type:  cb.ConfigItem_Policy,
			Value: utils.MarshalOrPanic(value.Policy),
		})
	}

	// Note, for now, all MSPs are encoded in both ApplicationGroup and OrdererGroup, so we only need to pick one
	for key, group := range channel.Groups[ApplicationGroup].Groups {
		msp, ok := group.Values[MSPKey]
		if !ok {
			panic("Expected MSP defined")
		}
		result.Items = append(result.Items, &cb.ConfigItem{
			Key:   key,
			Type:  cb.ConfigItem_MSP,
			Value: msp.Value,
		})
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
