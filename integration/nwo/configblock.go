/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/tools/configtxlator/update"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	protosorderer "github.com/hyperledger/fabric/protos/orderer"
	ectdraft_protos "github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

// GetConfigBlock retrieves the current config block for a channel
func GetConfigBlock(n *Network, peer *Peer, orderer *Orderer, channel string) *common.Block {
	tempDir, err := ioutil.TempDir("", "getConfigBlock")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	// fetch the config block
	output := filepath.Join(tempDir, "config_block.pb")
	sess, err := n.OrdererAdminSession(orderer, peer, commands.ChannelFetch{
		ChannelID:  channel,
		Block:      "config",
		Orderer:    n.OrdererAddress(orderer, ListenPort),
		OutputFile: output,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Received block: "))

	// unmarshal the config block bytes
	configBlock := UnmarshalBlockFromFile(output)
	return configBlock
}

// GetConfig retrieves the last config of the given channel
func GetConfig(n *Network, peer *Peer, orderer *Orderer, channel string) *common.Config {
	configBlock := GetConfigBlock(n, peer, orderer, channel)
	// unmarshal the envelope bytes
	envelope, err := utils.GetEnvelopeFromBlock(configBlock.Data.Data[0])
	Expect(err).NotTo(HaveOccurred())

	// unmarshal the payload bytes
	payload, err := utils.GetPayload(envelope)
	Expect(err).NotTo(HaveOccurred())

	// unmarshal the config envelope bytes
	configEnv := &common.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnv)
	Expect(err).NotTo(HaveOccurred())

	// clone the config
	return configEnv.Config
}

// UpdateConfig computes, signs, and submits a configuration update and waits
// for the update to complete.
func UpdateConfig(n *Network, orderer *Orderer, channel string, current, updated *common.Config, getConfigBlockFromOrderer bool, submitter *Peer, additionalSigners ...*Peer) {
	tempDir, err := ioutil.TempDir("", "updateConfig")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	// compute update
	configUpdate, err := update.Compute(current, updated)
	Expect(err).NotTo(HaveOccurred())
	configUpdate.ChannelId = channel

	signedEnvelope, err := utils.CreateSignedEnvelope(
		common.HeaderType_CONFIG_UPDATE,
		channel,
		nil, // local signer
		&common.ConfigUpdateEnvelope{ConfigUpdate: utils.MarshalOrPanic(configUpdate)},
		0, // message version
		0, // epoch
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(signedEnvelope).NotTo(BeNil())

	updateFile := filepath.Join(tempDir, "update.pb")
	err = ioutil.WriteFile(updateFile, utils.MarshalOrPanic(signedEnvelope), 0600)
	Expect(err).NotTo(HaveOccurred())

	for _, signer := range additionalSigners {
		sess, err := n.PeerAdminSession(signer, commands.SignConfigTx{File: updateFile})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}

	var currentBlockNumber uint64
	// get current configuration block number
	if getConfigBlockFromOrderer {
		currentBlockNumber = CurrentConfigBlockNumber(n, submitter, orderer, channel)
	} else {
		currentBlockNumber = CurrentConfigBlockNumber(n, submitter, nil, channel)
	}

	sess, err := n.PeerAdminSession(submitter, commands.ChannelUpdate{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, ListenPort),
		File:      updateFile,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Successfully submitted channel update"))

	if getConfigBlockFromOrderer {
		ccb := func() uint64 { return CurrentConfigBlockNumber(n, submitter, orderer, channel) }
		Eventually(ccb, n.EventuallyTimeout).Should(BeNumerically(">", currentBlockNumber))
		return
	}

	// wait for the block to be committed to all peers that
	// have joined the channel
	for _, peer := range n.PeersWithChannel(channel) {
		ccb := func() uint64 { return CurrentConfigBlockNumber(n, peer, nil, channel) }
		Eventually(ccb, n.EventuallyTimeout).Should(BeNumerically(">", currentBlockNumber))
	}
}

// UpdateOrdererConfig computes, signs, and submits a configuration update which requires orderers signature and waits
// for the update to complete.
func UpdateOrdererConfig(n *Network, orderer *Orderer, channel string, current, updated *common.Config, submitter *Peer, additionalSigners ...*Orderer) {
	tempDir, err := ioutil.TempDir("", "updateConfig")
	Expect(err).NotTo(HaveOccurred())
	updateFile := filepath.Join(tempDir, "update.pb")
	defer os.RemoveAll(tempDir)

	ComputeUpdateOrdererConfig(updateFile, n, channel, current, updated, submitter, additionalSigners...)

	currentBlockNumber := CurrentConfigBlockNumber(n, submitter, orderer, channel)

	Eventually(func() string {
		sess, err := n.OrdererAdminSession(orderer, submitter, commands.ChannelUpdate{
			ChannelID: channel,
			Orderer:   n.OrdererAddress(orderer, ListenPort),
			File:      updateFile,
		})
		if err != nil {
			return err.Error()
		}
		sess.Wait(n.EventuallyTimeout)
		if sess.ExitCode() != 0 {
			return fmt.Sprintf("exit code is %d", sess.ExitCode())
		}
		if strings.Contains(string(sess.Err.Contents()), "Successfully submitted channel update") {
			return ""
		}
		return fmt.Sprintf("channel update output: %s", string(sess.Err.Contents()))
	}, n.EventuallyTimeout).Should(BeEmpty())

	// wait for the block to be committed
	ccb := func() uint64 { return CurrentConfigBlockNumber(n, submitter, orderer, channel) }
	Eventually(ccb, n.EventuallyTimeout).Should(BeNumerically(">", currentBlockNumber))
}

// CurrentConfigBlockNumber retrieves the block number from the header of the
// current config block. This can be used to detect when configuration change
// has completed. If an orderer is not provided, the current config block will
// be fetched from the peer.
func CurrentConfigBlockNumber(n *Network, peer *Peer, orderer *Orderer, channel string) uint64 {
	tempDir, err := ioutil.TempDir("", "currentConfigBlock")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	// fetch the config block
	output := filepath.Join(tempDir, "config_block.pb")
	if orderer == nil {
		return CurrentConfigBlockNumberFromPeer(n, peer, channel, output)
	}

	FetchConfigBlock(n, peer, orderer, channel, output)

	// unmarshal the config block bytes
	configBlock := UnmarshalBlockFromFile(output)

	return configBlock.Header.Number
}

// CurrentConfigBlockNumberFromPeer retrieves the block number from the header
// of the peer's current config block.
func CurrentConfigBlockNumberFromPeer(n *Network, peer *Peer, channel, output string) uint64 {
	sess, err := n.PeerAdminSession(peer, commands.ChannelFetch{
		ChannelID:  channel,
		Block:      "config",
		OutputFile: output,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Received block: "))

	configBlock := UnmarshalBlockFromFile(output)

	return configBlock.Header.Number
}

// FetchConfigBlock fetches latest config block.
func FetchConfigBlock(n *Network, peer *Peer, orderer *Orderer, channel string, output string) {
	fetch := func() int {
		sess, err := n.OrdererAdminSession(orderer, peer, commands.ChannelFetch{
			ChannelID:  channel,
			Block:      "config",
			Orderer:    n.OrdererAddress(orderer, ListenPort),
			OutputFile: output,
		})
		Expect(err).NotTo(HaveOccurred())
		code := sess.Wait(n.EventuallyTimeout).ExitCode()
		if code == 0 {
			Expect(sess.Err).To(gbytes.Say("Received block: "))
		}
		return code
	}
	Eventually(fetch, n.EventuallyTimeout).Should(Equal(0))
}

// UpdateOrdererConfigFail computes, signs, and submits a configuration update which requires orderers signature
// and waits for the update to FAIL.
func UpdateOrdererConfigFail(n *Network, orderer *Orderer, channel string, current, updated *common.Config, submitter *Peer, additionalSigners ...*Orderer) {
	tempDir, err := ioutil.TempDir("", "updateConfig")
	Expect(err).NotTo(HaveOccurred())
	updateFile := filepath.Join(tempDir, "update.pb")
	defer os.RemoveAll(tempDir)

	ComputeUpdateOrdererConfig(updateFile, n, channel, current, updated, submitter, additionalSigners...)

	//session should not return with a zero exit code nor with a success response
	sess, err := n.OrdererAdminSession(orderer, submitter, commands.ChannelUpdate{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, ListenPort),
		File:      updateFile,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).ShouldNot(gexec.Exit(0))
	Expect(sess.Err).NotTo(gbytes.Say("Successfully submitted channel update"))
}

func ComputeUpdateOrdererConfig(updateFile string, n *Network, channel string, current, updated *common.Config, submitter *Peer, additionalSigners ...*Orderer) {
	// compute update
	configUpdate, err := update.Compute(current, updated)
	Expect(err).NotTo(HaveOccurred())
	configUpdate.ChannelId = channel

	signedEnvelope, err := utils.CreateSignedEnvelope(
		common.HeaderType_CONFIG_UPDATE,
		channel,
		nil, // local signer
		&common.ConfigUpdateEnvelope{ConfigUpdate: utils.MarshalOrPanic(configUpdate)},
		0, // message version
		0, // epoch
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(signedEnvelope).NotTo(BeNil())

	err = ioutil.WriteFile(updateFile, utils.MarshalOrPanic(signedEnvelope), 0600)
	Expect(err).NotTo(HaveOccurred())

	for _, signer := range additionalSigners {
		sess, err := n.OrdererAdminSession(signer, submitter, commands.SignConfigTx{File: updateFile})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}
}

// UnmarshalBlockFromFile unmarshals a proto encoded block from a file.
func UnmarshalBlockFromFile(blockFile string) *common.Block {
	blockBytes, err := ioutil.ReadFile(blockFile)
	Expect(err).NotTo(HaveOccurred())

	block, err := utils.UnmarshalBlock(blockBytes)
	Expect(err).NotTo(HaveOccurred())

	return block
}

// AddConsenter adds a new consenter to the given channel
func AddConsenter(n *Network, peer *Peer, orderer *Orderer, channel string, consenter ectdraft_protos.Consenter) {
	UpdateEtcdRaftMetadata(n, peer, orderer, channel, func(metadata *ectdraft_protos.ConfigMetadata) {
		metadata.Consenters = append(metadata.Consenters, &consenter)
	})
}

// RemoveConsenter removes a consenter with the given certificate in PEM format from the given channel
func RemoveConsenter(n *Network, peer *Peer, orderer *Orderer, channel string, certificate []byte) {
	UpdateEtcdRaftMetadata(n, peer, orderer, channel, func(metadata *ectdraft_protos.ConfigMetadata) {
		var newConsenters []*ectdraft_protos.Consenter
		for _, consenter := range metadata.Consenters {
			if bytes.Equal(consenter.ClientTlsCert, certificate) || bytes.Equal(consenter.ServerTlsCert, certificate) {
				continue
			}
			newConsenters = append(newConsenters, consenter)
		}

		metadata.Consenters = newConsenters
	})
}

// ConsensusMetadataMutator receives ConsensusType.Metadata and mutates it
type ConsensusMetadataMutator func([]byte) []byte

// MSPMutator receives FabricMSPConfig and mutates it.
type MSPMutator func(config msp.FabricMSPConfig) msp.FabricMSPConfig

// UpdateConsensusMetadata executes a config update that updates the consensus metadata according to the given ConsensusMetadataMutator
func UpdateConsensusMetadata(network *Network, peer *Peer, orderer *Orderer, channel string, mutateMetadata ConsensusMetadataMutator) {
	config := GetConfig(network, peer, orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	consensusTypeConfigValue := updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
	consensusTypeValue := &protosorderer.ConsensusType{}
	err := proto.Unmarshal(consensusTypeConfigValue.Value, consensusTypeValue)
	Expect(err).NotTo(HaveOccurred())

	consensusTypeValue.Metadata = mutateMetadata(consensusTypeValue.Metadata)

	updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value:     utils.MarshalOrPanic(consensusTypeValue),
	}

	UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
}

// UpdateEtcdRaftMetadata executes a config update that updates the etcdraft metadata according to the given function f
func UpdateEtcdRaftMetadata(network *Network, peer *Peer, orderer *Orderer, channel string, f func(md *ectdraft_protos.ConfigMetadata)) {
	UpdateConsensusMetadata(network, peer, orderer, channel, func(originalMetadata []byte) []byte {
		metadata := &ectdraft_protos.ConfigMetadata{}
		err := proto.Unmarshal(originalMetadata, metadata)
		Expect(err).NotTo(HaveOccurred())

		f(metadata)

		newMetadata, err := proto.Marshal(metadata)
		Expect(err).NotTo(HaveOccurred())
		return newMetadata
	})
}

func UpdateOrdererMSP(network *Network, peer *Peer, orderer *Orderer, channel, orgID string, mutateMSP MSPMutator) {
	config := GetConfig(network, peer, orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	// Unpack the MSP config
	rawMSPConfig := updatedConfig.ChannelGroup.Groups["Orderer"].Groups[orgID].Values["MSP"]
	mspConfig := &msp.MSPConfig{}
	err := proto.Unmarshal(rawMSPConfig.Value, mspConfig)
	Expect(err).NotTo(HaveOccurred())

	fabricConfig := &msp.FabricMSPConfig{}
	err = proto.Unmarshal(mspConfig.Config, fabricConfig)
	Expect(err).NotTo(HaveOccurred())

	// Mutate it as we are asked
	*fabricConfig = mutateMSP(*fabricConfig)

	// Wrap it back into the config
	mspConfig.Config = utils.MarshalOrPanic(fabricConfig)
	rawMSPConfig.Value = utils.MarshalOrPanic(mspConfig)

	UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
}
