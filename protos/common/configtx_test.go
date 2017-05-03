/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigGroup(t *testing.T) {
	var cg *ConfigGroup

	cg = nil

	assert.Equal(t, uint64(0), cg.GetVersion())
	assert.Equal(t, "", cg.GetModPolicy())
	assert.Nil(t, cg.GetGroups())
	assert.Nil(t, cg.GetValues())
	assert.Nil(t, cg.GetPolicies())

	cg = &ConfigGroup{
		Version:   uint64(1),
		ModPolicy: "ModPolicy",
		Groups:    make(map[string]*ConfigGroup),
		Values:    make(map[string]*ConfigValue),
		Policies:  make(map[string]*ConfigPolicy),
	}
	assert.Equal(t, uint64(1), cg.GetVersion())
	assert.Equal(t, "ModPolicy", cg.GetModPolicy())
	assert.NotNil(t, cg.GetGroups())
	assert.NotNil(t, cg.GetValues())
	assert.NotNil(t, cg.GetPolicies())

	cg = NewConfigGroup()
	cg.Groups["test"] = NewConfigGroup()
	cg.Values["test"] = &ConfigValue{}
	cg.Policies["test"] = &ConfigPolicy{}
	_, ok := cg.GetGroups()["test"]
	assert.Equal(t, true, ok)
	_, ok = cg.GetValues()["test"]
	assert.Equal(t, true, ok)
	_, ok = cg.GetPolicies()["test"]
	assert.Equal(t, true, ok)

	cg.Reset()
	assert.Nil(t, cg.GetGroups())

	_ = cg.String()
	_, _ = cg.Descriptor()
	cg.ProtoMessage()
}

func TestConfigValue(t *testing.T) {
	var cv *ConfigValue
	cv = nil
	assert.Equal(t, uint64(0), cv.GetVersion())
	assert.Equal(t, "", cv.GetModPolicy())
	assert.Nil(t, cv.GetValue())
	cv = &ConfigValue{
		Version:   uint64(1),
		ModPolicy: "ModPolicy",
		Value:     []byte{},
	}
	assert.Equal(t, uint64(1), cv.GetVersion())
	assert.Equal(t, "ModPolicy", cv.GetModPolicy())
	assert.NotNil(t, cv.GetValue())
	cv.Reset()
	_ = cv.String()
	_, _ = cv.Descriptor()
	cv.ProtoMessage()
}

func TestConfigEnvelope(t *testing.T) {
	var env *ConfigEnvelope

	env = nil
	assert.Nil(t, env.GetConfig())
	assert.Nil(t, env.GetLastUpdate())

	env = &ConfigEnvelope{
		Config:     &Config{},
		LastUpdate: &Envelope{},
	}
	assert.NotNil(t, env.GetConfig())
	assert.NotNil(t, env.GetLastUpdate())

	env.Reset()
	assert.Nil(t, env.GetConfig())

	_ = env.String()
	_, _ = env.Descriptor()
	env.ProtoMessage()
}

func TestConfigGroupSchema(t *testing.T) {
	var cgs *ConfigGroupSchema

	cgs = nil
	assert.Nil(t, cgs.GetGroups())
	assert.Nil(t, cgs.GetPolicies())
	assert.Nil(t, cgs.GetValues())

	cgs = &ConfigGroupSchema{
		Groups:   make(map[string]*ConfigGroupSchema),
		Values:   make(map[string]*ConfigValueSchema),
		Policies: make(map[string]*ConfigPolicySchema),
	}
	assert.NotNil(t, cgs.GetGroups())
	assert.NotNil(t, cgs.GetPolicies())
	assert.NotNil(t, cgs.GetValues())

	cgs.Reset()
	assert.Nil(t, cgs.GetGroups())

	_ = cgs.String()
	_, _ = cgs.Descriptor()
	cgs.ProtoMessage()
}

func TestGenericSchema(t *testing.T) {
	cvs := &ConfigValueSchema{}
	cvs.Reset()
	_ = cvs.String()
	_, _ = cvs.Descriptor()
	cvs.ProtoMessage()

	cps := &ConfigPolicySchema{}
	cps.Reset()
	_ = cps.String()
	_, _ = cps.Descriptor()
	cps.ProtoMessage()
}

func TestConfig(t *testing.T) {
	var c *Config

	c = nil
	assert.Nil(t, c.GetChannelGroup())
	assert.Equal(t, uint64(0), c.GetSequence())

	c = &Config{
		ChannelGroup: &ConfigGroup{},
		Sequence:     uint64(1),
	}
	assert.NotNil(t, c.GetChannelGroup())
	assert.Equal(t, uint64(1), c.GetSequence())

	c.Reset()
	assert.Nil(t, c.GetChannelGroup())

	_ = c.String()
	_, _ = c.Descriptor()
	c.ProtoMessage()
}

func TestConfigUpdateEnvelope(t *testing.T) {
	var env *ConfigUpdateEnvelope

	env = nil
	assert.Nil(t, env.GetSignatures())
	assert.Nil(t, env.GetConfigUpdate())

	env = &ConfigUpdateEnvelope{
		Signatures: []*ConfigSignature{
			{},
		},
		ConfigUpdate: []byte("configupdate"),
	}
	assert.NotNil(t, env.GetSignatures())
	assert.NotNil(t, env.GetConfigUpdate())

	env.Reset()
	assert.Nil(t, env.GetSignatures())

	_ = env.String()
	_, _ = env.Descriptor()
	env.ProtoMessage()
}

func TestConfigUpdate(t *testing.T) {
	var c *ConfigUpdate

	c = nil
	assert.Equal(t, "", c.GetChannelId())
	assert.Nil(t, c.GetReadSet())
	assert.Nil(t, c.GetWriteSet())

	c = &ConfigUpdate{
		ChannelId: "ChannelId",
		ReadSet:   &ConfigGroup{},
		WriteSet:  &ConfigGroup{},
	}
	assert.Equal(t, "ChannelId", c.GetChannelId())
	assert.NotNil(t, c.GetReadSet())
	assert.NotNil(t, c.GetWriteSet())

	c.Reset()
	assert.Nil(t, c.GetReadSet())

	_ = c.String()
	_, _ = c.Descriptor()
	c.ProtoMessage()
}

func TestConfigPolicy(t *testing.T) {
	var cp *ConfigPolicy

	cp = nil
	assert.Equal(t, uint64(0), cp.GetVersion())
	assert.Nil(t, cp.GetPolicy())
	assert.Equal(t, "", cp.GetModPolicy())

	cp = &ConfigPolicy{
		Version:   uint64(1),
		Policy:    &Policy{},
		ModPolicy: "ModPolicy",
	}
	assert.Equal(t, uint64(1), cp.GetVersion())
	assert.NotNil(t, cp.GetPolicy())
	assert.Equal(t, "ModPolicy", cp.GetModPolicy())

	cp.Reset()
	assert.Nil(t, cp.GetPolicy())

	_ = cp.String()
	_, _ = cp.Descriptor()
	cp.ProtoMessage()
}

func TestConfigSignature(t *testing.T) {
	var cs *ConfigSignature

	cs = nil
	assert.Nil(t, cs.GetSignature())
	assert.Nil(t, cs.GetSignatureHeader())
	cs = &ConfigSignature{
		Signature:       []byte{},
		SignatureHeader: []byte{},
	}
	assert.NotNil(t, cs.GetSignature())
	assert.NotNil(t, cs.GetSignatureHeader())

	cs.Reset()
	assert.Nil(t, cs.GetSignature())
	_ = cs.String()
	_, _ = cs.Descriptor()
	cs.ProtoMessage()
}
