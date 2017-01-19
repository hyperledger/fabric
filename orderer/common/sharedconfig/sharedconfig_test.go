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

package sharedconfig

import (
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	logging "github.com/op/go-logging"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func invalidMessage(key string) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   key,
		Value: []byte("Garbage Data"),
	}
}

func doesFuncCrash(crasher func(), test string) bool {
	// Adapted from https://talks.golang.org/2014/testing.slide#23 to test os.Exit() functionality
	if os.Getenv("BE_CRASHER") == "1" {
		crasher()
		return false
	}
	cmd := exec.Command(os.Args[0], "-test.run="+test)
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return true
	}
	return false
}

func TestDoubleBegin(t *testing.T) {
	crashes := doesFuncCrash(func() {
		m := NewManagerImpl()
		m.BeginConfig()
		m.BeginConfig()
	}, "TestDoubleBegin")

	if !crashes {
		t.Fatalf("Should have crashed on multiple begin configs")
	}
}

func TestCommitWithoutBegin(t *testing.T) {
	crashes := doesFuncCrash(func() {
		m := NewManagerImpl()
		m.CommitConfig()
	}, "TestCommitWithoutBegin")

	if !crashes {
		t.Fatalf("Should have crashed on multiple begin configs")
	}
}

func TestRollback(t *testing.T) {
	m := NewManagerImpl()
	m.pendingConfig = &ordererConfig{}
	m.RollbackConfig()
	if m.pendingConfig != nil {
		t.Fatalf("Should have cleared pending config on rollback")
	}
}

func TestConsensusType(t *testing.T) {
	endType := "foo"
	invalidMessage := invalidMessage(ConsensusTypeKey)
	validMessage := TemplateConsensusType(endType)
	otherValidMessage := TemplateConsensusType("bar")

	m := NewManagerImpl()
	m.BeginConfig()

	err := m.ProposeConfig(validMessage)
	if err != nil {
		t.Fatalf("Error applying valid config: %s", err)
	}

	m.CommitConfig()
	m.BeginConfig()

	err = m.ProposeConfig(invalidMessage)
	if err == nil {
		t.Fatalf("Should have failed on invalid message")
	}

	err = m.ProposeConfig(validMessage)
	if err != nil {
		t.Fatalf("Error re-applying valid config: %s", err)
	}

	err = m.ProposeConfig(otherValidMessage)
	if err == nil {
		t.Fatalf("Should not have applied config with different consensus type after it was initially set")
	}

	m.CommitConfig()

	if nowType := m.ConsensusType(); nowType != endType {
		t.Fatalf("Consensus type should have ended as %s but was %s", endType, nowType)
	}
}

func TestBatchSize(t *testing.T) {

	validMaxMessageCount := uint32(10)
	validAbsoluteMaxBytes := uint32(1000)
	validPreferredMaxBytes := uint32(500)

	t.Run("ValidConfiguration", func(t *testing.T) {
		m := NewManagerImpl()
		m.BeginConfig()
		err := m.ProposeConfig(
			TemplateBatchSize(&ab.BatchSize{MaxMessageCount: validMaxMessageCount, AbsoluteMaxBytes: validAbsoluteMaxBytes, PreferredMaxBytes: validPreferredMaxBytes}),
		)
		assert.Nil(t, err, "Error applying valid config: %s", err)
		m.CommitConfig()
		if m.BatchSize().MaxMessageCount != validMaxMessageCount {
			t.Fatalf("Got batch size max message count of %d. Expected: %d", m.BatchSize().MaxMessageCount, validMaxMessageCount)
		}
		if m.BatchSize().AbsoluteMaxBytes != validAbsoluteMaxBytes {
			t.Fatalf("Got batch size absolute max bytes of %d. Expected: %d", m.BatchSize().AbsoluteMaxBytes, validAbsoluteMaxBytes)
		}
		if m.BatchSize().PreferredMaxBytes != validPreferredMaxBytes {
			t.Fatalf("Got batch size preferred max bytes of %d. Expected: %d", m.BatchSize().PreferredMaxBytes, validPreferredMaxBytes)
		}
	})

	t.Run("UnserializableConfiguration", func(t *testing.T) {
		m := NewManagerImpl()
		m.BeginConfig()
		err := m.ProposeConfig(invalidMessage(BatchSizeKey))
		assert.NotNil(t, err, "Should have failed on invalid message")
		m.CommitConfig()
	})

	t.Run("ZeroMaxMessageCount", func(t *testing.T) {
		m := NewManagerImpl()
		m.BeginConfig()
		err := m.ProposeConfig(TemplateBatchSize(&ab.BatchSize{MaxMessageCount: 0, AbsoluteMaxBytes: validAbsoluteMaxBytes, PreferredMaxBytes: validPreferredMaxBytes}))
		assert.NotNil(t, err, "Should have rejected batch size max message count of 0")
		m.CommitConfig()
	})

	t.Run("ZeroAbsoluteMaxBytes", func(t *testing.T) {
		m := NewManagerImpl()
		m.BeginConfig()
		err := m.ProposeConfig(TemplateBatchSize(&ab.BatchSize{MaxMessageCount: validMaxMessageCount, AbsoluteMaxBytes: 0, PreferredMaxBytes: validPreferredMaxBytes}))
		assert.NotNil(t, err, "Should have rejected batch size absolute max message bytes of 0")
		m.CommitConfig()
	})

	t.Run("TooLargePreferredMaxBytes", func(t *testing.T) {
		m := NewManagerImpl()
		m.BeginConfig()
		err := m.ProposeConfig(TemplateBatchSize(&ab.BatchSize{MaxMessageCount: validMaxMessageCount, AbsoluteMaxBytes: validAbsoluteMaxBytes, PreferredMaxBytes: validAbsoluteMaxBytes + 1}))
		assert.NotNil(t, err, "Should have rejected batch size preferred max message bytes greater than absolute max message bytes")
		m.CommitConfig()
	})
}

