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

package main

import (
	"flag"
	"io/ioutil"

	"github.com/hyperledger/fabric/common/configtx"
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/utils"

	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("common/configtx/tool")

func main() {
	var outputBlock, outputChannelCreateTx, profile, channelID string

	flag.StringVar(&outputBlock, "outputBlock", "", "The path to write the genesis block to (if set)")
	flag.StringVar(&channelID, "channelID", provisional.TestChainID, "The channel ID to use in the configtx")
	flag.StringVar(&outputChannelCreateTx, "outputCreateChannelTx", "", "The path to write a channel creation configtx to (if set)")
	flag.StringVar(&profile, "profile", genesisconfig.SampleInsecureProfile, "The profile from configtx.yaml to use for generation.")
	flag.Parse()

	logging.SetLevel(logging.INFO, "")

	logger.Info("Loading configuration")
	config := genesisconfig.Load(profile)
	pgen := provisional.New(config)

	if outputBlock != "" {
		logger.Info("Generating genesis block")
		genesisBlock := pgen.GenesisBlock()
		logger.Info("Writing genesis block")
		err := ioutil.WriteFile(outputBlock, utils.MarshalOrPanic(genesisBlock), 0644)
		if err != nil {
			logger.Errorf("Error writing genesis block: %s", err)
		}
	}

	if outputChannelCreateTx != "" {
		logger.Info("Generating new channel configtx")
		// TODO, use actual MSP eventually
		signer, err := msp.NewNoopMsp().GetDefaultSigningIdentity()
		if err != nil {
			logger.Fatalf("Error getting signing identity: %s", err)
		}
		configtx, err := configtx.MakeChainCreationTransaction(provisional.AcceptAllPolicyKey, channelID, signer, pgen.ChannelTemplate())
		if err != nil {
			logger.Fatalf("Error generating configtx: %s", err)
		}
		logger.Info("Writing new channel tx")
		err = ioutil.WriteFile(outputChannelCreateTx, utils.MarshalOrPanic(configtx), 0644)
		if err != nil {
			logger.Errorf("Error writing channel create tx: %s", err)
		}
	}
}
