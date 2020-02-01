/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commonext

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
)

type ConfigUpdateEnvelope struct{ *common.ConfigUpdateEnvelope }

func (cue *ConfigUpdateEnvelope) Underlying() proto.Message {
	return cue.ConfigUpdateEnvelope
}

func (cue *ConfigUpdateEnvelope) StaticallyOpaqueFields() []string {
	return []string{"config_update"}
}

func (cue *ConfigUpdateEnvelope) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != cue.StaticallyOpaqueFields()[0] {
		return nil, fmt.Errorf("Not a marshaled field: %s", name)
	}
	return &common.ConfigUpdate{}, nil
}

type ConfigSignature struct{ *common.ConfigSignature }

func (cs *ConfigSignature) Underlying() proto.Message {
	return cs.ConfigSignature
}

func (cs *ConfigSignature) StaticallyOpaqueFields() []string {
	return []string{"signature_header"}
}

func (cs *ConfigSignature) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != cs.StaticallyOpaqueFields()[0] {
		return nil, fmt.Errorf("Not a marshaled field: %s", name)
	}
	return &common.SignatureHeader{}, nil
}

type Config struct{ *common.Config }

func (c *Config) Underlying() proto.Message {
	return c.Config
}

func (c *Config) DynamicFields() []string {
	return []string{"channel_group"}
}

func (c *Config) DynamicFieldProto(name string, base proto.Message) (proto.Message, error) {
	if name != c.DynamicFields()[0] {
		return nil, fmt.Errorf("Not a dynamic field: %s", name)
	}

	cg, ok := base.(*common.ConfigGroup)
	if !ok {
		return nil, fmt.Errorf("Config must embed a config group as its dynamic field")
	}

	return &DynamicChannelGroup{ConfigGroup: cg}, nil
}

// ConfigUpdateIsolatedDataTypes allows other proto packages to register types for the
// the isolated_data field.  This is necessary to break import cycles.
var ConfigUpdateIsolatedDataTypes = map[string]func(string) proto.Message{}

type ConfigUpdate struct{ *common.ConfigUpdate }

func (c *ConfigUpdate) Underlying() proto.Message {
	return c.ConfigUpdate
}

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

	cg, ok := base.(*common.ConfigGroup)
	if !ok {
		return nil, fmt.Errorf("Expected base to be *ConfigGroup, got %T", base)
	}

	return &DynamicChannelGroup{ConfigGroup: cg}, nil
}
