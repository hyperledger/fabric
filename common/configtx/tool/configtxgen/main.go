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
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/configtx"
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("common/configtx/tool")

func doOutputBlock(pgen provisional.Generator, channelID string, outputBlock string) {
	logger.Info("Generating genesis block")
	genesisBlock := pgen.GenesisBlockForChannel(channelID)
	logger.Info("Writing genesis block")
	err := ioutil.WriteFile(outputBlock, utils.MarshalOrPanic(genesisBlock), 0644)
	if err != nil {
		logger.Errorf("Error writing genesis block: %s", err)
	}
}

func doOutputChannelCreateTx(pgen provisional.Generator, channelID string, outputChannelCreateTx string) {
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

func doInspectBlock(inspectBlock string) {
	logger.Info("Inspecting block")
	data, err := ioutil.ReadFile(inspectBlock)
	logger.Info("Parsing genesis block")
	block := &cb.Block{}
	err = proto.Unmarshal(data, block)
	if err != nil {
		logger.Fatalf("Error unmarshaling block: %s", err)
	}

	ctx, err := utils.ExtractEnvelope(block, 0)
	if err != nil {
		logger.Fatalf("Error retrieving configtx from block: %s", err)
	}

	payload, err := utils.UnmarshalPayload(ctx.Payload)
	if err != nil {
		logger.Fatalf("Error extracting configtx payload: %s", err)
	}

	if payload.Header == nil {
		logger.Fatalf("Config block did not contain header")
	}

	header, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		logger.Fatalf("Error unmarshaling channel header: %s", err)
	}

	if header.Type != int32(cb.HeaderType_CONFIG) {
		logger.Fatalf("Bad header type: %d", header.Type)
	}

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		logger.Fatalf("Bad configuration envelope")
	}

	if configEnvelope.Config == nil {
		logger.Fatalf("ConfigEnvelope contained no config")
	}

	configResult, err := configtx.NewConfigResult(configEnvelope.Config.ChannelGroup, configtx.NewInitializer())
	if err != nil {
		logger.Fatalf("Error parsing configuration: %s", err)
	}

	buffer := &bytes.Buffer{}
	json.Indent(buffer, []byte(configResult.JSON()), "", "    ")

	fmt.Printf("Config for channel: %s at sequence %d\n", header.ChannelId, configEnvelope.Config.Sequence)

	fmt.Println(buffer.String())
}

func main() {
	var outputBlock, outputChannelCreateTx, profile, channelID, inspectBlock string

	flag.StringVar(&outputBlock, "outputBlock", "", "The path to write the genesis block to (if set)")
	flag.StringVar(&channelID, "channelID", provisional.TestChainID, "The channel ID to use in the configtx")
	flag.StringVar(&outputChannelCreateTx, "outputCreateChannelTx", "", "The path to write a channel creation configtx to (if set)")
	flag.StringVar(&profile, "profile", genesisconfig.SampleInsecureProfile, "The profile from configtx.yaml to use for generation.")
	flag.StringVar(&inspectBlock, "inspectBlock", "", "Prints the configuration contained in the block at the specified path")

	flag.Parse()

	logging.SetLevel(logging.INFO, "")

	logger.Info("Loading configuration")
	factory.InitFactories(nil)
	config := genesisconfig.Load(profile)
	pgen := provisional.New(config)

	if outputBlock != "" {
		doOutputBlock(pgen, channelID, outputBlock)
	}

	if outputChannelCreateTx != "" {
		doOutputChannelCreateTx(pgen, channelID, outputChannelCreateTx)
	}

	if inspectBlock != "" {
		doInspectBlock(inspectBlock)
	}

}
