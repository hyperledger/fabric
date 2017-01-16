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
	case "IngressPolicy":
		value := &ab.IngressPolicy{}
		if err := proto.Unmarshal(configItem.Value, value); err != nil {
			return viewableError(name, err)
		}
		return viewableIngressPolicy(configItem.Key, value)
	case "EgressPolicy":
		value := &ab.EgressPolicy{}
		if err := proto.Unmarshal(configItem.Value, value); err != nil {
			return viewableError(name, err)
		}
		return viewableEgressPolicy(configItem.Key, value)
	case "ChainCreators":
		value := &ab.ChainCreators{}
		if err := proto.Unmarshal(configItem.Value, value); err != nil {
			return viewableError(name, err)
		}
		return viewableChainCreators(configItem.Key, value)
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

func viewableIngressPolicy(name string, ingressPolicy *ab.IngressPolicy) Viewable {
	return &field{
		name:   name,
		values: []Viewable{viewableString("Name", ingressPolicy.Name)},
	}
}

func viewableEgressPolicy(name string, egressPolicy *ab.EgressPolicy) Viewable {
	return &field{
		name:   name,
		values: []Viewable{viewableString("Name", egressPolicy.Name)},
	}
}

func viewableChainCreators(name string, creators *ab.ChainCreators) Viewable {
	return &field{
		name:   name,
		values: []Viewable{viewableStringSlice("Policies", creators.Policies)},
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
