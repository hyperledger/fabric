/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protoutil"
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/gstruct"
)

type Chaincode struct {
	Name                string
	Version             string
	Path                string
	Ctor                string
	Policy              string // only used for legacy lifecycle. For new lifecycle use SignaturePolicy
	Lang                string
	CollectionsConfig   string // optional
	PackageFile         string
	PackageID           string            // if unspecified, chaincode won't be executable. Can use SetPackageIDFromPackageFile() to set.
	CodeFiles           map[string]string // map from paths on the filesystem to code.tar.gz paths
	Sequence            string
	EndorsementPlugin   string
	ValidationPlugin    string
	InitRequired        bool
	Label               string
	SignaturePolicy     string
	ChannelConfigPolicy string
}

func (c *Chaincode) SetPackageIDFromPackageFile() {
	fileBytes, err := ioutil.ReadFile(c.PackageFile)
	Expect(err).NotTo(HaveOccurred())
	hashStr := fmt.Sprintf("%x", util.ComputeSHA256(fileBytes))
	c.PackageID = c.Label + ":" + hashStr
}

// DeployChaincode is a helper that will install chaincode to all peers that
// are connected to the specified channel, approve the chaincode on one of the
// peers of each organization in the network, commit the chaincode definition
// on the channel using one of the peers, and wait for the chaincode commit to
// complete on all of the peers. It uses the _lifecycle implementation.
// NOTE V2_0 capabilities must be enabled for this functionality to work.
func DeployChaincode(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peers ...*Peer) {
	if len(peers) == 0 {
		peers = n.PeersWithChannel(channel)
	}
	if len(peers) == 0 {
		return
	}

	PackageAndInstallChaincode(n, chaincode, peers...)

	// approve for each org
	ApproveChaincodeForMyOrg(n, channel, orderer, chaincode, peers...)

	// commit definition
	CheckCommitReadinessUntilReady(n, channel, chaincode, n.PeerOrgs(), peers...)
	CommitChaincode(n, channel, orderer, chaincode, peers[0], peers...)

	// init the chaincode, if required
	if chaincode.InitRequired {
		InitChaincode(n, channel, orderer, chaincode, peers...)
	}
}

// DeployChaincodeLegacy is a helper that will install chaincode to all peers
// that are connected to the specified channel, instantiate the chaincode on
// one of the peers, and wait for the instantiation to complete on all of the
// peers. It uses the legacy lifecycle (lscc) implementation.
//
// NOTE: This helper should not be used to deploy the same chaincode on
// multiple channels as the install will fail on subsequent calls. Instead,
// simply use InstantiateChaincode().
func DeployChaincodeLegacy(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peers ...*Peer) {
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

	// only create chaincode package if it doesn't already exist
	if fi, err := os.Stat(chaincode.PackageFile); os.IsNotExist(err) || fi.Size() == 0 {
		PackageChaincodeLegacy(n, chaincode, peers[0])
	}

	// install on all peers
	InstallChaincodeLegacy(n, chaincode, peers...)

	// instantiate on the first peer
	InstantiateChaincodeLegacy(n, channel, orderer, chaincode, peers[0], peers...)
}

func PackageAndInstallChaincode(n *Network, chaincode Chaincode, peers ...*Peer) {
	// create temp file for chaincode package if not provided
	if chaincode.PackageFile == "" {
		tempFile, err := ioutil.TempFile("", "chaincode-package")
		Expect(err).NotTo(HaveOccurred())
		tempFile.Close()
		defer os.Remove(tempFile.Name())
		chaincode.PackageFile = tempFile.Name()
	}

	// only create chaincode package if it doesn't already exist
	if _, err := os.Stat(chaincode.PackageFile); os.IsNotExist(err) {
		switch chaincode.Lang {
		case "binary", "ccaas":
			PackageChaincodeBinary(chaincode)
		default:
			PackageChaincode(n, chaincode, peers[0])
		}
	}

	// install on all peers
	InstallChaincode(n, chaincode, peers...)
}

