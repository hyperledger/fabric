/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

type Chaincode struct {
	Name              string
	Version           string
	Path              string
	Ctor              string
	Policy            string
	Lang              string
	CollectionsConfig string // optional
	PackageFile       string
	Hash              string
	Sequence          string
	EndorsementPlugin string
	ValidationPlugin  string
	InitRequired      bool
}

// DeployChaincodeNewLifecycle is a helper that will install chaincode to all
// peers that are connected to the specified channel, approve the chaincode
// on one of the peers of each organization in the network, commit the chaincode
// definition on the channel using one of the peers, and wait for the chaincode
// commit to complete on all of the peers. It uses the _lifecycle implementation.
// NOTE V2_0 capabilities must be enabled for this functionality to work.
func DeployChaincodeNewLifecycle(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peers ...*Peer) {
	if len(peers) == 0 {
		peers = n.PeersWithChannel(channel)
	}
	if len(peers) == 0 {
		return
	}

	// create temp file for chaincode package if not provided
	if chaincode.PackageFile == "" {
		tempFile, err := ioutil.TempFile("", "chaincode-package")
		Expect(err).NotTo(HaveOccurred())
		tempFile.Close()
		defer os.Remove(tempFile.Name())
		chaincode.PackageFile = tempFile.Name()
	}

	// package using the first peer
	PackageChaincodeNewLifecycle(n, chaincode, peers[0])

	// install on all peers
	InstallChaincodeNewLifecycle(n, chaincode, peers...)

	// approve for each org
	for _, org := range n.PeerOrgs() {
		ApproveChaincodeForMyOrgNewLifecycle(n, channel, orderer, chaincode, n.PeersInOrg(org.Name)...)
	}

	// commit definition
	CommitChaincodeNewLifecycle(n, channel, orderer, chaincode, peers[0], peers...)

	// init the chaincode, if required
	if chaincode.InitRequired {
		InitChaincodeNewLifecycle(n, channel, orderer, chaincode, peers...)
	}
}

// DeployChaincode is a helper that will install chaincode to all peers
// that are connected to the specified channel, instantiate the chaincode on
// one of the peers, and wait for the instantiation to complete on all of the
// peers. It uses the legacy lifecycle (lscc) implementation.
//
// NOTE: This helper should not be used to deploy the same chaincode on
// multiple channels as the install will fail on subsequent calls. Instead,
// simply use InstantiateChaincode().
func DeployChaincode(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peers ...*Peer) {
	if len(peers) == 0 {
		peers = n.PeersWithChannel(channel)
	}
	if len(peers) == 0 {
		return
	}

	// create temp file for chaincode package if not provided
	if chaincode.PackageFile == "" {
		tempFile, err := ioutil.TempFile("", "chaincode-package")
		Expect(err).NotTo(HaveOccurred())
		tempFile.Close()
		defer os.Remove(tempFile.Name())
		chaincode.PackageFile = tempFile.Name()
	}

	// package using the first peer
	PackageChaincode(n, chaincode, peers[0])

	// install on all peers
	InstallChaincode(n, chaincode, peers...)

	// instantiate on the first peer
	InstantiateChaincode(n, channel, orderer, chaincode, peers[0], peers...)
}

