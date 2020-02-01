/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	cb "github.com/hyperledger/fabric-protos-go/common"
)

const (
	// ConsortiumsGroupKey is the group name for the consortiums config
	ConsortiumsGroupKey = "Consortiums"
)

// ConsortiumsConfig holds the consoritums configuration information
type ConsortiumsConfig struct {
	consortiums map[string]Consortium
}

// NewConsortiumsConfig creates a new instance of the consoritums config
func NewConsortiumsConfig(consortiumsGroup *cb.ConfigGroup, mspConfig *MSPConfigHandler) (*ConsortiumsConfig, error) {
	cc := &ConsortiumsConfig{
		consortiums: make(map[string]Consortium),
	}

	for consortiumName, consortiumGroup := range consortiumsGroup.Groups {
		var err error
		if cc.consortiums[consortiumName], err = NewConsortiumConfig(consortiumGroup, mspConfig); err != nil {
			return nil, err
		}
	}
	return cc, nil
}

// Consortiums returns a map of the current consortiums
func (cc *ConsortiumsConfig) Consortiums() map[string]Consortium {
	return cc.consortiums
}
