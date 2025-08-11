/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	protosorderer "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/internal/configtxlator/update"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"google.golang.org/protobuf/proto"
)

// GetConfigBlock retrieves the current config block for a channel.
func GetConfigBlock(n *Network, peer *Peer, orderer *Orderer, channel string) *common.Block {
	tempDir, err := os.MkdirTemp(n.RootDir, "getConfigBlock")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	// fetch the config block
	output := filepath.Join(tempDir, "config_block.pb")
	FetchConfigBlock(n, peer, orderer, channel, output)

	// unmarshal the config block bytes
	configBlock := UnmarshalBlockFromFile(output)
	return configBlock
}

// FetchConfigBlock fetches latest config block.
func FetchConfigBlock(n *Network, peer *Peer, orderer *Orderer, channel string, output string) {
	fetch := func() int {
		sess, err := n.OrdererAdminSession(orderer, peer, commands.ChannelFetch{
			ChannelID:  channel,
			Block:      "config",
			Orderer:    n.OrdererAddress(orderer, ListenPort),
			OutputFile: output,
			ClientAuth: n.ClientAuthRequired,
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

// GetConfig retrieves the last config of the given channel.
func GetConfig(n *Network, peer *Peer, orderer *Orderer, channel string) *common.Config {
	configBlock := GetConfigBlock(n, peer, orderer, channel)
	// unmarshal the envelope bytes
	envelope, err := protoutil.GetEnvelopeFromBlock(configBlock.Data.Data[0])
	Expect(err).NotTo(HaveOccurred())

	// unmarshal the payload bytes
	payload, err := protoutil.UnmarshalPayload(envelope.Payload)
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
func UpdateConfig(n *Network, orderer *Orderer, channel string, current, updated *common.Config, getConfigBlockFromOrderer bool, submitter *Peer, ordererSigners []*Orderer, additionalSigners ...*Peer) {
	tempDir, err := os.MkdirTemp("", "updateConfig")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	// compute update
	configUpdate, err := update.Compute(current, updated)
	Expect(err).NotTo(HaveOccurred())
	configUpdate.ChannelId = channel

	signedEnvelope, err := protoutil.CreateSignedEnvelope(
		common.HeaderType_CONFIG_UPDATE,
		channel,
		nil, // local signer
		&common.ConfigUpdateEnvelope{ConfigUpdate: protoutil.MarshalOrPanic(configUpdate)},
		0, // message version
		0, // epoch
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(signedEnvelope).NotTo(BeNil())

	updateFile := filepath.Join(tempDir, "update.pb")
	err = os.WriteFile(updateFile, protoutil.MarshalOrPanic(signedEnvelope), 0o600)
	Expect(err).NotTo(HaveOccurred())

	for _, signer := range additionalSigners {
		sess, err := n.PeerAdminSession(signer, commands.SignConfigTx{
			File:       updateFile,
			ClientAuth: n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}

	for _, signer := range ordererSigners {
		sess, err := n.OrdererAdminSession(signer, submitter, commands.SignConfigTx{
			File:       updateFile,
			ClientAuth: n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}

	sess, err := n.PeerAdminSession(submitter, commands.SignConfigTx{
		File:       updateFile,
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	var currentBlockNumber uint64
	// get current configuration block number
	if getConfigBlockFromOrderer {
		currentBlockNumber = CurrentConfigBlockNumber(n, submitter, orderer, channel)
	} else {
		currentBlockNumber = CurrentConfigBlockNumber(n, submitter, nil, channel)
	}

	updateEnvelopeBytes, err := os.ReadFile(updateFile)
	Expect(err).NotTo(HaveOccurred())

	updateEnvelope := &common.Envelope{}
	err = proto.Unmarshal(updateEnvelopeBytes, updateEnvelope)
	Expect(err).NotTo(HaveOccurred())

	ready := make(chan struct{})
	go func() {
		defer GinkgoRecover()
		Update(n, orderer, channel, updateEnvelope)
		close(ready)
	}()
	Eventually(ready, n.EventuallyTimeout).Should(BeClosed())

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

// CurrentConfigBlockNumber retrieves the block number from the header of the
// current config block. This can be used to detect when configuration change
// has completed. If an orderer is not provided, the current config block will
// be fetched from the peer.
func CurrentConfigBlockNumber(n *Network, peer *Peer, orderer *Orderer, channel string) uint64 {
	tempDir, err := os.MkdirTemp(n.RootDir, "currentConfigBlock")
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
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Received block: "))

	configBlock := UnmarshalBlockFromFile(output)

	return configBlock.Header.Number
}

// UpdateOrdererConfig computes, signs, and submits a configuration update
// which requires orderers signature and waits for the update to complete.
func UpdateOrdererConfig(n *Network, orderer *Orderer, channel string, current, updated *common.Config, submitter *Peer, additionalSigners ...*Orderer) {
	tempDir, err := os.MkdirTemp(n.RootDir, "updateConfig")
	Expect(err).NotTo(HaveOccurred())
	updateFile := filepath.Join(tempDir, "update.pb")
	defer os.RemoveAll(tempDir)

	currentBlockNumber := CurrentConfigBlockNumber(n, submitter, orderer, channel)
	ComputeUpdateOrdererConfig(updateFile, n, channel, current, updated, submitter, additionalSigners...)

	updateEnvelopeBytes, err := os.ReadFile(updateFile)
	Expect(err).NotTo(HaveOccurred())

	updateEnvelope := &common.Envelope{}
	err = proto.Unmarshal(updateEnvelopeBytes, updateEnvelope)
	Expect(err).NotTo(HaveOccurred())

	ready := make(chan struct{})
	go func() {
		defer GinkgoRecover()
		Update(n, orderer, channel, updateEnvelope)
		close(ready)
	}()
	Eventually(ready, n.EventuallyTimeout).Should(BeClosed())

	// wait for the block to be committed
	ccb := func() uint64 { return CurrentConfigBlockNumber(n, submitter, orderer, channel) }
	Eventually(ccb, n.EventuallyTimeout).Should(BeNumerically(">", currentBlockNumber))
}

// UpdateOrdererConfigFails computes, signs, and submits a configuration
// update which requires orderer signatures. The caller should wait on the
// returned seession retrieve the exit code.
func UpdateOrdererConfigFails(n *Network, orderer *Orderer, channel string, current, updated *common.Config, errStr string, submitter *Peer, additionalSigners ...*Orderer) {
	tempDir, err := os.MkdirTemp(n.RootDir, "updateConfig")
	Expect(err).NotTo(HaveOccurred())
	updateFile := filepath.Join(tempDir, "update.pb")

	ComputeUpdateOrdererConfig(updateFile, n, channel, current, updated, submitter, additionalSigners...)

	sess, err := n.OrdererAdminSession(orderer, submitter, commands.SignConfigTx{
		File:       updateFile,
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	updateEnvelopeBytes, err := os.ReadFile(updateFile)
	Expect(err).NotTo(HaveOccurred())

	updateEnvelope := &common.Envelope{}
	err = proto.Unmarshal(updateEnvelopeBytes, updateEnvelope)
	Expect(err).NotTo(HaveOccurred())

	ready := make(chan struct{})
	go func() {
		defer GinkgoRecover()
		UpdateWithStatus(n, orderer, channel, updateEnvelope, http.StatusBadRequest, errStr)
		close(ready)
	}()
	Eventually(ready, n.EventuallyTimeout).Should(BeClosed())
}

func ComputeUpdateOrdererConfig(updateFile string, n *Network, channel string, current, updated *common.Config, submitter *Peer, additionalSigners ...*Orderer) {
	// compute update
	configUpdate, err := update.Compute(current, updated)
	Expect(err).NotTo(HaveOccurred())
	configUpdate.ChannelId = channel

	signedEnvelope, err := protoutil.CreateSignedEnvelope(
		common.HeaderType_CONFIG_UPDATE,
		channel,
		nil, // local signer
		&common.ConfigUpdateEnvelope{ConfigUpdate: protoutil.MarshalOrPanic(configUpdate)},
		0, // message version
		0, // epoch
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(signedEnvelope).NotTo(BeNil())

	err = os.WriteFile(updateFile, protoutil.MarshalOrPanic(signedEnvelope), 0o600)
	Expect(err).NotTo(HaveOccurred())

	for _, signer := range additionalSigners {
		sess, err := n.OrdererAdminSession(signer, submitter, commands.SignConfigTx{File: updateFile})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}
}

// UnmarshalBlockFromFile unmarshals a proto encoded block from a file.
func UnmarshalBlockFromFile(blockFile string) *common.Block {
	blockBytes, err := os.ReadFile(blockFile)
	Expect(err).NotTo(HaveOccurred())

	block, err := protoutil.UnmarshalBlock(blockBytes)
	Expect(err).NotTo(HaveOccurred())

	return block
}

// ConsensusMetadataMutator receives ConsensusType.Metadata and mutates it.
type ConsensusMetadataMutator func([]byte) []byte

// MSPMutator receives FabricMSPConfig and mutates it.
type MSPMutator func(config *msp.FabricMSPConfig) *msp.FabricMSPConfig

// UpdateConsensusMetadata executes a config update that updates the consensus
// metadata according to the given ConsensusMetadataMutator.
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
		Value:     protoutil.MarshalOrPanic(consensusTypeValue),
	}

	UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
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
	fabricConfig = mutateMSP(fabricConfig)

	// Wrap it back into the config
	mspConfig.Config = protoutil.MarshalOrPanic(fabricConfig)
	rawMSPConfig.Value = protoutil.MarshalOrPanic(mspConfig)

	UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
}

func UpdateConsenters(network *Network, peer *Peer, orderer *Orderer, channel string, f func(orderers *common.Orderers)) {
	config := GetConfig(network, peer, orderer, channel)

	updatedConfig := proto.Clone(config).(*common.Config)

	rawOrderers := updatedConfig.ChannelGroup.Groups["Orderer"].Values["Orderers"]

	orderersVal := &common.Orderers{}
	err := proto.Unmarshal(rawOrderers.Value, orderersVal)
	Expect(err).NotTo(HaveOccurred())

	f(orderersVal)

	policies.EncodeBFTBlockVerificationPolicy(orderersVal.ConsenterMapping, updatedConfig.ChannelGroup.Groups["Orderer"])

	rawOrderers.Value, err = proto.Marshal(orderersVal)
	Expect(err).NotTo(HaveOccurred())

	updatedConfig.ChannelGroup.Groups["Orderer"].Values["Orderers"].Value = protoutil.MarshalOrPanic(orderersVal)

	UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
}

// UpdateOrdererEndpoints executes a config update that updates the orderer metadata according to the given endpoints
func UpdateOrdererEndpoints(network *Network, peer *Peer, orderer *Orderer, channel string, endpoints ...string) {
	config := GetConfig(network, peer, orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	ordererGrp := updatedConfig.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups
	// Get the first orderer org config
	var firstOrdererConfig *common.ConfigGroup
	for _, grp := range ordererGrp {
		firstOrdererConfig = grp
		break
	}

	firstOrdererConfig.Values["Endpoints"].Value = protoutil.MarshalOrPanic(&common.OrdererAddresses{
		Addresses: endpoints,
	})

	UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
}