func PackageChaincodeNewLifecycle(n *Network, chaincode Chaincode, peer *Peer) {
	sess, err := n.PeerAdminSession(peer, commands.ChaincodePackageLifecycle{
		Path:       chaincode.Path,
		Lang:       chaincode.Lang,
		OutputFile: chaincode.PackageFile,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
}

func PackageChaincode(n *Network, chaincode Chaincode, peer *Peer) {
	sess, err := n.PeerAdminSession(peer, commands.ChaincodePackage{
		Name:       chaincode.Name,
		Version:    chaincode.Version,
		Path:       chaincode.Path,
		Lang:       chaincode.Lang,
		OutputFile: chaincode.PackageFile,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
}

func InstallChaincodeNewLifecycle(n *Network, chaincode Chaincode, peers ...*Peer) {
	for _, p := range peers {
		sess, err := n.PeerAdminSession(p, commands.ChaincodeInstallLifecycle{
			Name:        chaincode.Name,
			Version:     chaincode.Version,
			PackageFile: chaincode.PackageFile,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		sess, err = n.PeerAdminSession(p, commands.ChaincodeQueryInstalledLifecycle{})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say(fmt.Sprintf("Name: %s, Version: %s, Hash:", chaincode.Name, chaincode.Version)))
	}
}

func InstallChaincode(n *Network, chaincode Chaincode, peers ...*Peer) {
	for _, p := range peers {
		sess, err := n.PeerAdminSession(p, commands.ChaincodeInstall{
			Name:        chaincode.Name,
			Version:     chaincode.Version,
			Path:        chaincode.Path,
			Lang:        chaincode.Lang,
			PackageFile: chaincode.PackageFile,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		sess, err = n.PeerAdminSession(p, commands.ChaincodeListInstalled{})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say(fmt.Sprintf("Name: %s, Version: %s,", chaincode.Name, chaincode.Version)))
	}
}

func ApproveChaincodeForMyOrgNewLifecycle(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peers ...*Peer) {
	if chaincode.Hash == "" {
		pkgBytes, err := ioutil.ReadFile(chaincode.PackageFile)
		Expect(err).NotTo(HaveOccurred())
		hash := util.ComputeSHA256(pkgBytes)
		chaincode.Hash = hex.EncodeToString(hash)
	}

	// TODO Figure out why private data is unable to be fetched by the
	// other peers for the org. For now, we will have each peer in the
	// org endorse the proposal.
	peerAddresses := []string{}
	for _, p := range peers {
		peerAddresses = append(peerAddresses, n.PeerAddress(p, ListenPort))
	}

	sess, err := n.PeerAdminSession(peers[0], commands.ChaincodeApproveForMyOrgLifecycle{
		ChannelID:         channel,
		Orderer:           n.OrdererAddress(orderer, ListenPort),
		Name:              chaincode.Name,
		Version:           chaincode.Version,
		Hash:              chaincode.Hash,
		Sequence:          chaincode.Sequence,
		EndorsementPlugin: chaincode.EndorsementPlugin,
		ValidationPlugin:  chaincode.ValidationPlugin,
		Policy:            chaincode.Policy,
		InitRequired:      chaincode.InitRequired,
		PeerAddresses:     peerAddresses,
		WaitForEvent:      true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	for i := 0; i < len(peerAddresses); i++ {
		Eventually(sess.Err, n.EventuallyTimeout).Should(gbytes.Say(`\Qcommitted with status (VALID)\E`))
	}
}

func CommitChaincodeNewLifecycle(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peer *Peer, checkPeers ...*Peer) {
	if chaincode.Hash == "" {
		pkgBytes, err := ioutil.ReadFile(chaincode.PackageFile)
		Expect(err).NotTo(HaveOccurred())
		hash := util.ComputeSHA256(pkgBytes)
		chaincode.Hash = hex.EncodeToString(hash)
	}

	// TODO don't hardcode this
	// commit using one peer per org
	peerAddresses := []string{
		n.PeerAddress(n.Peer("org1", "peer1"), ListenPort),
		n.PeerAddress(n.Peer("org2", "peer1"), ListenPort),
	}

	sess, err := n.PeerAdminSession(peer, commands.ChaincodeCommitLifecycle{
		ChannelID:         channel,
		Orderer:           n.OrdererAddress(orderer, ListenPort),
		Name:              chaincode.Name,
		Version:           chaincode.Version,
		Hash:              chaincode.Hash,
		Sequence:          chaincode.Sequence,
		EndorsementPlugin: chaincode.EndorsementPlugin,
		ValidationPlugin:  chaincode.ValidationPlugin,
		Policy:            chaincode.Policy,
		InitRequired:      chaincode.InitRequired,
		PeerAddresses:     peerAddresses,
		WaitForEvent:      true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	for i := 0; i < len(peerAddresses); i++ {
		Eventually(sess.Err, n.EventuallyTimeout).Should(gbytes.Say(`\Qcommitted with status (VALID)\E`))
	}
	EnsureCommitted(n, channel, chaincode.Name, chaincode.Version, chaincode.Sequence, checkPeers...)
}

func EnsureCommitted(n *Network, channel, name, version, sequence string, peers ...*Peer) {
	for _, p := range peers {
		Eventually(listCommitted(n, p, channel, name), n.EventuallyTimeout).Should(
			gbytes.Say(fmt.Sprintf("Committed chaincode definition for chaincode '%s' on channel '%s':\nVersion: %s, Sequence: %s", name, channel, version, sequence)),
		)
	}
}

func InitChaincodeNewLifecycle(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peers ...*Peer) {
	// TODO don't hardcode this
	peerAddresses := []string{
		n.PeerAddress(n.Peer("org1", "peer1"), ListenPort),
		n.PeerAddress(n.Peer("org2", "peer1"), ListenPort),
	}
	sess, err := n.PeerUserSession(peers[0], "User1", commands.ChaincodeInvoke{
		ChannelID:     channel,
		Orderer:       n.OrdererAddress(orderer, ListenPort),
		Name:          chaincode.Name,
		Ctor:          chaincode.Ctor,
		PeerAddresses: peerAddresses,
		WaitForEvent:  true,
		IsInit:        true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	for i := 0; i < len(peerAddresses); i++ {
		Eventually(sess.Err, n.EventuallyTimeout).Should(gbytes.Say(`\Qcommitted with status (VALID)\E`))
	}
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))
}

func InstantiateChaincode(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peer *Peer, checkPeers ...*Peer) {
	sess, err := n.PeerAdminSession(peer, commands.ChaincodeInstantiate{
		ChannelID:         channel,
		Orderer:           n.OrdererAddress(orderer, ListenPort),
		Name:              chaincode.Name,
		Version:           chaincode.Version,
		Ctor:              chaincode.Ctor,
		Policy:            chaincode.Policy,
		Lang:              chaincode.Lang,
		CollectionsConfig: chaincode.CollectionsConfig,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	EnsureInstantiated(n, channel, chaincode.Name, chaincode.Version, checkPeers...)
}

func EnsureInstantiated(n *Network, channel, name, version string, peers ...*Peer) {
	for _, p := range peers {
		Eventually(listInstantiated(n, p, channel), n.EventuallyTimeout).Should(
			gbytes.Say(fmt.Sprintf("Name: %s, Version: %s,", name, version)),
		)
	}
}

func UpgradeChaincode(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peers ...*Peer) {
	if len(peers) == 0 {
		peers = n.PeersWithChannel(channel)
	}
	if len(peers) == 0 {
		return
	}

	// install on all peers
	InstallChaincode(n, chaincode, peers...)

	// upgrade from the first peer
	sess, err := n.PeerAdminSession(peers[0], commands.ChaincodeUpgrade{
		ChannelID:         channel,
		Orderer:           n.OrdererAddress(orderer, ListenPort),
		Name:              chaincode.Name,
		Version:           chaincode.Version,
		Ctor:              chaincode.Ctor,
		Policy:            chaincode.Policy,
		CollectionsConfig: chaincode.CollectionsConfig,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	EnsureInstantiated(n, channel, chaincode.Name, chaincode.Version, peers...)
}

func listCommitted(n *Network, peer *Peer, channel, name string) func() *gbytes.Buffer {
	return func() *gbytes.Buffer {
		sess, err := n.PeerAdminSession(peer, commands.ChaincodeListCommittedLifecycle{
			ChannelID: channel,
			Name:      name,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		return sess.Buffer()
	}
}

func listInstantiated(n *Network, peer *Peer, channel string) func() *gbytes.Buffer {
	return func() *gbytes.Buffer {
		sess, err := n.PeerAdminSession(peer, commands.ChaincodeListInstantiated{
			ChannelID: channel,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		return sess.Buffer()
	}
}

// EnableV2_0Capabilities enables the V2_0 capabilities for a running network.
// It generates the config update using the first peer, signs the configuration
// with the subsequent peers, and then submits the config update using the
// first peer.
func EnableV2_0Capabilities(network *Network, channel string, orderer *Orderer, peers ...*Peer) {
	if len(peers) == 0 {
		return
	}

	config := GetConfig(network, peers[0], orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	// include the V2_0 capability in the config
	updatedConfig.ChannelGroup.Groups["Application"].Values["Capabilities"] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value: protoutil.MarshalOrPanic(
			&common.Capabilities{
				Capabilities: map[string]*common.Capability{
					"V2_0": {},
				},
			},
		),
	}

	UpdateConfig(network, orderer, channel, config, updatedConfig, false, peers[0], peers...)
}
