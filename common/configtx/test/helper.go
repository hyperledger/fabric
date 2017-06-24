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

package test

import (
	"github.com/hyperledger/fabric/common/config"
	configtxmsp "github.com/hyperledger/fabric/common/config/msp"
	"github.com/hyperledger/fabric/common/configtx"
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/genesis"
	cf "github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	mspproto "github.com/hyperledger/fabric/protos/msp"
)

var logger = flogging.MustGetLogger("common/configtx/test")

const (
	// AcceptAllPolicyKey is the key of the AcceptAllPolicy.
	AcceptAllPolicyKey = "AcceptAllPolicy"
)

func getConfigDir() string {
	mspDir, err := cf.GetDevMspDir()
	if err != nil {
		logger.Panicf("Could not find genesis.yaml, try setting GOPATH correctly")
	}

	return mspDir
}

// MakeGenesisBlock creates a genesis block using the test templates for the given chainID
func MakeGenesisBlock(chainID string) (*cb.Block, error) {
	return genesis.NewFactoryImpl(CompositeTemplate()).Block(chainID)
}

// MakeGenesisBlockWithMSPs creates a genesis block using the MSPs provided for the given chainID
func MakeGenesisBlockFromMSPs(chainID string, appMSPConf, ordererMSPConf *mspproto.MSPConfig,
	appOrgID, ordererOrgID string) (*cb.Block, error) {
	appOrgTemplate := configtx.NewSimpleTemplate(configtxmsp.TemplateGroupMSP([]string{config.ApplicationGroupKey, appOrgID}, appMSPConf))
	ordererOrgTemplate := configtx.NewSimpleTemplate(configtxmsp.TemplateGroupMSP([]string{config.OrdererGroupKey, ordererOrgID}, ordererMSPConf))
	composite := configtx.NewCompositeTemplate(OrdererTemplate(), appOrgTemplate, ApplicationOrgTemplate(), ordererOrgTemplate)
	return genesis.NewFactoryImpl(composite).Block(chainID)
}

// OrderererTemplate returns the test orderer template
func OrdererTemplate() configtx.Template {
	genConf := genesisconfig.Load(genesisconfig.SampleInsecureProfile)
	return provisional.New(genConf).ChannelTemplate()
}

// sampleOrgID apparently _must_ be set to DEFAULT or things break
// Beware when changing!
const sampleOrgID = "DEFAULT"

// ApplicationOrgTemplate returns the SAMPLE org with MSP template
func ApplicationOrgTemplate() configtx.Template {
	mspConf, err := msp.GetLocalMspConfig(getConfigDir(), nil, sampleOrgID)
	if err != nil {
		logger.Panicf("Could not load sample MSP config: %s", err)
	}
	return configtx.NewSimpleTemplate(configtxmsp.TemplateGroupMSP([]string{config.ApplicationGroupKey, sampleOrgID}, mspConf))
}

// OrdererOrgTemplate returns the SAMPLE org with MSP template
func OrdererOrgTemplate() configtx.Template {
	mspConf, err := msp.GetLocalMspConfig(getConfigDir(), nil, sampleOrgID)
	if err != nil {
		logger.Panicf("Could not load sample MSP config: %s", err)
	}
	return configtx.NewSimpleTemplate(configtxmsp.TemplateGroupMSP([]string{config.OrdererGroupKey, sampleOrgID}, mspConf))
}

// CompositeTemplate returns the composite template of peer, orderer, and MSP
func CompositeTemplate() configtx.Template {
	return configtx.NewCompositeTemplate(OrdererTemplate(), ApplicationOrgTemplate(), OrdererOrgTemplate())
}
