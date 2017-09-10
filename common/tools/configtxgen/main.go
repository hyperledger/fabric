/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/common/tools/configtxgen/metadata"
	"github.com/hyperledger/fabric/common/tools/configtxgen/provisional"
	"github.com/hyperledger/fabric/common/tools/protolator"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"

	logging "github.com/op/go-logging"
)

var exitCode = 0

var logger = flogging.MustGetLogger("common/tools/configtxgen")

func doOutputBlock(config *genesisconfig.Profile, channelID string, outputBlock string) error {
	pgen := provisional.New(config)
	logger.Info("Generating genesis block")
	if config.Orderer == nil {
		return fmt.Errorf("config does not contain an Orderers section, necessary for all config blocks, aborting")
	}
	if config.Consortiums == nil {
		logger.Warning("Genesis block does not contain a consortiums group definition.  This block cannot be used for orderer bootstrap.")
	}
	genesisBlock := pgen.GenesisBlockForChannel(channelID)
	logger.Info("Writing genesis block")
	err := ioutil.WriteFile(outputBlock, utils.MarshalOrPanic(genesisBlock), 0644)
	if err != nil {
		return fmt.Errorf("Error writing genesis block: %s", err)
	}
	return nil
}

func doOutputChannelCreateTx(conf *genesisconfig.Profile, channelID string, outputChannelCreateTx string) error {
	logger.Info("Generating new channel configtx")

	if conf.Application == nil {
		return fmt.Errorf("Cannot define a new channel with no Application section")
	}

	if conf.Consortium == "" {
		return fmt.Errorf("Cannot define a new channel with no Consortium value")
	}

	// XXX we ignore the non-application org names here, once the tool supports configuration updates
	// we should come up with a cleaner way to handle this, but leaving as is for the moment to not break
	// backwards compatibility
	var orgNames []string
	for _, org := range conf.Application.Organizations {
		orgNames = append(orgNames, org.Name)
	}
	configtx, err := channelconfig.MakeChainCreationTransaction(channelID, conf.Consortium, nil, orgNames...)
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

func doOutputAnchorPeersUpdate(conf *genesisconfig.Profile, channelID string, outputAnchorPeersUpdate string, asOrg string) error {
	logger.Info("Generating anchor peer update")
	if asOrg == "" {
		return fmt.Errorf("Must specify an organization to update the anchor peer for")
	}

	if conf.Application == nil {
		return fmt.Errorf("Cannot update anchor peers without an application section")
	}

	var org *genesisconfig.Organization
	for _, iorg := range conf.Application.Organizations {
		if iorg.Name == asOrg {
			org = iorg
		}
	}

	if org == nil {
		return fmt.Errorf("No organization name matching: %s", asOrg)
	}

	anchorPeers := make([]*pb.AnchorPeer, len(org.AnchorPeers))
	for i, anchorPeer := range org.AnchorPeers {
		anchorPeers[i] = &pb.AnchorPeer{
			Host: anchorPeer.Host,
			Port: int32(anchorPeer.Port),
		}
	}

	configGroup := channelconfig.TemplateAnchorPeers(org.Name, anchorPeers)
	configGroup.Groups[channelconfig.ApplicationGroupKey].Groups[org.Name].Values[channelconfig.AnchorPeersKey].ModPolicy = channelconfig.AdminsPolicyKey
	configUpdate := &cb.ConfigUpdate{
		ChannelId: channelID,
		WriteSet:  configGroup,
		ReadSet:   cb.NewConfigGroup(),
	}

	// Add all the existing config to the readset
	configUpdate.ReadSet.Groups[channelconfig.ApplicationGroupKey] = cb.NewConfigGroup()
	configUpdate.ReadSet.Groups[channelconfig.ApplicationGroupKey].Version = 1
	configUpdate.ReadSet.Groups[channelconfig.ApplicationGroupKey].ModPolicy = channelconfig.AdminsPolicyKey
	configUpdate.ReadSet.Groups[channelconfig.ApplicationGroupKey].Groups[org.Name] = cb.NewConfigGroup()
	configUpdate.ReadSet.Groups[channelconfig.ApplicationGroupKey].Groups[org.Name].Values[channelconfig.MSPKey] = &cb.ConfigValue{}
	configUpdate.ReadSet.Groups[channelconfig.ApplicationGroupKey].Groups[org.Name].Policies[channelconfig.ReadersPolicyKey] = &cb.ConfigPolicy{}
	configUpdate.ReadSet.Groups[channelconfig.ApplicationGroupKey].Groups[org.Name].Policies[channelconfig.WritersPolicyKey] = &cb.ConfigPolicy{}
	configUpdate.ReadSet.Groups[channelconfig.ApplicationGroupKey].Groups[org.Name].Policies[channelconfig.AdminsPolicyKey] = &cb.ConfigPolicy{}

	// Add all the existing at the same versions to the writeset
	configUpdate.WriteSet.Groups[channelconfig.ApplicationGroupKey].Version = 1
	configUpdate.WriteSet.Groups[channelconfig.ApplicationGroupKey].ModPolicy = channelconfig.AdminsPolicyKey
	configUpdate.WriteSet.Groups[channelconfig.ApplicationGroupKey].Groups[org.Name].Version = 1
	configUpdate.WriteSet.Groups[channelconfig.ApplicationGroupKey].Groups[org.Name].ModPolicy = channelconfig.AdminsPolicyKey
	configUpdate.WriteSet.Groups[channelconfig.ApplicationGroupKey].Groups[org.Name].Values[channelconfig.MSPKey] = &cb.ConfigValue{}
	configUpdate.WriteSet.Groups[channelconfig.ApplicationGroupKey].Groups[org.Name].Policies[channelconfig.ReadersPolicyKey] = &cb.ConfigPolicy{}
	configUpdate.WriteSet.Groups[channelconfig.ApplicationGroupKey].Groups[org.Name].Policies[channelconfig.WritersPolicyKey] = &cb.ConfigPolicy{}
	configUpdate.WriteSet.Groups[channelconfig.ApplicationGroupKey].Groups[org.Name].Policies[channelconfig.AdminsPolicyKey] = &cb.ConfigPolicy{}

	configUpdateEnvelope := &cb.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(configUpdate),
	}

	update := &cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
					ChannelId: channelID,
					Type:      int32(cb.HeaderType_CONFIG_UPDATE),
				}),
			},
			Data: utils.MarshalOrPanic(configUpdateEnvelope),
		}),
	}

	logger.Info("Writing anchor peer update")
	err := ioutil.WriteFile(outputAnchorPeersUpdate, utils.MarshalOrPanic(update), 0644)
	if err != nil {
		return fmt.Errorf("Error writing channel anchor peer update: %s", err)
	}
	return nil
}

