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
	"testing"

	"github.com/hyperledger/fabric/orderer/common/util"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

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
	invalidMessage :=
		&cb.ConfigurationItem{
			Type:  cb.ConfigurationItem_Orderer,
			Key:   ConsensusTypeKey,
			Value: []byte("Garbage Data"),
		}
	validMessage := &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   ConsensusTypeKey,
		Value: util.MarshalOrPanic(&ab.ConsensusType{Type: endType}),
	}
	otherValidMessage := &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   ConsensusTypeKey,
		Value: util.MarshalOrPanic(&ab.ConsensusType{Type: "bar"}),
	}
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
	endBatchSize := 10
	invalidMessage :=
		&cb.ConfigurationItem{
			Type:  cb.ConfigurationItem_Orderer,
			Key:   BatchSizeKey,
			Value: []byte("Garbage Data"),
		}
	zeroBatchSize := &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   BatchSizeKey,
		Value: util.MarshalOrPanic(&ab.BatchSize{Messages: 0}),
	}
	negativeBatchSize := &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   BatchSizeKey,
		Value: util.MarshalOrPanic(&ab.BatchSize{Messages: -1}),
	}
	validMessage := &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   BatchSizeKey,
		Value: util.MarshalOrPanic(&ab.BatchSize{Messages: int32(endBatchSize)}),
	}
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

	err = m.ProposeConfig(zeroBatchSize)
	if err == nil {
		t.Fatalf("Should have rejected batch size of 0")
	}

	err = m.ProposeConfig(negativeBatchSize)
	if err == nil {
		t.Fatalf("Should have rejected negative batch size")
	}

	m.CommitConfig()

	if nowBatchSize := m.BatchSize(); nowBatchSize != endBatchSize {
		t.Fatalf("Got batch size of %d when expecting batch size of %d", nowBatchSize, endBatchSize)
	}

}
