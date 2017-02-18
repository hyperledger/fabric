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

package application

import (
	"testing"

	api "github.com/hyperledger/fabric/common/configvalues"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"

	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func makeInvalidConfigValue() *cb.ConfigValue {
	return &cb.ConfigValue{
		Value: []byte("Garbage Data"),
	}
}

func groupToKeyValue(configGroup *cb.ConfigGroup) (string, *cb.ConfigValue) {
	for _, group := range configGroup.Groups[GroupKey].Groups {
		for key, value := range group.Values {
			return key, value
		}
	}
	panic("No value encoded")
}

func TestApplicationOrgInterface(t *testing.T) {
	_ = api.ValueProposer(NewApplicationOrgConfig("id", nil))
}

func TestApplicationOrgDoubleBegin(t *testing.T) {
	m := NewApplicationOrgConfig("id", nil)
	m.BeginValueProposals(nil)
	assert.Panics(t, func() { m.BeginValueProposals(nil) }, "Two begins back to back should have caused a panic")
}

func TestApplicationOrgCommitWithoutBegin(t *testing.T) {
	m := NewApplicationOrgConfig("id", nil)
	assert.Panics(t, m.CommitProposals, "Committing without beginning should have caused a panic")
}

func TestApplicationOrgRollback(t *testing.T) {
	m := NewApplicationOrgConfig("id", nil)
	m.pendingConfig = &applicationOrgConfig{}
	m.RollbackProposals()
	assert.Nil(t, m.pendingConfig, "Should have cleared pending config on rollback")
}

func TestApplicationOrgAnchorPeers(t *testing.T) {
	endVal := []*pb.AnchorPeer{
		&pb.AnchorPeer{Host: "foo", Port: 234, Cert: []byte("foocert")},
		&pb.AnchorPeer{Host: "bar", Port: 237, Cert: []byte("barcert")},
	}
	invalidMessage := makeInvalidConfigValue()
	validMessage := TemplateAnchorPeers("id", endVal)
	m := NewApplicationOrgConfig("id", nil)
	m.BeginValueProposals(nil)

	assert.Error(t, m.ProposeValue(AnchorPeersKey, invalidMessage), "Should have failed on invalid message")
	assert.NoError(t, m.ProposeValue(groupToKeyValue(validMessage)), "Should not have failed on invalid message")
	m.CommitProposals()

	assert.Equal(t, m.AnchorPeers(), endVal, "Did not set updated anchor peers")
}