func PackageChaincode(n *Network, chaincode Chaincode, peer *Peer) {
	sess, err := n.PeerAdminSession(peer, commands.ChaincodePackage{
		Path:       chaincode.Path,
		Lang:       chaincode.Lang,
		Label:      chaincode.Label,
		OutputFile: chaincode.PackageFile,
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
}

func PackageChaincodeLegacy(n *Network, chaincode Chaincode, peer *Peer) {
	sess, err := n.PeerAdminSession(peer, commands.ChaincodePackageLegacy{
		Name:       chaincode.Name,
		Version:    chaincode.Version,
		Path:       chaincode.Path,
		Lang:       chaincode.Lang,
		OutputFile: chaincode.PackageFile,
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
}

func CheckPackageID(n *Network, packageFile string, packageID string, peer *Peer) {
	sess, err := n.PeerAdminSession(peer, commands.ChaincodeCalculatePackageID{
		PackageFile: packageFile,
		ClientAuth:  n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gbytes.Say(fmt.Sprintf(`\Q%s\E`, packageID)))
}

func InstallChaincode(n *Network, chaincode Chaincode, peers ...*Peer) {
	// Ensure 'jq' exists in path, because we need it to build chaincode
	if _, err := exec.LookPath("jq"); err != nil {
		ginkgo.Fail("'jq' is needed to build chaincode but it wasn't found in the PATH")
	}

	if chaincode.PackageID == "" {
		chaincode.SetPackageIDFromPackageFile()
	}

	for _, p := range peers {
		sess, err := n.PeerAdminSession(p, commands.ChaincodeInstall{
			PackageFile: chaincode.PackageFile,
			ClientAuth:  n.ClientAuthRequired,
		})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		EventuallyWithOffset(1, sess, n.EventuallyTimeout).Should(gexec.Exit())

		EnsureInstalled(n, chaincode.Label, chaincode.PackageID, p)
		CheckPackageID(n, chaincode.PackageFile, chaincode.PackageID, p)
	}
}

func InstallChaincodeLegacy(n *Network, chaincode Chaincode, peers ...*Peer) {
	// Ensure 'jq' exists in path, because we need it to build chaincode
	if _, err := exec.LookPath("jq"); err != nil {
		ginkgo.Fail("'jq' is needed to build chaincode but it wasn't found in the PATH")
	}

	for _, p := range peers {
		sess, err := n.PeerAdminSession(p, commands.ChaincodeInstallLegacy{
			Name:        chaincode.Name,
			Version:     chaincode.Version,
			Path:        chaincode.Path,
			Lang:        chaincode.Lang,
			PackageFile: chaincode.PackageFile,
			ClientAuth:  n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		EventuallyWithOffset(1, sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		sess, err = n.PeerAdminSession(p, commands.ChaincodeListInstalledLegacy{
			ClientAuth: n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		EventuallyWithOffset(1, sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		ExpectWithOffset(1, sess).To(gbytes.Say(fmt.Sprintf("Name: %s, Version: %s,", chaincode.Name, chaincode.Version)))
	}
}

func ApproveChaincodeForMyOrg(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peers ...*Peer) {
	if chaincode.PackageID == "" {
		chaincode.SetPackageIDFromPackageFile()
	}

	// used to ensure we only approve once per org
	approvedOrgs := map[string]bool{}
	for _, p := range peers {
		if _, ok := approvedOrgs[p.Organization]; !ok {
			sess, err := n.PeerAdminSession(p, commands.ChaincodeApproveForMyOrg{
				ChannelID:           channel,
				Orderer:             n.OrdererAddress(orderer, ListenPort),
				Name:                chaincode.Name,
				Version:             chaincode.Version,
				PackageID:           chaincode.PackageID,
				Sequence:            chaincode.Sequence,
				EndorsementPlugin:   chaincode.EndorsementPlugin,
				ValidationPlugin:    chaincode.ValidationPlugin,
				SignaturePolicy:     chaincode.SignaturePolicy,
				ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
				InitRequired:        chaincode.InitRequired,
				CollectionsConfig:   chaincode.CollectionsConfig,
				ClientAuth:          n.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
			approvedOrgs[p.Organization] = true
			Eventually(sess.Err, n.EventuallyTimeout).Should(gbytes.Say(fmt.Sprintf(`\Qcommitted with status (VALID) at %s\E`, n.PeerAddress(p, ListenPort))))
		}
	}
}

func EnsureChaincodeApproved(n *Network, peer *Peer, channel, name, sequence string) {
	sequenceInt, err := strconv.ParseInt(sequence, 10, 64)
	Expect(err).NotTo(HaveOccurred())
	Eventually(queryApproved(n, peer, channel, name, sequence), n.EventuallyTimeout).Should(
		gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Sequence": Equal(sequenceInt),
		}),
	)
}

func CheckCommitReadinessUntilReady(n *Network, channel string, chaincode Chaincode, checkOrgs []*Organization, peers ...*Peer) {
	for _, p := range peers {
		keys := gstruct.Keys{}
		for _, org := range checkOrgs {
			keys[org.MSPID] = BeTrue()
		}
		Eventually(checkCommitReadiness(n, p, channel, chaincode), n.EventuallyTimeout).Should(
			gstruct.MatchKeys(gstruct.IgnoreExtras, keys),
		)
	}
}

func CommitChaincode(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peer *Peer, checkPeers ...*Peer) {
	// commit using one peer per org
	commitOrgs := map[string]bool{}
	var peerAddresses []string
	for _, p := range checkPeers {
		if exists := commitOrgs[p.Organization]; !exists {
			peerAddresses = append(peerAddresses, n.PeerAddress(p, ListenPort))
			commitOrgs[p.Organization] = true
		}
	}

	sess, err := n.PeerAdminSession(peer, commands.ChaincodeCommit{
		ChannelID:           channel,
		Orderer:             n.OrdererAddress(orderer, ListenPort),
		Name:                chaincode.Name,
		Version:             chaincode.Version,
		Sequence:            chaincode.Sequence,
		EndorsementPlugin:   chaincode.EndorsementPlugin,
		ValidationPlugin:    chaincode.ValidationPlugin,
		SignaturePolicy:     chaincode.SignaturePolicy,
		ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
		InitRequired:        chaincode.InitRequired,
		CollectionsConfig:   chaincode.CollectionsConfig,
		PeerAddresses:       peerAddresses,
		ClientAuth:          n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	for i := 0; i < len(peerAddresses); i++ {
		Eventually(sess.Err, n.EventuallyTimeout).Should(gbytes.Say(`\Qcommitted with status (VALID)\E`))
	}
	checkOrgs := []*Organization{}
	for org := range commitOrgs {
		checkOrgs = append(checkOrgs, n.Organization(org))
	}
	EnsureChaincodeCommitted(n, channel, chaincode.Name, chaincode.Version, chaincode.Sequence, checkOrgs, checkPeers...)
}

// EnsureChaincodeCommitted polls each supplied peer until the chaincode definition
// has been committed to the peer's ledger.
func EnsureChaincodeCommitted(n *Network, channel, name, version, sequence string, checkOrgs []*Organization, peers ...*Peer) {
	for _, p := range peers {
		sequenceInt, err := strconv.ParseInt(sequence, 10, 64)
		Expect(err).NotTo(HaveOccurred())
		approvedKeys := gstruct.Keys{}
		for _, org := range checkOrgs {
			approvedKeys[org.MSPID] = BeTrue()
		}
		Eventually(listCommitted(n, p, channel, name), n.EventuallyTimeout).Should(
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Version":   Equal(version),
				"Sequence":  Equal(sequenceInt),
				"Approvals": gstruct.MatchKeys(gstruct.IgnoreExtras, approvedKeys),
			}),
		)
	}
}

func InitChaincode(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peers ...*Peer) {
	// init using one peer per org
	initOrgs := map[string]bool{}
	var peerAddresses []string
	for _, p := range peers {
		if exists := initOrgs[p.Organization]; !exists {
			peerAddresses = append(peerAddresses, n.PeerAddress(p, ListenPort))
			initOrgs[p.Organization] = true
		}
	}

	sess, err := n.PeerUserSession(peers[0], "User1", commands.ChaincodeInvoke{
		ChannelID:     channel,
		Orderer:       n.OrdererAddress(orderer, ListenPort),
		Name:          chaincode.Name,
		Ctor:          chaincode.Ctor,
		PeerAddresses: peerAddresses,
		WaitForEvent:  true,
		IsInit:        true,
		ClientAuth:    n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	for i := 0; i < len(peerAddresses); i++ {
		Eventually(sess.Err, n.EventuallyTimeout).Should(gbytes.Say(`\Qcommitted with status (VALID)\E`))
	}
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))
}

func InstantiateChaincodeLegacy(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peer *Peer, checkPeers ...*Peer) {
	sess, err := n.PeerAdminSession(peer, commands.ChaincodeInstantiateLegacy{
		ChannelID:         channel,
		Orderer:           n.OrdererAddress(orderer, ListenPort),
		Name:              chaincode.Name,
		Version:           chaincode.Version,
		Ctor:              chaincode.Ctor,
		Policy:            chaincode.Policy,
		Lang:              chaincode.Lang,
		CollectionsConfig: chaincode.CollectionsConfig,
		ClientAuth:        n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	EnsureInstantiatedLegacy(n, channel, chaincode.Name, chaincode.Version, checkPeers...)
}

func EnsureInstantiatedLegacy(n *Network, channel, name, version string, peers ...*Peer) {
	for _, p := range peers {
		Eventually(listInstantiatedLegacy(n, p, channel), n.EventuallyTimeout).Should(
			gbytes.Say(fmt.Sprintf("Name: %s, Version: %s,", name, version)),
		)
	}
}

func UpgradeChaincodeLegacy(n *Network, channel string, orderer *Orderer, chaincode Chaincode, peers ...*Peer) {
	if len(peers) == 0 {
		peers = n.PeersWithChannel(channel)
	}
	if len(peers) == 0 {
		return
	}

	// install on all peers
	InstallChaincodeLegacy(n, chaincode, peers...)

	// upgrade from the first peer
	sess, err := n.PeerAdminSession(peers[0], commands.ChaincodeUpgradeLegacy{
		ChannelID:         channel,
		Orderer:           n.OrdererAddress(orderer, ListenPort),
		Name:              chaincode.Name,
		Version:           chaincode.Version,
		Ctor:              chaincode.Ctor,
		Policy:            chaincode.Policy,
		CollectionsConfig: chaincode.CollectionsConfig,
		ClientAuth:        n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	EnsureInstantiatedLegacy(n, channel, chaincode.Name, chaincode.Version, peers...)
}

func EnsureInstalled(n *Network, label, packageID string, peers ...*Peer) {
	for _, p := range peers {
		Eventually(QueryInstalled(n, p), n.EventuallyTimeout).Should(
			ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras,
				gstruct.Fields{
					"Label":     Equal(label),
					"PackageId": Equal(packageID),
				},
			)),
		)
	}
}

func QueryInstalledReferences(n *Network, channel, label, packageID string, checkPeer *Peer, nameVersions ...[]string) {
	chaincodes := make([]*lifecycle.QueryInstalledChaincodesResult_Chaincode, len(nameVersions))
	for i, nameVersion := range nameVersions {
		chaincodes[i] = &lifecycle.QueryInstalledChaincodesResult_Chaincode{
			Name:    nameVersion[0],
			Version: nameVersion[1],
		}
	}

	Expect(QueryInstalled(n, checkPeer)()).To(
		ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras,
			gstruct.Fields{
				"Label":     Equal(label),
				"PackageId": Equal(packageID),
				"References": HaveKeyWithValue(channel, gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras,
					gstruct.Fields{
						"Chaincodes": ConsistOf(chaincodes),
					},
				))),
			},
		)),
	)
}

type queryInstalledOutput struct {
	InstalledChaincodes []lifecycle.QueryInstalledChaincodesResult_InstalledChaincode `json:"installed_chaincodes"`
}

func QueryInstalled(n *Network, peer *Peer) func() []lifecycle.QueryInstalledChaincodesResult_InstalledChaincode {
	return func() []lifecycle.QueryInstalledChaincodesResult_InstalledChaincode {
		sess, err := n.PeerAdminSession(peer, commands.ChaincodeQueryInstalled{
			ClientAuth: n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		output := &queryInstalledOutput{}
		err = json.Unmarshal(sess.Out.Contents(), output)
		Expect(err).NotTo(HaveOccurred())
		return output.InstalledChaincodes
	}
}

type checkCommitReadinessOutput struct {
	Approvals map[string]bool `json:"approvals"`
}

func checkCommitReadiness(n *Network, peer *Peer, channel string, chaincode Chaincode) func() map[string]bool {
	return func() map[string]bool {
		sess, err := n.PeerAdminSession(peer, commands.ChaincodeCheckCommitReadiness{
			ChannelID:           channel,
			Name:                chaincode.Name,
			Version:             chaincode.Version,
			Sequence:            chaincode.Sequence,
			EndorsementPlugin:   chaincode.EndorsementPlugin,
			ValidationPlugin:    chaincode.ValidationPlugin,
			SignaturePolicy:     chaincode.SignaturePolicy,
			ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
			InitRequired:        chaincode.InitRequired,
			CollectionsConfig:   chaincode.CollectionsConfig,
			ClientAuth:          n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		output := &checkCommitReadinessOutput{}
		err = json.Unmarshal(sess.Out.Contents(), output)
		Expect(err).NotTo(HaveOccurred())
		return output.Approvals
	}
}

type queryApprovedOutput struct {
	Sequence int64 `json:"sequence"`
}

// queryApproved returns the result of the queryApproved command.
// If the command fails for any reason, it will return an empty output object.
func queryApproved(n *Network, peer *Peer, channel, name, sequence string) func() queryApprovedOutput {
	return func() queryApprovedOutput {
		sess, err := n.PeerAdminSession(peer, commands.ChaincodeQueryApproved{
			ChannelID:     channel,
			Name:          name,
			Sequence:      sequence,
			PeerAddresses: []string{n.PeerAddress(peer, ListenPort)},
			ClientAuth:    n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
		output := &queryApprovedOutput{}
		if sess.ExitCode() == 1 {
			// don't try to unmarshal the output as JSON if the query failed
			return *output
		}
		err = json.Unmarshal(sess.Out.Contents(), output)
		Expect(err).NotTo(HaveOccurred())
		return *output
	}
}

type queryCommittedOutput struct {
	Sequence  int64           `json:"sequence"`
	Version   string          `json:"version"`
	Approvals map[string]bool `json:"approvals"`
}

// listCommitted returns the result of the queryCommitted command.
// If the command fails for any reason (e.g. namespace not defined
// or a database access issue), it will return an empty output object.
func listCommitted(n *Network, peer *Peer, channel, name string) func() queryCommittedOutput {
	return func() queryCommittedOutput {
		sess, err := n.PeerAdminSession(peer, commands.ChaincodeListCommitted{
			ChannelID:  channel,
			Name:       name,
			ClientAuth: n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
		output := &queryCommittedOutput{}
		if sess.ExitCode() == 1 {
			// don't try to unmarshal the output as JSON if the query failed
			return *output
		}
		err = json.Unmarshal(sess.Out.Contents(), output)
		Expect(err).NotTo(HaveOccurred())
		return *output
	}
}

func listInstantiatedLegacy(n *Network, peer *Peer, channel string) func() *gbytes.Buffer {
	return func() *gbytes.Buffer {
		sess, err := n.PeerAdminSession(peer, commands.ChaincodeListInstantiatedLegacy{
			ChannelID:  channel,
			ClientAuth: n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		return sess.Buffer()
	}
}

// EnableCapabilities enables a specific capabilities flag for a running network.
// It generates the config update using the first peer, signs the configuration
// with the subsequent peers, and then submits the config update using the
// first peer.
func EnableCapabilities(network *Network, channel, capabilitiesGroup, capabilitiesVersion string, orderer *Orderer, peers ...*Peer) {
	if len(peers) == 0 {
		return
	}

	config := GetConfig(network, peers[0], orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	updatedConfig.ChannelGroup.Groups[capabilitiesGroup].Values["Capabilities"] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value: protoutil.MarshalOrPanic(
			&common.Capabilities{
				Capabilities: map[string]*common.Capability{
					capabilitiesVersion: {},
				},
			},
		),
	}

	UpdateConfig(network, orderer, channel, config, updatedConfig, false, peers[0], peers...)
}

// WaitUntilEqualLedgerHeight waits until all specified peers have the
// provided ledger height on a channel
func WaitUntilEqualLedgerHeight(n *Network, channel string, height int, peers ...*Peer) {
	for _, peer := range peers {
		Eventually(func() int {
			return GetLedgerHeight(n, peer, channel)
		}, n.EventuallyTimeout).Should(Equal(height))
	}
}

// GetLedgerHeight returns the current ledger height for a peer on
// a channel
func GetLedgerHeight(n *Network, peer *Peer, channel string) int {
	sess, err := n.PeerUserSession(peer, "User1", commands.ChannelInfo{
		ChannelID:  channel,
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())

	if sess.ExitCode() == 1 {
		// if org is not yet member of channel, peer will return error
		return -1
	}

	channelInfoStr := strings.TrimPrefix(string(sess.Buffer().Contents()[:]), "Blockchain info:")
	channelInfo := common.BlockchainInfo{}
	json.Unmarshal([]byte(channelInfoStr), &channelInfo)
	return int(channelInfo.Height)
}

// GetMaxLedgerHeight returns the maximum ledger height for the
// peers on a channel
func GetMaxLedgerHeight(n *Network, channel string, peers ...*Peer) int {
	var maxHeight int
	for _, peer := range peers {
		peerHeight := GetLedgerHeight(n, peer, channel)
		if peerHeight > maxHeight {
			maxHeight = peerHeight
		}
	}
	return maxHeight
}
