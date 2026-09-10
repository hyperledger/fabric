/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"fmt"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	protosorderer "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer/smartbft"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

//go:generate mockery -dir . -name Filters -case underscore -output mocks

// Filters applies the filters on the outer envelope
type Filters interface {
	ApplyFilters(channel string, env *common.Envelope) error
}

//go:generate mockery -dir . -name ConfigUpdateProposer -case underscore -output mocks

// ConfigUpdateProposer produces a ConfigEnvelope
type ConfigUpdateProposer interface {
	ProposeConfigUpdate(channel string, configtx *common.Envelope) (*common.ConfigEnvelope, error)
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
func (cbv *ConfigBlockValidator) ValidateConfig(envelope *common.Envelope) error {
	payload, err := protoutil.UnmarshalPayload(envelope.GetPayload())
	if err != nil {
		return err
	}

	if payload.GetHeader() == nil {
		return fmt.Errorf("no header was set")
	}

	if payload.Header.ChannelHeader == nil {
		return fmt.Errorf("no channel header was set")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payload.GetHeader().GetChannelHeader())
	if err != nil {
		return fmt.Errorf("channel header unmarshalling error: %s", err)
	}

	switch chdr.GetType() {
	case int32(common.HeaderType_CONFIG):
		configEnvelope := &common.ConfigEnvelope{}
		if err = proto.Unmarshal(payload.GetData(), configEnvelope); err != nil {
			return fmt.Errorf("data unmarshalling error: %s", err)
		}
		return cbv.verifyConfigUpdateMsg(envelope, configEnvelope, chdr)
	default:
		return errors.Errorf("unexpected envelope type %s", common.HeaderType_name[chdr.GetType()])
	}
}

func (cbv *ConfigBlockValidator) checkConsentersMatchPolicy(conf *common.Config) error {
	if conf == nil {
		return fmt.Errorf("empty Config")
	}

	if conf.GetChannelGroup() == nil {
		return fmt.Errorf("empty channel group")
	}

	if len(conf.GetChannelGroup().GetGroups()) == 0 {
		return fmt.Errorf("no groups in channel group")
	}

	if conf.GetChannelGroup().GetGroups()["Orderer"] == nil {
		return fmt.Errorf("no 'Orderer' group in channel groups")
	}

	if len(conf.GetChannelGroup().GetGroups()["Orderer"].GetValues()) == 0 {
		return fmt.Errorf("no values in 'Orderer' group")
	}

	if conf.GetChannelGroup().GetGroups()["Orderer"].GetValues()["Orderers"] == nil {
		return fmt.Errorf("no values in 'Orderer' group")
	}

	ords := &common.Orderers{}
	if err := proto.Unmarshal(conf.GetChannelGroup().GetGroups()["Orderer"].GetValues()["Orderers"].GetValue(), ords); err != nil {
		return err
	}

	n := len(ords.GetConsenterMapping())
	f := (n - 1) / 3

	var identities []*msp.MSPPrincipal
	var pols []*common.SignaturePolicy
	for i, consenter := range ords.GetConsenterMapping() {
		if consenter == nil {
			return fmt.Errorf("consenter %d in the mapping is empty", i)
		}
		pols = append(pols, &common.SignaturePolicy{
			Type: &common.SignaturePolicy_SignedBy{
				SignedBy: int32(i),
			},
		})
		identities = append(identities, &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_IDENTITY,
			Principal:               protoutil.MarshalOrPanic(&msp.SerializedIdentity{Mspid: consenter.GetMspId(), IdBytes: consenter.GetIdentity()}),
		})
	}

	quorumSize := policies.ComputeBFTQuorum(n, f)
	sp := &common.SignaturePolicyEnvelope{
		Rule:       policydsl.NOutOf(int32(quorumSize), pols),
		Identities: identities,
	}

	expectedConfigPol := &common.Policy{
		Type:  int32(common.Policy_SIGNATURE),
		Value: protoutil.MarshalOrPanic(sp),
	}

	if len(conf.GetChannelGroup().GetGroups()["Orderer"].GetPolicies()) == 0 {
		return fmt.Errorf("empty policies in 'Orderer' group")
	}

	if conf.GetChannelGroup().GetGroups()["Orderer"].GetPolicies()["BlockValidation"] == nil {
		return fmt.Errorf("block validation policy is not found in the policies of 'Orderer' group")
	}

	actualPolicy := conf.GetChannelGroup().GetGroups()["Orderer"].GetPolicies()["BlockValidation"].GetPolicy()

	if !proto.Equal(expectedConfigPol, actualPolicy) {
		return fmt.Errorf("block validation policy should be a signature policy: %v but it is %v instead", expectedConfigPol, actualPolicy)
	}

	consensusTypeConfigValue := conf.GetChannelGroup().GetGroups()["Orderer"].GetValues()["ConsensusType"]

	if consensusTypeConfigValue == nil {
		return fmt.Errorf("missing consensus type property in config")
	}

	consensusTypeValue := &protosorderer.ConsensusType{}
	if err := proto.Unmarshal(consensusTypeConfigValue.GetValue(), consensusTypeValue); err != nil {
		return fmt.Errorf("invalid consensus type property in config: %v", err)
	}

	configOptions := &smartbft.Options{}
	if err := proto.Unmarshal(consensusTypeValue.GetMetadata(), configOptions); err != nil {
		return fmt.Errorf("invalid options encoded in consensus metadata: %v", err)
	}

	if configOptions.GetLeaderRotation() == smartbft.Options_ROTATION_ON {
		return fmt.Errorf("leader rotation must be turned off for this version or be unspecified")
	}

	return nil
}

