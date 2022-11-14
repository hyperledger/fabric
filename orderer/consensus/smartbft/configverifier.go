/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name Filters -case underscore -output mocks

// Filters applies the filters on the outer envelope
type Filters interface {
	ApplyFilters(channel string, env *cb.Envelope) error
}

//go:generate mockery -dir . -name ChannelConfigTemplator -case underscore -output mocks

// ChannelConfigTemplator returnes a channel creation transaction to the system channel
type ChannelConfigTemplator interface {
	NewChannelConfig(env *cb.Envelope) (channelconfig.Resources, error)
}

//go:generate mockery -dir . -name ConfigUpdateProposer -case underscore -output mocks

// ConfigUpdateProposer produces a ConfigEnvelope
type ConfigUpdateProposer interface {
	ProposeConfigUpdate(channel string, configtx *cb.Envelope) (*cb.ConfigEnvelope, error)
}

//go:generate mockery -dir . -name Bundle -case underscore -output mocks

// Bundle defines the channelconfig resources interface
type Bundle interface {
	channelconfig.Resources
}

//go:generate mockery -dir . -name ConfigTxValidator -case underscore -output mocks

// ConfigTxValidator defines the configtx validator interface
type ConfigTxValidator interface {
	configtx.Validator
}

// ConfigBlockValidator struct
type ConfigBlockValidator struct {
	ChannelConfigTemplator ChannelConfigTemplator
	ConfigUpdateProposer   ConfigUpdateProposer
	ValidatingChannel      string
	Filters                Filters
	Logger                 *flogging.FabricLogger
}

// ValidateConfig validates config from envelope
func (cbv *ConfigBlockValidator) ValidateConfig(envelope *cb.Envelope) error {
	payload, err := protoutil.UnmarshalPayload(envelope.Payload)
	if err != nil {
		return err
	}

	if payload.Header == nil {
		return fmt.Errorf("no header was set")
	}

	if payload.Header.ChannelHeader == nil {
		return fmt.Errorf("no channel header was set")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return fmt.Errorf("channel header unmarshalling error: %s", err)
	}

	switch chdr.Type {
	case int32(cb.HeaderType_CONFIG):
		configEnvelope := &cb.ConfigEnvelope{}
		if err = proto.Unmarshal(payload.Data, configEnvelope); err != nil {
			return fmt.Errorf("data unmarshalling error: %s", err)
		}
		return cbv.verifyConfigUpdateMsg(envelope, configEnvelope, chdr)

	case int32(cb.HeaderType_ORDERER_TRANSACTION):
		env, err := protoutil.UnmarshalEnvelope(payload.Data)
		if err != nil {
			return fmt.Errorf("data unmarshalling error: %s", err)
		}

		configEnvelope := &cb.ConfigEnvelope{}
		_, err = protoutil.UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, configEnvelope)
		if err != nil {
			return fmt.Errorf("data unmarshalling error: %s", err)
		}
		return cbv.verifyConfigUpdateMsg(envelope, configEnvelope, chdr)

	default:
		return errors.Errorf("unexpected envelope type %s", cb.HeaderType_name[chdr.Type])
	}
}

func (cbv *ConfigBlockValidator) verifyConfigUpdateMsg(outEnv *cb.Envelope, confEnv *cb.ConfigEnvelope, chdr *cb.ChannelHeader) error {
	if confEnv == nil || confEnv.LastUpdate == nil || confEnv.Config == nil {
		return errors.New("invalid config envelope")
	}
	envPayload, err := protoutil.UnmarshalPayload(confEnv.LastUpdate.Payload)
	if err != nil {
		return err
	}

	if envPayload.Header == nil {
		return errors.New("inner header is nil")
	}

	if envPayload.Header.ChannelHeader == nil {
		return errors.New("inner channelheader is nil")
	}

	typ := cb.HeaderType(chdr.Type)

	cbv.Logger.Infof("Applying filters for config update of type %s to channel %s", typ, chdr.ChannelId)

	// First apply the filters on the outer envelope, regardless of the type of transaction it is.
	if err := cbv.Filters.ApplyFilters(chdr.ChannelId, outEnv); err != nil {
		return err
	}

	var expectedConfigEnv *cb.ConfigEnvelope
	channelID, err := protoutil.ChannelID(confEnv.LastUpdate)
	if err != nil {
		return errors.Errorf("error extracting channel ID from config update")
	}

	if cbv.ValidatingChannel != channelID {
		if cb.HeaderType(chdr.Type) != cb.HeaderType_ORDERER_TRANSACTION {
			// If we reached here, then it's a Config transaction to the wrong channel, so abort it.
			return errors.Errorf("header type is %s but channel is %s", typ, chdr.ChannelId)
		}

		// Else it's a channel creation transaction to the system channel.
		bundle, err := cbv.ChannelConfigTemplator.NewChannelConfig(confEnv.LastUpdate)
		if err != nil {
			cbv.Logger.Errorf("cannot construct new config from last update: %v", err)
			return err
		}

		expectedConfigEnv, err = bundle.ConfigtxValidator().ProposeConfigUpdate(confEnv.LastUpdate)
		if err != nil {
			cbv.Logger.Errorf("Rejecting config update due to %v", err)
			return err
		}
	} else {
		if cb.HeaderType(chdr.Type) == cb.HeaderType_ORDERER_TRANSACTION {
			return errors.Errorf("expected config transaction but got orderer transaction")
		}
		expectedConfigEnv, err = cbv.ConfigUpdateProposer.ProposeConfigUpdate(chdr.ChannelId, confEnv.LastUpdate)
		if err != nil {
			cbv.Logger.Errorf("Rejecting config proposal due to %v", err)
			return err
		}
	}

	// Extract the Config from the result of ProposeConfigUpdate, and compare it
	// with the pending config.
	if proto.Equal(confEnv.Config, expectedConfigEnv.Config) {
		return nil
	}
	cbv.Logger.Errorf("Pending Config is %v, but it should be %v", confEnv.Config, expectedConfigEnv.Config)
	return errors.Errorf("pending config does not match calculated expected config")
}
