/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"fmt"

	mspa "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/policydsl"

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
	ConfigUpdateProposer ConfigUpdateProposer
	ValidatingChannel    string
	Filters              Filters
	Logger               *flogging.FabricLogger
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
	default:
		return errors.Errorf("unexpected envelope type %s", cb.HeaderType_name[chdr.Type])
	}
}

func (cbv *ConfigBlockValidator) checkConsentersMatchPolicy(conf *cb.Config) error {
	if conf == nil {
		return fmt.Errorf("empty Config")
	}

	if conf.ChannelGroup == nil {
		return fmt.Errorf("empty channel group")
	}

	if len(conf.ChannelGroup.Groups) == 0 {
		return fmt.Errorf("no groups in channel group")
	}

	if conf.ChannelGroup.Groups["Orderer"] == nil {
		return fmt.Errorf("no 'Orderer' group in channel groups")
	}

	if len(conf.ChannelGroup.Groups["Orderer"].Values) == 0 {
		return fmt.Errorf("no values in 'Orderer' group")
	}

	if conf.ChannelGroup.Groups["Orderer"].Values["Orderers"] == nil {
		return fmt.Errorf("no values in 'Orderer' group")
	}

	ords := &cb.Orderers{}
	if err := proto.Unmarshal(conf.ChannelGroup.Groups["Orderer"].Values["Orderers"].Value, ords); err != nil {
		return err
	}

	n := len(ords.ConsenterMapping)
	f := (n - 1) / 3

	var identities []*mspa.MSPPrincipal
	var pols []*cb.SignaturePolicy
	for i, consenter := range ords.ConsenterMapping {
		if consenter == nil {
			return fmt.Errorf("consenter %d in the mapping is empty", i)
		}
		pols = append(pols, &cb.SignaturePolicy{
			Type: &cb.SignaturePolicy_SignedBy{
				SignedBy: int32(i),
			},
		})
		identities = append(identities, &mspa.MSPPrincipal{
			PrincipalClassification: mspa.MSPPrincipal_IDENTITY,
			Principal:               protoutil.MarshalOrPanic(&mspa.SerializedIdentity{Mspid: consenter.MspId, IdBytes: consenter.Identity}),
		})
	}

	quorumSize := policies.ComputeBFTQuorum(n, f)
	sp := &cb.SignaturePolicyEnvelope{
		Rule:       policydsl.NOutOf(int32(quorumSize), pols),
		Identities: identities,
	}

	expectedConfigPol := &cb.Policy{
		Type:  int32(cb.Policy_SIGNATURE),
		Value: protoutil.MarshalOrPanic(sp),
	}

	if conf.ChannelGroup.Groups["Orderer"].Policies == nil || len(conf.ChannelGroup.Groups["Orderer"].Policies) == 0 {
		return fmt.Errorf("empty policies in 'Orderer' group")
	}

	if conf.ChannelGroup.Groups["Orderer"].Policies["BlockValidation"] == nil {
		return fmt.Errorf("block validation policy is not found in the policies of 'Orderer' group")
	}

	actualPolicy := conf.ChannelGroup.Groups["Orderer"].Policies["BlockValidation"].Policy

	if !proto.Equal(expectedConfigPol, actualPolicy) {
		return fmt.Errorf("block validation policy should be a signature policy: %v but it is %v instead", expectedConfigPol, actualPolicy)
	}

	return nil
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
		return errors.Errorf("transaction is aimed at channel %s but our channel is %s", channelID, cbv.ValidatingChannel)
	} else {
		expectedConfigEnv, err = cbv.ConfigUpdateProposer.ProposeConfigUpdate(chdr.ChannelId, confEnv.LastUpdate)
		if err != nil {
			cbv.Logger.Errorf("Rejecting config proposal due to %v", err)
			return err
		}
	}

	if err := cbv.checkConsentersMatchPolicy(confEnv.Config); err != nil {
		return err
	}

	// Extract the Config from the result of ProposeConfigUpdate, and compare it
	// with the pending config.
	if proto.Equal(confEnv.Config, expectedConfigEnv.Config) {
		return nil
	}
	cbv.Logger.Errorf("Pending Config is %v, but it should be %v", confEnv.Config, expectedConfigEnv.Config)
	return errors.Errorf("pending config does not match calculated expected config")
}
