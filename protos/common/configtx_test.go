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

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigGroup(t *testing.T) {
	var cg *ConfigGroup

	cg = nil

	assert.Nil(t, cg.GetGroups())
	assert.Nil(t, cg.GetValues())
	assert.Nil(t, cg.GetPolicies())

	cg = &ConfigGroup{
		Groups:   make(map[string]*ConfigGroup),
		Values:   make(map[string]*ConfigValue),
		Policies: make(map[string]*ConfigPolicy),
	}
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
	cv := &ConfigValue{}
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

	c = &Config{
		ChannelGroup: &ConfigGroup{},
	}
	assert.NotNil(t, c.GetChannelGroup())

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

	env = &ConfigUpdateEnvelope{
		Signatures: []*ConfigSignature{
			&ConfigSignature{},
		},
	}
	assert.NotNil(t, env.GetSignatures())

	env.Reset()
	assert.Nil(t, env.GetSignatures())

	_ = env.String()
	_, _ = env.Descriptor()
	env.ProtoMessage()
}

func TestConfigUpdate(t *testing.T) {
	var c *ConfigUpdate

	c = nil
	assert.Nil(t, c.GetReadSet())
	assert.Nil(t, c.GetWriteSet())

	c = &ConfigUpdate{
		ReadSet:  &ConfigGroup{},
		WriteSet: &ConfigGroup{},
	}
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
	assert.Nil(t, cp.GetPolicy())

	cp = &ConfigPolicy{
		Policy: &Policy{},
	}
	assert.NotNil(t, cp.GetPolicy())

	cp.Reset()
	assert.Nil(t, cp.GetPolicy())

	_ = cp.String()
	_, _ = cp.Descriptor()
	cp.ProtoMessage()
}

func TestConfigSignature(t *testing.T) {
	cs := &ConfigSignature{}

	cs.Reset()
	_ = cs.String()
	_, _ = cs.Descriptor()
	cs.ProtoMessage()
}
