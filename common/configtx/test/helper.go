/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package test

import (
	"sync"

	cb "github.com/hyperledger/fabric-protos-go/common"
	mspproto "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/protoutil"
	"gopkg.in/yaml.v2"
)

var logger = flogging.MustGetLogger("common.configtx.test")

type profiles struct {
	mutex    sync.Mutex
	profiles map[string]*genesisconfig.Profile
}

func (p *profiles) Load(name string) *genesisconfig.Profile {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if profile, ok := p.profiles[name]; ok {
		return cloneProfile(profile)
	}

	if p.profiles == nil {
		p.profiles = map[string]*genesisconfig.Profile{}
	}

	profile := genesisconfig.Load(name, configtest.GetDevConfigDir())
	p.profiles[name] = profile
	return cloneProfile(profile)
}

func cloneProfile(p *genesisconfig.Profile) *genesisconfig.Profile {
	var result genesisconfig.Profile
	serialized, err := yaml.Marshal(p)
	if err != nil {
		panic(err)
	}

	err = yaml.Unmarshal(serialized, &result)
	if err != nil {
		panic(err)
	}

	return &result
}

var profileCache = &profiles{}

// MakeGenesisBlock creates a genesis block using the test templates for the given chainID
func MakeGenesisBlock(chainID string) (*cb.Block, error) {
	profile := profileCache.Load(genesisconfig.SampleDevModeSoloProfile)
	channelGroup, err := encoder.NewChannelGroup(profile)
	if err != nil {
		logger.Panicf("Error creating channel config: %s", err)
	}

	gb := genesis.NewFactoryImpl(channelGroup).Block(chainID)
	if gb == nil {
		return gb, nil
	}

	txsFilter := util.NewTxValidationFlagsSetValue(len(gb.Data.Data), peer.TxValidationCode_VALID)
	gb.Metadata.Metadata[cb.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter

	return gb, nil
}

// MakeGenesisBlockWithMSPs creates a genesis block using the MSPs provided for the given chainID
func MakeGenesisBlockFromMSPs(chainID string, appMSPConf, ordererMSPConf *mspproto.MSPConfig, appOrgID, ordererOrgID string) (*cb.Block, error) {
	profile := profileCache.Load(genesisconfig.SampleDevModeSoloProfile)
	profile.Orderer.Organizations = nil
	channelGroup, err := encoder.NewChannelGroup(profile)
	if err != nil {
		logger.Panicf("Error creating channel config: %s", err)
	}

	ordererOrg := protoutil.NewConfigGroup()
	ordererOrg.ModPolicy = channelconfig.AdminsPolicyKey
	ordererOrg.Values[channelconfig.MSPKey] = &cb.ConfigValue{
		Value:     protoutil.MarshalOrPanic(channelconfig.MSPValue(ordererMSPConf).Value()),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}

	applicationOrg := protoutil.NewConfigGroup()
	applicationOrg.ModPolicy = channelconfig.AdminsPolicyKey
	applicationOrg.Values[channelconfig.MSPKey] = &cb.ConfigValue{
		Value:     protoutil.MarshalOrPanic(channelconfig.MSPValue(appMSPConf).Value()),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}
	applicationOrg.Values[channelconfig.AnchorPeersKey] = &cb.ConfigValue{
		Value:     protoutil.MarshalOrPanic(channelconfig.AnchorPeersValue([]*pb.AnchorPeer{}).Value()),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}

	channelGroup.Groups[channelconfig.OrdererGroupKey].Groups[ordererOrgID] = ordererOrg
	channelGroup.Groups[channelconfig.ApplicationGroupKey].Groups[appOrgID] = applicationOrg

	return genesis.NewFactoryImpl(channelGroup).Block(chainID), nil
}
