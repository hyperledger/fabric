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

func doOutputBlock(pgen provisional.Generator, channelID string, outputBlock string) error {
	logger.Info("Generating genesis block")
	genesisBlock := pgen.GenesisBlockForChannel(channelID)
	logger.Info("Writing genesis block")
	err := ioutil.WriteFile(outputBlock, utils.MarshalOrPanic(genesisBlock), 0644)
	if err != nil {
		return fmt.Errorf("Error writing genesis block: %s", err)
	}
	return nil
}

func doOutputChannelCreateTx(pgen provisional.Generator, channelID string, outputChannelCreateTx string) error {
	logger.Info("Generating new channel configtx")
	// TODO, use actual MSP eventually
	signer, err := msp.NewNoopMsp().GetDefaultSigningIdentity()
	if err != nil {
		return fmt.Errorf("Error getting signing identity: %s", err)
	}
	configtx, err := configtx.MakeChainCreationTransaction(provisional.AcceptAllPolicyKey, channelID, signer, pgen.ChannelTemplate())
	if err != nil {
		return fmt.Errorf("Error generating configtx: %s", err)
	}
	logger.Info("Writing new channel tx")
	err = ioutil.WriteFile(outputChannelCreateTx, utils.MarshalOrPanic(configtx), 0644)
	if err != nil {
		return fmt.Errorf("Error writing channel create tx: %s", err)
	}
	return nil
}

func doInspectBlock(inspectBlock string) error {
	logger.Info("Inspecting block")
	data, err := ioutil.ReadFile(inspectBlock)
	logger.Info("Parsing genesis block")
	block := &cb.Block{}
	err = proto.Unmarshal(data, block)
	if err != nil {
		fmt.Errorf("Error unmarshaling block: %s", err)
	}

	ctx, err := utils.ExtractEnvelope(block, 0)
	if err != nil {
		return fmt.Errorf("Error retrieving configtx from block: %s", err)
	}

	payload, err := utils.UnmarshalPayload(ctx.Payload)
	if err != nil {
		return fmt.Errorf("Error extracting configtx payload: %s", err)
	}

	if payload.Header == nil {
		return fmt.Errorf("Config block did not contain header")
	}

	header, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return fmt.Errorf("Error unmarshaling channel header: %s", err)
	}

	if header.Type != int32(cb.HeaderType_CONFIG) {
		return fmt.Errorf("Bad header type: %d", header.Type)
	}

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return fmt.Errorf("Bad configuration envelope")
	}

	if configEnvelope.Config == nil {
		return fmt.Errorf("ConfigEnvelope contained no config")
	}

	configResult, err := configtx.NewConfigResult(configEnvelope.Config.ChannelGroup, configtx.NewInitializer())
	if err != nil {
		return fmt.Errorf("Error parsing configuration: %s", err)
	}

	buffer := &bytes.Buffer{}
	err = json.Indent(buffer, []byte(configResult.JSON()), "", "    ")
	if err != nil {
		return fmt.Errorf("Error in output JSON (usually a programming bug): %s", err)
	}

	fmt.Printf("Config for channel: %s at sequence %d\n", header.ChannelId, configEnvelope.Config.Sequence)

	fmt.Println(buffer.String())
	return nil
}

func doInspectChannelCreateTx(inspectChannelCreateTx string) error {
	logger.Info("Inspecting transaction")
	data, err := ioutil.ReadFile(inspectChannelCreateTx)
	logger.Info("Parsing transaction")
	env, err := utils.UnmarshalEnvelope(data)
	if err != nil {
		return fmt.Errorf("Error unmarshaling envelope: %s", err)
	}

	payload, err := utils.UnmarshalPayload(env.Payload)
	if err != nil {
		return fmt.Errorf("Error extracting configtx payload: %s", err)
	}

	if payload.Header == nil {
		return fmt.Errorf("Config block did not contain header")
	}

	header, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return fmt.Errorf("Error unmarshaling channel header: %s", err)
	}

	if header.Type != int32(cb.HeaderType_CONFIG_UPDATE) {
		return fmt.Errorf("Bad header type: %d", header.Type)
	}

	configUpdateEnvelope, err := configtx.UnmarshalConfigUpdateEnvelope(payload.Data)
	if err != nil {
		return fmt.Errorf("Bad ConfigUpdateEnvelope")
	}

	configUpdate, err := configtx.UnmarshalConfigUpdate(configUpdateEnvelope.ConfigUpdate)
	if err != nil {
		return fmt.Errorf("ConfigUpdateEnvelope contained no config")
	}

	if configUpdate.ChannelId != header.ChannelId {
		return fmt.Errorf("ConfigUpdateEnvelope was for different channel than envelope: %s vs %s", configUpdate.ChannelId, header.ChannelId)
	}

	configResult, err := configtx.NewConfigResult(configUpdate.WriteSet, configtx.NewInitializer())
	if err != nil {
		return fmt.Errorf("Error parsing configuration: %s", err)
	}

	buffer := &bytes.Buffer{}
	err = json.Indent(buffer, []byte(configResult.JSON()), "", "    ")
	if err != nil {
		return fmt.Errorf("Error in output JSON (usually a programming bug): %s", err)
	}

	fmt.Printf("Config for channel: %s\n", header.ChannelId)

	fmt.Println(buffer.String())
	return nil
}

func main() {
	var outputBlock, outputChannelCreateTx, profile, channelID, inspectBlock, inspectChannelCreateTx string

	flag.StringVar(&outputBlock, "outputBlock", "", "The path to write the genesis block to (if set)")
	flag.StringVar(&channelID, "channelID", provisional.TestChainID, "The channel ID to use in the configtx")
	flag.StringVar(&outputChannelCreateTx, "outputCreateChannelTx", "", "The path to write a channel creation configtx to (if set)")
	flag.StringVar(&profile, "profile", genesisconfig.SampleInsecureProfile, "The profile from configtx.yaml to use for generation.")
	flag.StringVar(&inspectBlock, "inspectBlock", "", "Prints the configuration contained in the block at the specified path")
	flag.StringVar(&inspectChannelCreateTx, "inspectChannelCreateTx", "", "Prints the configuration contained in the transaction at the specified path")

	flag.Parse()

	logging.SetLevel(logging.INFO, "")

	logger.Info("Loading configuration")
	factory.InitFactories(nil)
	config := genesisconfig.Load(profile)
	pgen := provisional.New(config)

	if outputBlock != "" {
		if err := doOutputBlock(pgen, channelID, outputBlock); err != nil {
			logger.Fatalf("Error on outputBlock: %s", err)
		}
	}

	if outputChannelCreateTx != "" {
		if err := doOutputChannelCreateTx(pgen, channelID, outputChannelCreateTx); err != nil {
			logger.Fatalf("Error on outputChannelCreateTx: %s", err)
		}
	}

	if inspectBlock != "" {
		if err := doInspectBlock(inspectBlock); err != nil {
			logger.Fatalf("Error on inspectBlock: %s", err)
		}
	}

	if inspectChannelCreateTx != "" {
		if err := doInspectChannelCreateTx(inspectChannelCreateTx); err != nil {
			logger.Fatalf("Error on inspectChannelCreateTx: %s", err)
		}
	}

}
