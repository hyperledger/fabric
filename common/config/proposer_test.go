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

package config

import (
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

type mockValues struct {
	ProtoMsgMap    map[string]proto.Message
	ValidateReturn error
}

func (v *mockValues) ProtoMsg(key string) (proto.Message, bool) {
	msg, ok := v.ProtoMsgMap[key]
	return msg, ok
}

func (v *mockValues) Validate(interface{}, map[string]ValueProposer) error {
	return v.ValidateReturn
}

func (v *mockValues) Commit() {}

func newMockValues() *mockValues {
	return &mockValues{
		ProtoMsgMap: make(map[string]proto.Message),
	}
}

type mockHandler struct {
	AllocateReturn *mockValues
	NewGroupMap    map[string]ValueProposer
	NewGroupError  error
}

func (h *mockHandler) Allocate() Values {
	return h.AllocateReturn
}

func (h *mockHandler) NewGroup(name string) (ValueProposer, error) {
	group, ok := h.NewGroupMap[name]
	if !ok {
		return nil, fmt.Errorf("Missing group implies error")
	}
	return group, nil
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		AllocateReturn: newMockValues(),
		NewGroupMap:    make(map[string]ValueProposer),
	}
}

func TestDoubleSameBegin(t *testing.T) {
	p := NewProposer(&mockHandler{AllocateReturn: &mockValues{}})
	p.BeginValueProposals(p, nil)
	assert.Panics(t, func() { p.BeginValueProposals(p, nil) }, "Two begins back to back should have caused a panic")
}

func TestDoubleDifferentBegin(t *testing.T) {
	p := NewProposer(&mockHandler{AllocateReturn: &mockValues{}})
	p.BeginValueProposals(t, nil)
	p.BeginValueProposals(p, nil)
	// This function would panic on error
}

func TestCommitWithoutBegin(t *testing.T) {
	p := NewProposer(&mockHandler{AllocateReturn: &mockValues{}})
	assert.Panics(t, func() { p.CommitProposals(t) }, "Commit without begin should have caused a panic")
}

func TestRollback(t *testing.T) {
	p := NewProposer(&mockHandler{AllocateReturn: &mockValues{}})
	p.pending[t] = &config{}
	p.RollbackProposals(t)
	assert.Nil(t, p.pending[t], "Should have cleared pending config on rollback")
}

func TestGoodKeys(t *testing.T) {
	mh := newMockHandler()
	mh.AllocateReturn.ProtoMsgMap["Envelope"] = &cb.Envelope{}
	mh.AllocateReturn.ProtoMsgMap["Payload"] = &cb.Payload{}

	p := NewProposer(mh)
	_, err := p.BeginValueProposals(t, nil)
	assert.NoError(t, err)

	env := &cb.Envelope{Payload: []byte("SOME DATA")}
	pay := &cb.Payload{Data: []byte("SOME OTHER DATA")}

	assert.NoError(t, p.ProposeValue(t, "Envelope", &cb.ConfigValue{Value: utils.MarshalOrPanic(env)}))
	assert.NoError(t, p.ProposeValue(t, "Payload", &cb.ConfigValue{Value: utils.MarshalOrPanic(pay)}))

	assert.Equal(t, mh.AllocateReturn.ProtoMsgMap["Envelope"], env)
	assert.Equal(t, mh.AllocateReturn.ProtoMsgMap["Payload"], pay)
}

func TestBadMarshaling(t *testing.T) {
	mh := newMockHandler()
	mh.AllocateReturn.ProtoMsgMap["Envelope"] = &cb.Envelope{}

	p := NewProposer(mh)
	_, err := p.BeginValueProposals(t, nil)
	assert.NoError(t, err)

	assert.Error(t, p.ProposeValue(t, "Envelope", &cb.ConfigValue{Value: []byte("GARBAGE")}), "Should have errored unmarshaling")
}

func TestBadMissingMessage(t *testing.T) {
	mh := newMockHandler()
	mh.AllocateReturn.ProtoMsgMap["Payload"] = &cb.Payload{}

	p := NewProposer(mh)
	_, err := p.BeginValueProposals(t, nil)
	assert.NoError(t, err)

	assert.Error(t, p.ProposeValue(t, "Envelope", &cb.ConfigValue{Value: utils.MarshalOrPanic(&cb.Envelope{})}), "Should have errored on unexpected message")
}

func TestGroups(t *testing.T) {
	mh := newMockHandler()
	mh.NewGroupMap["foo"] = nil
	mh.NewGroupMap["bar"] = nil

	p := NewProposer(mh)
	_, err := p.BeginValueProposals(t, []string{"foo", "bar"})
	assert.NoError(t, err, "Both groups were present")
	p.CommitProposals(t)

	mh.NewGroupMap = make(map[string]ValueProposer)
	_, err = p.BeginValueProposals(t, []string{"foo", "bar"})
	assert.NoError(t, err, "Should not have tried to recreate the groups")
	p.CommitProposals(t)

	_, err = p.BeginValueProposals(t, []string{"foo", "other"})
	assert.Error(t, err, "Should not have errored when trying to create 'other'")

	_, err = p.BeginValueProposals(t, []string{"foo"})
	assert.NoError(t, err, "Should be able to begin again without rolling back because of error")
}
