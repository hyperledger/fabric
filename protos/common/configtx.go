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
	"fmt"

	"github.com/golang/protobuf/proto"
)

func NewConfigGroup() *ConfigGroup {
	return &ConfigGroup{
		Groups:   make(map[string]*ConfigGroup),
		Values:   make(map[string]*ConfigValue),
		Policies: make(map[string]*ConfigPolicy),
	}
}

func (cue *ConfigUpdateEnvelope) StaticallyOpaqueFields() []string {
	return []string{"config_update"}
}

func (cue *ConfigUpdateEnvelope) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != cue.StaticallyOpaqueFields()[0] {
		return nil, fmt.Errorf("Not a marshaled field: %s", name)
	}
	return &ConfigUpdate{}, nil
}

func (cs *ConfigSignature) StaticallyOpaqueFields() []string {
	return []string{"signature_header"}
}

func (cs *ConfigSignature) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != cs.StaticallyOpaqueFields()[0] {
		return nil, fmt.Errorf("Not a marshaled field: %s", name)
	}
	return &SignatureHeader{}, nil
}

// DynamicConfigTypes allows for other packages outside of the common package
// to register dynamic config types
var DynamicConfigTypes = map[ConfigType]func(cg *ConfigGroup) proto.Message{
	ConfigType_CHANNEL: func(cg *ConfigGroup) proto.Message {
		return &DynamicChannelGroup{
			ConfigGroup: cg,
		}
	},
}

func (c *Config) DynamicFields() []string {
	return []string{"channel_group"}
}

func (c *Config) DynamicFieldProto(name string, base proto.Message) (proto.Message, error) {
	if name != c.DynamicFields()[0] {
		return nil, fmt.Errorf("Not a dynamic field: %s", name)
	}

	cg, ok := base.(*ConfigGroup)
	if !ok {
		return nil, fmt.Errorf("Config must embed a config group as its dynamic field")
	}

	dm, ok := DynamicConfigTypes[ConfigType(c.Type)]
	if !ok {
		return nil, fmt.Errorf("Unknown config type: %d", c.Type)
	}
	return dm(cg), nil
}

// ConfigUpdateIsolatedDataTypes allows other proto packages to register types for the
// the isolated_data field.  This is necessary to break import cycles.
var ConfigUpdateIsolatedDataTypes = map[string]func(string) proto.Message{}

func (c *ConfigUpdate) StaticallyOpaqueMapFields() []string {
	return []string{"isolated_data"}
}

func (c *ConfigUpdate) StaticallyOpaqueMapFieldProto(name string, key string) (proto.Message, error) {
	if name != c.StaticallyOpaqueMapFields()[0] {
		return nil, fmt.Errorf("Not a statically opaque map field: %s", name)
	}

	mf, ok := ConfigUpdateIsolatedDataTypes[key]
	if !ok {
		return nil, fmt.Errorf("Unknown map key: %s", key)
	}

	return mf(key), nil
}

func (c *ConfigUpdate) DynamicFields() []string {
	return []string{"read_set", "write_set"}
}

func (c *ConfigUpdate) DynamicFieldProto(name string, base proto.Message) (proto.Message, error) {
	if name != c.DynamicFields()[0] && name != c.DynamicFields()[1] {
		return nil, fmt.Errorf("Not a dynamic field: %s", name)
	}

	cg, ok := base.(*ConfigGroup)
	if !ok {
		return nil, fmt.Errorf("Expected base to be *ConfigGroup, got %T", base)
	}

	dm, ok := DynamicConfigTypes[ConfigType(c.Type)]
	if !ok {
		return nil, fmt.Errorf("Unknown config type: %d", c.Type)
	}
	return dm(cg), nil
}

func (cv *ConfigValue) VariablyOpaqueFields() []string {
	return []string{"value"}
}

func (cv *ConfigValue) Underlying() proto.Message {
	return cv
}

func (cg *ConfigGroup) DynamicMapFields() []string {
	return []string{"groups", "values"}
}

func (cg *ConfigGroup) Underlying() proto.Message {
	return cg
}
