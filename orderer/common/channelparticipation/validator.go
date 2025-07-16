/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation

import (
	"bytes"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// ValidateJoinBlock checks whether this block can be used as a join block for the channel participation API.
// It returns the channel ID.
// It verifies that it is not a system channel by checking that consortiums config does not exist.
// It verifies it is an application channel by checking that the application group exists.
// It returns an error when it cannot be used as a join-block.
func ValidateJoinBlock(configBlock *cb.Block) (channelID string, err error) {
	if !protoutil.IsConfigBlock(configBlock) {
		return "", errors.New("block is not a config block")
	}

	if configBlock.Metadata == nil || len(configBlock.Metadata.Metadata) == 0 {
		return "", errors.New("invalid block: does not have metadata")
	}

	dataHash := protoutil.ComputeBlockDataHash(configBlock.Data)
	if !bytes.Equal(dataHash, configBlock.Header.DataHash) {
		return "", errors.New("invalid block: Header.DataHash is different from Hash(block.Data)")
	}

	envelope, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return "", err
	}

	cryptoProvider := factory.GetDefault()
	bundle, err := channelconfig.NewBundleFromEnvelope(envelope, cryptoProvider)
	if err != nil {
		return "", err
	}

	channelID = bundle.ConfigtxValidator().ChannelID()

	// Check channel type
	_, isSystemChannel := bundle.ConsortiumsConfig()
	if isSystemChannel {
		return "", errors.WithMessage(types.ErrSystemChannelNotSupported, "invalid config: contains consortiums")
	}

	_, isAppChannel := bundle.ApplicationConfig()
	if !isAppChannel {
		return "", errors.New("invalid config: must contain application config")
	}

	return channelID, err
}

// ValidateUpdateConfigEnvelope checks whether this envelope can be used as an update config for the channel participation API.
// It returns the channel ID.
// It verifies that it is not a system channel by checking that consortiums config does not exist.
// It verifies it is an application channel by checking that the application group exists.
// It returns an error when it cannot be used as a update config envelope.
func ValidateUpdateConfigEnvelope(env *cb.Envelope) (channelID string, err error) {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return "", errors.New("bad payload")
	}

	if payload.Header == nil || payload.Header.ChannelHeader == nil {
		return "", errors.New("bad header")
	}

	ch, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return "", errors.New("could not unmarshall channel header")
	}

	if ch.Type != int32(cb.HeaderType_CONFIG_UPDATE) {
		return "", errors.New("bad type")
	}

	if ch.ChannelId == "" {
		return "", errors.New("empty channel id")
	}

	configUpdateEnv, err := protoutil.EnvelopeToConfigUpdate(env)
	if err != nil {
		return "", err
	}

	configUpdate, err := configtx.UnmarshalConfigUpdate(configUpdateEnv.ConfigUpdate)
	if err != nil {
		return "", err
	}

	return configUpdate.ChannelId, nil
}
