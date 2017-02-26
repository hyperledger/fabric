/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package config

import (
	"testing"

	ab "github.com/hyperledger/fabric/protos/orderer"

	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func TestConsensusType(t *testing.T) {
	oc := &OrdererConfig{ordererGroup: &OrdererGroup{}, protos: &OrdererProtos{ConsensusType: &ab.ConsensusType{Type: "foo"}}}
	assert.NoError(t, oc.validateConsensusType(), "Should have validly set new consensus type")

	oc = &OrdererConfig{
		ordererGroup: &OrdererGroup{OrdererConfig: &OrdererConfig{protos: &OrdererProtos{ConsensusType: &ab.ConsensusType{Type: "foo"}}}},
		protos:       &OrdererProtos{ConsensusType: &ab.ConsensusType{Type: "foo"}},
	}
	assert.NoError(t, oc.validateConsensusType(), "Should have kept consensus type")

	oc = &OrdererConfig{
		ordererGroup: &OrdererGroup{OrdererConfig: &OrdererConfig{protos: &OrdererProtos{ConsensusType: &ab.ConsensusType{Type: "bar"}}}},
		protos:       &OrdererProtos{ConsensusType: &ab.ConsensusType{Type: "foo"}},
	}
	assert.Error(t, oc.validateConsensusType(), "Should have failed to change consensus type")
}

func TestBatchSize(t *testing.T) {

	validMaxMessageCount := uint32(10)
	validAbsoluteMaxBytes := uint32(1000)
	validPreferredMaxBytes := uint32(500)

	oc := &OrdererConfig{protos: &OrdererProtos{BatchSize: &ab.BatchSize{MaxMessageCount: validMaxMessageCount, AbsoluteMaxBytes: validAbsoluteMaxBytes, PreferredMaxBytes: validPreferredMaxBytes}}}
	assert.NoError(t, oc.validateBatchSize(), "BatchSize was valid")

	oc = &OrdererConfig{protos: &OrdererProtos{BatchSize: &ab.BatchSize{MaxMessageCount: 0, AbsoluteMaxBytes: validAbsoluteMaxBytes, PreferredMaxBytes: validPreferredMaxBytes}}}
	assert.Error(t, oc.validateBatchSize(), "MaxMessageCount was zero")

	oc = &OrdererConfig{protos: &OrdererProtos{BatchSize: &ab.BatchSize{MaxMessageCount: validMaxMessageCount, AbsoluteMaxBytes: 0, PreferredMaxBytes: validPreferredMaxBytes}}}
	assert.Error(t, oc.validateBatchSize(), "AbsoluteMaxBytes was zero")

	oc = &OrdererConfig{protos: &OrdererProtos{BatchSize: &ab.BatchSize{MaxMessageCount: validMaxMessageCount, AbsoluteMaxBytes: validAbsoluteMaxBytes, PreferredMaxBytes: validAbsoluteMaxBytes + 1}}}
	assert.Error(t, oc.validateBatchSize(), "PreferredMaxBytes larger to AbsoluteMaxBytes")
}

func TestBatchTimeout(t *testing.T) {
	oc := &OrdererConfig{protos: &OrdererProtos{BatchTimeout: &ab.BatchTimeout{Timeout: "1s"}}}
	assert.NoError(t, oc.validateBatchTimeout(), "Valid batch timeout")

	oc = &OrdererConfig{protos: &OrdererProtos{BatchTimeout: &ab.BatchTimeout{Timeout: "-1s"}}}
	assert.Error(t, oc.validateBatchTimeout(), "Negative batch timeout")

	oc = &OrdererConfig{protos: &OrdererProtos{BatchTimeout: &ab.BatchTimeout{Timeout: "0s"}}}
	assert.Error(t, oc.validateBatchTimeout(), "Zero batch timeout")
}

func TestKafkaBrokers(t *testing.T) {
	oc := &OrdererConfig{protos: &OrdererProtos{KafkaBrokers: &ab.KafkaBrokers{Brokers: []string{"127.0.0.1:9092", "foo.bar:9092"}}}}
	assert.NoError(t, oc.validateKafkaBrokers(), "Valid kafka brokers")

	oc = &OrdererConfig{protos: &OrdererProtos{KafkaBrokers: &ab.KafkaBrokers{Brokers: []string{"127.0.0.1", "foo.bar", "127.0.0.1:-1", "localhost:65536", "foo.bar.:9092", ".127.0.0.1:9092", "-foo.bar:9092"}}}}
	assert.Error(t, oc.validateKafkaBrokers(), "Invalid kafka brokers")
}