func (cbv *ConfigBlockValidator) verifyConfigUpdateMsg(outEnv *common.Envelope, confEnv *common.ConfigEnvelope, chdr *common.ChannelHeader) error {
	if confEnv == nil || confEnv.GetLastUpdate() == nil || confEnv.GetConfig() == nil {
		return errors.New("invalid config envelope")
	}
	envPayload, err := protoutil.UnmarshalPayload(confEnv.GetLastUpdate().GetPayload())
	if err != nil {
		return err
	}

	if envPayload.GetHeader() == nil {
		return errors.New("inner header is nil")
	}

	if envPayload.Header.ChannelHeader == nil {
		return errors.New("inner channelheader is nil")
	}

	typ := common.HeaderType(chdr.GetType())

	cbv.Logger.Infof("Applying filters for config update of type %s to channel %s", typ, chdr.GetChannelId())

	// First apply the filters on the outer envelope, regardless of the type of transaction it is.
	if err := cbv.Filters.ApplyFilters(chdr.GetChannelId(), outEnv); err != nil {
		return err
	}

	var expectedConfigEnv *common.ConfigEnvelope
	channelID, err := protoutil.ChannelID(confEnv.GetLastUpdate())
	if err != nil {
		return errors.Errorf("error extracting channel ID from config update")
	}

	if cbv.ValidatingChannel != channelID {
		return errors.Errorf("transaction is aimed at channel %s but our channel is %s", channelID, cbv.ValidatingChannel)
	} else {
		expectedConfigEnv, err = cbv.ConfigUpdateProposer.ProposeConfigUpdate(chdr.GetChannelId(), confEnv.GetLastUpdate())
		if err != nil {
			cbv.Logger.Errorf("Rejecting config proposal due to %v", err)
			return err
		}
	}

	if err := cbv.checkConsentersMatchPolicy(confEnv.GetConfig()); err != nil {
		return err
	}

	// Extract the Config from the result of ProposeConfigUpdate, and compare it
	// with the pending config.
	if proto.Equal(confEnv.GetConfig(), expectedConfigEnv.GetConfig()) {
		return nil
	}
	cbv.Logger.Errorf("Pending Config is %v, but it should be %v", confEnv.GetConfig(), expectedConfigEnv.GetConfig())
	return errors.Errorf("pending config does not match calculated expected config")
}