func doInspectBlock(inspectBlock string) error {
	logger.Info("Inspecting block")
	data, err := ioutil.ReadFile(inspectBlock)
	if err != nil {
		return fmt.Errorf("Could not read block %s", inspectBlock)
	}

	logger.Info("Parsing genesis block")
	block, err := utils.UnmarshalBlock(data)
	if err != nil {
		return fmt.Errorf("error unmarshaling to block: %s", err)
	}
	err = protolator.DeepMarshalJSON(os.Stdout, block)
	if err != nil {
		return fmt.Errorf("malformed block contents: %s", err)
	}
	return nil
}

func doInspectChannelCreateTx(inspectChannelCreateTx string) error {
	logger.Info("Inspecting transaction")
	data, err := ioutil.ReadFile(inspectChannelCreateTx)
	if err != nil {
		return fmt.Errorf("could not read channel create tx: %s", err)
	}

	logger.Info("Parsing transaction")
	env, err := utils.UnmarshalEnvelope(data)
	if err != nil {
		return fmt.Errorf("Error unmarshaling envelope: %s", err)
	}

	err = protolator.DeepMarshalJSON(os.Stdout, env)
	if err != nil {
		return fmt.Errorf("malformed transaction contents: %s", err)
	}

	return nil
}

func main() {
	var outputBlock, outputChannelCreateTx, profile, channelID, inspectBlock, inspectChannelCreateTx, outputAnchorPeersUpdate, asOrg string

	flag.StringVar(&outputBlock, "outputBlock", "", "The path to write the genesis block to (if set)")
	flag.StringVar(&channelID, "channelID", provisional.TestChainID, "The channel ID to use in the configtx")
	flag.StringVar(&outputChannelCreateTx, "outputCreateChannelTx", "", "The path to write a channel creation configtx to (if set)")
	flag.StringVar(&profile, "profile", genesisconfig.SampleInsecureSoloProfile, "The profile from configtx.yaml to use for generation.")
	flag.StringVar(&inspectBlock, "inspectBlock", "", "Prints the configuration contained in the block at the specified path")
	flag.StringVar(&inspectChannelCreateTx, "inspectChannelCreateTx", "", "Prints the configuration contained in the transaction at the specified path")
	flag.StringVar(&outputAnchorPeersUpdate, "outputAnchorPeersUpdate", "", "Creates an config update to update an anchor peer (works only with the default channel creation, and only for the first update)")
	flag.StringVar(&asOrg, "asOrg", "", "Performs the config generation as a particular organization (by name), only including values in the write set that org (likely) has privilege to set")

	version := flag.Bool("version", false, "Show version information")

	flag.Parse()

	// show version
	if *version {
		printVersion()
		os.Exit(exitCode)
	}

	logging.SetLevel(logging.INFO, "")

	// don't need to panic when running via command line
	defer func() {
		if err := recover(); err != nil {
			if strings.Contains(fmt.Sprint(err), "Error reading configuration: Unsupported Config Type") {
				logger.Error("Could not find configtx.yaml. " +
					"Please make sure that FABRIC_CFG_PATH is set to a path " +
					"which contains configtx.yaml")
			}
			os.Exit(1)
		}
	}()

	logger.Info("Loading configuration")
	factory.InitFactories(nil)
	config := genesisconfig.Load(profile)

	if outputBlock != "" {
		if err := doOutputBlock(config, channelID, outputBlock); err != nil {
			logger.Fatalf("Error on outputBlock: %s", err)
		}
	}

	if outputChannelCreateTx != "" {
		if err := doOutputChannelCreateTx(config, channelID, outputChannelCreateTx); err != nil {
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

	if outputAnchorPeersUpdate != "" {
		if err := doOutputAnchorPeersUpdate(config, channelID, outputAnchorPeersUpdate, asOrg); err != nil {
			logger.Fatalf("Error on inspectChannelCreateTx: %s", err)
		}
	}
}

func printVersion() {
	fmt.Println(metadata.GetVersionInfo())
}
