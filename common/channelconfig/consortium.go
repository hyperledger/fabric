/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/pkg/errors"
)

const (
	// ChannelCreationPolicyKey is the key used in the consortium config to denote the policy
	// to be used in evaluating whether a channel creation request is authorized
	ChannelCreationPolicyKey = "ChannelCreationPolicy"
)

// ConsortiumProtos holds the config protos for the consortium config
type ConsortiumProtos struct {
	ChannelCreationPolicy *cb.Policy
}

// ConsortiumConfig holds the consoritums configuration information
type ConsortiumConfig struct {
	protos *ConsortiumProtos
	orgs   map[string]Org
}

// NewConsortiumConfig creates a new instance of the consoritums config
func NewConsortiumConfig(consortiumGroup *cb.ConfigGroup, mspConfig *MSPConfigHandler) (*ConsortiumConfig, error) {
	cc := &ConsortiumConfig{
		protos: &ConsortiumProtos{},
		orgs:   make(map[string]Org),
	}

	if err := DeserializeProtoValuesFromGroup(consortiumGroup, cc.protos); err != nil {
		return nil, errors.Wrap(err, "failed to deserialize values")
	}

	for orgName, orgGroup := range consortiumGroup.Groups {
		var err error
		if cc.orgs[orgName], err = NewOrganizationConfig(orgName, orgGroup, mspConfig); err != nil {
			return nil, err
		}
	}

	return cc, nil
}

// Organizations returns the set of organizations in the consortium
func (cc *ConsortiumConfig) Organizations() map[string]Org {
	return cc.orgs
}

// CreationPolicy returns the policy structure used to validate
// the channel creation
func (cc *ConsortiumConfig) ChannelCreationPolicy() *cb.Policy {
	return cc.protos.ChannelCreationPolicy
}