func TestBatchTimeout(t *testing.T) {
	endBatchTimeout, _ := time.ParseDuration("1s")
	invalidMessage := invalidMessage(BatchTimeoutKey)
	negativeBatchTimeout := TemplateBatchTimeout("-1s")
	zeroBatchTimeout := TemplateBatchTimeout("0s")
	validMessage := TemplateBatchTimeout(endBatchTimeout.String())

	m := NewManagerImpl()
	m.BeginConfig()

	err := m.ProposeConfig(validMessage)
	if err != nil {
		t.Fatalf("Error applying valid config: %s", err)
	}

	err = m.ProposeConfig(invalidMessage)
	if err == nil {
		t.Fatalf("Should have failed on invalid message")
	}

	err = m.ProposeConfig(negativeBatchTimeout)
	if err == nil {
		t.Fatalf("Should have rejected negative batch timeout: %s", err)
	}

	err = m.ProposeConfig(zeroBatchTimeout)
	if err == nil {
		t.Fatalf("Should have rejected batch timeout of 0")
	}

	m.CommitConfig()

	if nowBatchTimeout := m.BatchTimeout(); nowBatchTimeout != endBatchTimeout {
		t.Fatalf("Got batch timeout of %s when expecting batch size of %s", nowBatchTimeout.String(), endBatchTimeout.String())
	}
}

func TestKafkaBrokers(t *testing.T) {
	endList := []string{"127.0.0.1:9092", "foo.bar:9092"}

	invalidMessage := invalidMessage(KafkaBrokersKey)
	zeroBrokers := TemplateKafkaBrokers([]string{})
	badList := []string{"127.0.0.1", "foo.bar", "127.0.0.1:-1", "localhost:65536", "foo.bar.:9092", ".127.0.0.1:9092", "-foo.bar:9092"}
	badMessages := []*cb.ConfigurationItem{}
	for _, badAddress := range badList {
		badMessages = append(badMessages, TemplateKafkaBrokers([]string{badAddress}))
	}

	validMessage := TemplateKafkaBrokers(endList)

	m := NewManagerImpl()
	m.BeginConfig()

	err := m.ProposeConfig(validMessage)
	if err != nil {
		t.Fatalf("Error applying valid config: %s", err)
	}

	err = m.ProposeConfig(invalidMessage)
	if err == nil {
		t.Fatalf("Should have failed on invalid message")
	}

	err = m.ProposeConfig(zeroBrokers)
	if err == nil {
		t.Fatalf("Should have rejected empty brokers list")
	}

	for i := range badMessages {
		err = m.ProposeConfig(badMessages[i])
		if err == nil {
			t.Fatalf("Should have rejected broker address which is obviously malformed")
		}
	}

	m.CommitConfig()

	nowList := m.KafkaBrokers()
	switch {
	case len(nowList) != len(endList), nowList[0] != endList[0]:
		t.Fatalf("Got brokers list %s when expecting brokers list %s", nowList, endList)
	default:
		return
	}
}

func testPolicyNames(m *ManagerImpl, key string, initializer func(val []string) *cb.ConfigurationItem, retriever func() []string, t *testing.T) {
	endPolicy := []string{"foo", "bar"}
	invalidMessage := invalidMessage(key)
	validMessage := initializer(endPolicy)

	m.BeginConfig()

	err := m.ProposeConfig(validMessage)
	if err != nil {
		t.Fatalf("Error applying valid config: %s", err)
	}

	m.CommitConfig()
	m.BeginConfig()

	err = m.ProposeConfig(invalidMessage)
	if err == nil {
		t.Fatalf("Should have failed on invalid message")
	}

	err = m.ProposeConfig(validMessage)
	if err != nil {
		t.Fatalf("Error re-applying valid config: %s", err)
	}

	m.CommitConfig()

	if nowPolicy := retriever(); !reflect.DeepEqual(nowPolicy, endPolicy) {
		t.Fatalf("%s should have ended as %s but was %s", key, endPolicy, nowPolicy)
	}
}

func TestIngressPolicyNames(t *testing.T) {
	m := NewManagerImpl()
	testPolicyNames(m, IngressPolicyNamesKey, TemplateIngressPolicyNames, m.IngressPolicyNames, t)
}

func TestEgressPolicyNames(t *testing.T) {
	m := NewManagerImpl()
	testPolicyNames(m, EgressPolicyNamesKey, TemplateEgressPolicyNames, m.EgressPolicyNames, t)
}

func TestChainCreationPolicyNames(t *testing.T) {
	m := NewManagerImpl()
	testPolicyNames(m, ChainCreationPolicyNamesKey, TemplateChainCreationPolicyNames, m.ChainCreationPolicyNames, t)
}

func TestEmptyChainCreationPolicyNames(t *testing.T) {
	m := NewManagerImpl()

	m.BeginConfig()

	err := m.ProposeConfig(TemplateChainCreationPolicyNames(nil))
	if err != nil {
		t.Fatalf("Error applying valid config: %s", err)
	}

	m.CommitConfig()

	if m.ChainCreationPolicyNames() == nil {
		t.Fatalf("Should have gotten back empty slice, not nil")
	}
}
