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

package configtx

import (
	"fmt"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
)

const (
	msgVersion = int32(0)
	epoch      = 0
)

// Template can be used to facilitate creation of config transactions
type Template interface {
	// Envelope returns a ConfigUpdateEnvelope for the given chainID
	Envelope(chainID string) (*cb.ConfigUpdateEnvelope, error)
}

type simpleTemplate struct {
	configGroup *cb.ConfigGroup
}

// NewSimpleTemplate creates a Template using the supplied ConfigGroups
func NewSimpleTemplate(configGroups ...*cb.ConfigGroup) Template {
	sts := make([]Template, len(configGroups))
	for i, group := range configGroups {
		sts[i] = &simpleTemplate{
			configGroup: group,
		}
	}
	return NewCompositeTemplate(sts...)
}

// Envelope returns a ConfigUpdateEnvelope for the given chainID
func (st *simpleTemplate) Envelope(chainID string) (*cb.ConfigUpdateEnvelope, error) {
	config, err := proto.Marshal(&cb.ConfigUpdate{
		ChannelId: chainID,
		WriteSet:  st.configGroup,
	})

	if err != nil {
		return nil, err
	}

	return &cb.ConfigUpdateEnvelope{
		ConfigUpdate: config,
	}, nil
}

type compositeTemplate struct {
	templates []Template
}

// NewCompositeTemplate creates a Template using the source Templates
func NewCompositeTemplate(templates ...Template) Template {
	return &compositeTemplate{templates: templates}
}

func copyGroup(source *cb.ConfigGroup, target *cb.ConfigGroup) error {
	for key, value := range source.Values {
		_, ok := target.Values[key]
		if ok {
			return fmt.Errorf("Duplicate key: %s", key)
		}
		target.Values[key] = value
	}

	for key, policy := range source.Policies {
		_, ok := target.Policies[key]
		if ok {
			return fmt.Errorf("Duplicate policy: %s", key)
		}
		target.Policies[key] = policy
	}

	for key, group := range source.Groups {
		_, ok := target.Groups[key]
		if !ok {
			newGroup := cb.NewConfigGroup()
			newGroup.ModPolicy = group.ModPolicy
			target.Groups[key] = newGroup
		}

		err := copyGroup(group, target.Groups[key])
		if err != nil {
			return fmt.Errorf("Error copying group %s: %s", key, err)
		}
	}
	return nil
}

// Envelope returns a ConfigUpdateEnvelope for the given chainID
func (ct *compositeTemplate) Envelope(chainID string) (*cb.ConfigUpdateEnvelope, error) {
	channel := cb.NewConfigGroup()

	for i := range ct.templates {
		configEnv, err := ct.templates[i].Envelope(chainID)
		if err != nil {
			return nil, err
		}
		config, err := UnmarshalConfigUpdate(configEnv.ConfigUpdate)
		if err != nil {
			return nil, err
		}
		err = copyGroup(config.WriteSet, channel)
		if err != nil {
			return nil, err
		}
	}

	marshaledConfig, err := proto.Marshal(&cb.ConfigUpdate{
		ChannelId: chainID,
		WriteSet:  channel,
	})
	if err != nil {
		return nil, err
	}

	return &cb.ConfigUpdateEnvelope{ConfigUpdate: marshaledConfig}, nil
}

type modPolicySettingTemplate struct {
	modPolicy string
	template  Template
}

// NewModPolicySettingTemplate wraps another template and sets the ModPolicy of
// every ConfigGroup/ConfigValue/ConfigPolicy without a modPolicy to modPolicy
func NewModPolicySettingTemplate(modPolicy string, template Template) Template {
	return &modPolicySettingTemplate{
		modPolicy: modPolicy,
		template:  template,
	}
}

func setGroupModPolicies(modPolicy string, group *cb.ConfigGroup) {
	if group.ModPolicy == "" {
		group.ModPolicy = modPolicy
	}

	for _, value := range group.Values {
		if value.ModPolicy != "" {
			continue
		}
		value.ModPolicy = modPolicy
	}

	for _, policy := range group.Policies {
		if policy.ModPolicy != "" {
			continue
		}
		policy.ModPolicy = modPolicy
	}

	for _, nextGroup := range group.Groups {
		setGroupModPolicies(modPolicy, nextGroup)
	}
}

func (mpst *modPolicySettingTemplate) Envelope(channelID string) (*cb.ConfigUpdateEnvelope, error) {
	configUpdateEnv, err := mpst.template.Envelope(channelID)
	if err != nil {
		return nil, err
	}

	config, err := UnmarshalConfigUpdate(configUpdateEnv.ConfigUpdate)
	if err != nil {
		return nil, err
	}

	setGroupModPolicies(mpst.modPolicy, config.WriteSet)
	configUpdateEnv.ConfigUpdate = utils.MarshalOrPanic(config)
	return configUpdateEnv, nil
}

type channelCreationTemplate struct {
	consortiumName string
	orgs           []string
}
