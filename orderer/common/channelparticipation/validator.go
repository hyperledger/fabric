/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation

import (
	"errors"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/protoutil"
)

// ValidateJoinBlock checks whether this block can be used as a join block for the channel participation API.
// It returns the channel ID, and whether it is an system channel if it contains consortiums, or otherwise
// an application channel if an application group exists. It returns an error when it cannot be used as a join-block.
func ValidateJoinBlock(configBlock *cb.Block) (channelID string, isAppChannel bool, err error) {
	if !protoutil.IsConfigBlock(configBlock) {
		return "", false, errors.New("block is not a config block")
	}

	envelope, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return "", false, err
	}

	cryptoProvider := factory.GetDefault()
	bundle, err := channelconfig.NewBundleFromEnvelope(envelope, cryptoProvider)
	if err != nil {
		return "", false, err
	}

	channelID = bundle.ConfigtxValidator().ChannelID()

	// Check channel type
	_, isSystemChannel := bundle.ConsortiumsConfig()
	if !isSystemChannel {
		_, isAppChannel = bundle.ApplicationConfig()
		if !isAppChannel {
			return "", false, errors.New("invalid config: must have at least one of application or consortiums")
		}
	}

	return channelID, isAppChannel, err
}
