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

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

// This file contains the functions needed to create Viewables for protos defined in
// the orderer configuration proto

type ordererTypes struct{}

func (ot ordererTypes) Value(configItem *cb.ConfigurationItem) Viewable {
	name := "Value"
	switch configItem.Key {
	case "ConsensusType":
		value := &ab.ConsensusType{}
		if err := proto.Unmarshal(configItem.Value, value); err != nil {
			return viewableError(name, err)
		}
		return viewableConsensusType(configItem.Key, value)
	case "BatchSize":
		value := &ab.BatchSize{}
		if err := proto.Unmarshal(configItem.Value, value); err != nil {
			return viewableError(name, err)
		}
		return viewableBatchSize(configItem.Key, value)
	case "BatchTimeout":
		value := &ab.BatchTimeout{}
		if err := proto.Unmarshal(configItem.Value, value); err != nil {
			return viewableError(name, err)
		}
		return viewableBatchTimeout(configItem.Key, value)
	case "CreationPolicy":
		value := &ab.CreationPolicy{}
		if err := proto.Unmarshal(configItem.Value, value); err != nil {
			return viewableError(name, err)
		}
		return viewableCreationPolicy(configItem.Key, value)
	case "IngressPolicyNames":
		value := &ab.IngressPolicyNames{}
		if err := proto.Unmarshal(configItem.Value, value); err != nil {
			return viewableError(name, err)
		}
		return viewableIngressPolicyNames(configItem.Key, value)
	case "EgressPolicyNames":
		value := &ab.EgressPolicyNames{}
		if err := proto.Unmarshal(configItem.Value, value); err != nil {
			return viewableError(name, err)
		}
		return viewableEgressPolicyNames(configItem.Key, value)
	case "ChainCreationPolicyNames":
		value := &ab.ChainCreationPolicyNames{}
		if err := proto.Unmarshal(configItem.Value, value); err != nil {
			return viewableError(name, err)
		}
		return viewableChainCreationPolicyNames(configItem.Key, value)
	case "KafkaBrokers":
		value := &ab.KafkaBrokers{}
		if err := proto.Unmarshal(configItem.Value, value); err != nil {
			return viewableError(name, err)
		}
		return viewableKafkaBrokers(configItem.Key, value)
	default:
	}
	return viewableError(name, fmt.Errorf("Unknown key: %s", configItem.Key))
}

func viewableConsensusType(name string, consensusType *ab.ConsensusType) Viewable {
	return &field{
		name:   name,
		values: []Viewable{viewableString("Type", consensusType.Type)},
	}
}

func viewableBatchTimeout(name string, batchTimeout *ab.BatchTimeout) Viewable {
	return &field{
		name:   name,
		values: []Viewable{viewableString("Timeout", batchTimeout.Timeout)},
	}
}

func viewableIngressPolicyNames(name string, ingressPolicy *ab.IngressPolicyNames) Viewable {
	return &field{
		name:   name,
		values: []Viewable{viewableStringSlice("Name", ingressPolicy.Names)},
	}
}

func viewableEgressPolicyNames(name string, egressPolicy *ab.EgressPolicyNames) Viewable {
	return &field{
		name:   name,
		values: []Viewable{viewableStringSlice("Names", egressPolicy.Names)},
	}
}

func viewableChainCreationPolicyNames(name string, chainCreationPolicyNames *ab.ChainCreationPolicyNames) Viewable {
	return &field{
		name:   name,
		values: []Viewable{viewableStringSlice("Names", chainCreationPolicyNames.Names)},
	}
}

func viewableKafkaBrokers(name string, brokers *ab.KafkaBrokers) Viewable {
	return &field{
		name:   name,
		values: []Viewable{viewableStringSlice("Brokers", brokers.Brokers)},
	}
}

func viewableCreationPolicy(name string, creationPolicy *ab.CreationPolicy) Viewable {
	return &field{
		name: name,
		values: []Viewable{
			viewableString("Policy", creationPolicy.Policy),
			viewableBytes("Digest", creationPolicy.Digest),
		},
	}
}

func viewableBatchSize(name string, batchSize *ab.BatchSize) Viewable {
	return &field{
		name: name,
		values: []Viewable{
			viewableUint32("MaxMessageCount", batchSize.MaxMessageCount),
			viewableUint32("AbsoluteMaxBytes", batchSize.AbsoluteMaxBytes),
		},
	}
}

func init() {
	typeMap[cb.ConfigurationItem_Orderer] = ordererTypes{}
}
