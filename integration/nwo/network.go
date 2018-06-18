/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/helpers"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/runner"
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
	"github.com/tedsuo/ifrit/grouper"
)

// Organization models information about an Organization. It includes
// the information needed to populate an MSP with cryptogen.
type Organization struct {
	MSPID         string `yaml:"msp_id,omitempty"`
	Name          string `yaml:"name,omitempty"`
	Domain        string `yaml:"domain,omitempty"`
	EnableNodeOUs bool   `yaml:"enable_node_organizational_units"`
	Users         int    `yaml:"users,omitempty"`
	CA            *CA    `yaml:"ca,omitempty"`
}

type CA struct {
	Hostname string `yaml:"hostname,omitempty"`
}

// A Consortium is a named collection of Organizations. It is used to populate
// the Orderer geneesis block profile.
type Consortium struct {
	Name          string   `yaml:"name,omitempty"`
	Organizations []string `yaml:"organizations,omitempty"`
}

// Consensus indicates the orderer types and how many broker and zookeeper
// instances.
type Consensus struct {
	Type       string `yaml:"type,omitempty"`
	Brokers    int    `yaml:"brokers,omitempty"`
	ZooKeepers int    `yaml:"zookeepers,omitempty"`
}

// The SystemChannel declares the name of the network system channel and its
// associated configtxgen profile name.
type SystemChannel struct {
	Name    string `yaml:"name,omitempty"`
	Profile string `yaml:"profile,omitempty"`
}

// Channel associates a channel name with a configtxgen profile name.
type Channel struct {
	Name    string `yaml:"name,omitempty"`
	Profile string `yaml:"profile,omitempty"`
}

// Orderer defines an orderer instance and its owning organization.
type Orderer struct {
	Name         string `yaml:"name,omitempty"`
	Organization string `yaml:"organization,omitempty"`
}

// ID provides a unique identifier for an orderer instance.
func (o Orderer) ID() string {
	return fmt.Sprintf("%s.%s", o.Organization, o.Name)
}

// Peer defines a peer instance, it's owning organization, and the list of
// channels that the peer shoudl be joined to.
type Peer struct {
	Name         string         `yaml:"name,omitempty"`
	Organization string         `yaml:"organization,omitempty"`
	Channels     []*PeerChannel `yaml:"channels,omitempty"`
}

// PeerChannel names of the channel a peer should be joined to and whether or
// not the peer should be an anchor for the channel.
type PeerChannel struct {
	Name   string `yaml:"name,omitempty"`
	Anchor bool   `yaml:"anchor"`
}

// ID provides a unique identifier for a peer instance.
func (p *Peer) ID() string {
	return fmt.Sprintf("%s.%s", p.Organization, p.Name)
}

// Anchor returns true if this peer is an anchor for any channel it has joined.
func (p *Peer) Anchor() bool {
	for _, c := range p.Channels {
		if c.Anchor {
			return true
		}
	}
	return false
}

// A profile encapsulates basic information for a configtxgen profile.
type Profile struct {
	Name          string   `yaml:"name,omitempty"`
	Orderers      []string `yaml:"orderers,omitempty"`
	Consortium    string   `yaml:"consortium,omitempty"`
	Organizations []string `yaml:"organizations,omitempty"`
}

// Network holds information about a fabric network.
type Network struct {
	RootDir      string
	StartPort    uint16
	NetworkID    string
	Components   *Components
	DockerClient *docker.Client

	PortsByBrokerID  map[string]Ports
	PortsByOrdererID map[string]Ports
	PortsByPeerID    map[string]Ports
	Organizations    []*Organization
	SystemChannel    *SystemChannel
	Channels         []*Channel
	Consensus        *Consensus
	Orderers         []*Orderer
	Peers            []*Peer
	Profiles         []*Profile
	Consortiums      []*Consortium

	colorIndex uint
}

// New creates a Network from a simple configuration. All generated or managed
// artifacts for the network will be located under rootDir. Ports will be
// allocated sequentially from the specified startPort.
func New(c *Config, rootDir string, client *docker.Client, startPort int, components *Components) *Network {
	network := &Network{
		StartPort:    uint16(startPort),
		RootDir:      rootDir,
		Components:   components,
		DockerClient: client,

		NetworkID:        helpers.UniqueName(),
		PortsByBrokerID:  map[string]Ports{},
		PortsByOrdererID: map[string]Ports{},
		PortsByPeerID:    map[string]Ports{},

		Organizations: c.Organizations,
		Consensus:     c.Consensus,
		Orderers:      c.Orderers,
		Peers:         c.Peers,
		SystemChannel: c.SystemChannel,
		Channels:      c.Channels,
		Profiles:      c.Profiles,
		Consortiums:   c.Consortiums,
	}

	for i := 0; i < network.Consensus.Brokers; i++ {
		ports := Ports{}
		for _, portName := range BrokerPortNames() {
			ports[portName] = network.ReservePort()
		}
		network.PortsByBrokerID[strconv.Itoa(i)] = ports
	}

	for _, o := range c.Orderers {
		ports := Ports{}
		for _, portName := range OrdererPortNames() {
			ports[portName] = network.ReservePort()
		}
		network.PortsByOrdererID[o.ID()] = ports
	}

	for _, p := range c.Peers {
		ports := Ports{}
		for _, portName := range PeerPortNames() {
			ports[portName] = network.ReservePort()
		}
		network.PortsByPeerID[p.ID()] = ports
	}
	return network
}

// ConfigTxPath returns the path to the generated configtxgen configuration
// file.
func (n *Network) ConfigTxConfigPath() string {
	return filepath.Join(n.RootDir, "configtx.yaml")
}

// CryptoPath returns the path to the directory where cryptogen will place its
// generated artifacts.
func (n *Network) CryptoPath() string {
	return filepath.Join(n.RootDir, "crypto")
}

// CryptoConfigPath returns the path to the generated cryptogen configuration
// file.
func (n *Network) CryptoConfigPath() string {
	return filepath.Join(n.RootDir, "crypto-config.yaml")
}

// OutputBlockPath returns the path to the genesis block for the named system
// channel.
func (n *Network) OutputBlockPath(name string) string {
	return filepath.Join(n.RootDir, fmt.Sprintf("%s_block.pb", name))
}

// CreateChannelTxPath returns the path to the create channel transaction for
// the named channel.
func (n *Network) CreateChannelTxPath(name string) string {
	return filepath.Join(n.RootDir, fmt.Sprintf("%s_tx.pb", name))
}

// OrdererDir returns the path to the configuration directory for the specified
// Orderer.
func (n *Network) OrdererDir(o *Orderer) string {
	return filepath.Join(n.RootDir, "orderers", o.ID())
}

// OrdererConfigPath returns the path to the orderer configuration document for
// the specified Orderer.
func (n *Network) OrdererConfigPath(o *Orderer) string {
	return filepath.Join(n.OrdererDir(o), "orderer.yaml")
}

// PeerDir returns the path to the configuration directory for the specified
// Peer.
func (n *Network) PeerDir(p *Peer) string {
	return filepath.Join(n.RootDir, "peers", p.ID())
}

// PeerConfigPath returns the path to the peer configuration document for the
// specified peer.
func (n *Network) PeerConfigPath(p *Peer) string {
	return filepath.Join(n.PeerDir(p), "core.yaml")
}

// PeerUserMSPDir returns the path to the MSP directory containing the
// certificates and keys for the specified user of the peer.
func (n *Network) PeerUserMSPDir(p *Peer, user string) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.RootDir,
		"crypto",
		"peerOrganizations",
		org.Domain,
		"users",
		fmt.Sprintf("%s@%s", user, org.Domain),
		"msp",
	)
}

// PeerLocalMSPDir returns the path to the local MSP directory for the peer.
func (n *Network) PeerLocalMSPDir(p *Peer) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.RootDir,
		"crypto",
		"peerOrganizations",
		org.Domain,
		"peers",
		fmt.Sprintf("%s.%s", p.Name, org.Domain),
		"msp",
	)
}

// PeerOrgMSPDir returns the path to the MSP directory of the Peer organization.
func (n *Network) PeerOrgMSPDir(org *Organization) string {
	return filepath.Join(
		n.RootDir,
		"crypto",
		"peerOrganizations",
		org.Domain,
		"msp",
	)
}

// OrdererOrgMSPDir returns the path to the MSP directory of the Orderer
// organization.
func (n *Network) OrdererOrgMSPDir(o *Organization) string {
	return filepath.Join(
		n.RootDir,
		"crypto",
		"ordererOrganizations",
		o.Domain,
		"msp",
	)
}

// OrdererLocalMSPDir returns the path to the local MSP directory for the
// Orderer.
func (n *Network) OrdererLocalMSPDir(o *Orderer) string {
	org := n.Organization(o.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.RootDir,
		"crypto",
		"ordererOrganizations",
		org.Domain,
		"orderers",
		fmt.Sprintf("%s.%s", o.Name, org.Domain),
		"msp",
	)
}

// GenerateConfigTree generates the configuration documents required to
// bootstrap a fabric network. A configuration file will be generated for
// cryptogen, configtxgen, and for each peer and orderer. The contents of the
// documents will be based on the Config used to create the Network.
//
// When this method completes, the resulting tree will look something like
// this:
//
// ${rootDir}/configtx.yaml
// ${rootDir}/crypto-config.yaml
// ${rootDir}/orderers/orderer0.orderer-org/orderer.yaml
// ${rootDir}/peers/peer0.org1/core.yaml
// ${rootDir}/peers/peer0.org2/core.yaml
// ${rootDir}/peers/peer1.org1/core.yaml
// ${rootDir}/peers/peer1.org2/core.yaml
//
func (n *Network) GenerateConfigTree() {
	n.GenerateCryptoConfig()
	n.GenerateConfigTxConfig()
	for _, o := range n.Orderers {
		n.GenerateOrdererConfig(o)
	}
	for _, p := range n.Peers {
		n.GenerateCoreConfig(p)
	}
}

// Bootstrap generates the cryptographic material, orderer system channel
// genesis block, and create channel transactions needed to run a fabric
// network.
//
// The cryptogen tool is used to create crypto material from the contents of
// ${rootDir}/crypto-config.yaml. The generated artifacts will be placed in
// ${rootDir}/crypto/...
//
// The gensis block is generated from the profile referenced by the
// SystemChannel.Profile attribute. The block is written to
// ${rootDir}/${SystemChannel.Name}_block.pb.
//
// The create channel transactions are generated for each Channel referenced by
// the Network using the channel's Profile attribute. The transactions are
// written to ${rootDir}/${Channel.Name}_tx.pb.
func (n *Network) Bootstrap() {
	_, err := n.DockerClient.CreateNetwork(
		docker.CreateNetworkOptions{
			Name:   n.NetworkID,
			Driver: "bridge",
		},
	)
	Expect(err).NotTo(HaveOccurred())

	sess, err := n.Cryptogen(commands.Generate{
		Config: n.CryptoConfigPath(),
		Output: n.CryptoPath(),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess).Should(gexec.Exit(0))

	sess, err = n.ConfigTxGen(commands.OutputBlock{
		ChannelID:   n.SystemChannel.Name,
		Profile:     n.SystemChannel.Profile,
		ConfigPath:  n.RootDir,
		OutputBlock: n.OutputBlockPath(n.SystemChannel.Name),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess).Should(gexec.Exit(0))

	for _, c := range n.Channels {
		sess, err := n.ConfigTxGen(commands.CreateChannelTx{
			ChannelID:             c.Name,
			Profile:               c.Profile,
			ConfigPath:            n.RootDir,
			OutputCreateChannelTx: n.CreateChannelTxPath(c.Name),
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess).Should(gexec.Exit(0))
	}
}

// Cleanup attempts to cleanup docker related artifacts that may
// have been created by the network.
func (n *Network) Cleanup() {
	nw, err := n.DockerClient.NetworkInfo(n.NetworkID)
	Expect(err).NotTo(HaveOccurred())

	err = n.DockerClient.RemoveNetwork(nw.ID)
	Expect(err).NotTo(HaveOccurred())

	containers, err := n.DockerClient.ListContainers(docker.ListContainersOptions{All: true})
	Expect(err).NotTo(HaveOccurred())
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.HasPrefix(name, "/"+n.NetworkID) {
				err := n.DockerClient.RemoveContainer(docker.RemoveContainerOptions{ID: c.ID, Force: true})
				Expect(err).NotTo(HaveOccurred())
				break
			}
		}
	}

	images, err := n.DockerClient.ListImages(docker.ListImagesOptions{All: true})
	Expect(err).NotTo(HaveOccurred())
	for _, i := range images {
		for _, tag := range i.RepoTags {
			if strings.HasPrefix(tag, n.NetworkID) {
				err := n.DockerClient.RemoveImage(i.ID)
				Expect(err).NotTo(HaveOccurred())
				break
			}
		}
	}
}

// CreateAndJoinChannels will create all channels specified in the config that
// are referenced by peers. The referencing peers will then be joined to the
// channel(s).
//
// The network must be running before this is called.
func (n *Network) CreateAndJoinChannels(o *Orderer) {
	for _, c := range n.Channels {
		n.CreateAndJoinChannel(o, c.Name)
	}
}

// CreateAndJoinChannel will create the specified channel. The referencing
// peers will then be joined to the channel.
//
// The network must be running before this is called.
func (n *Network) CreateAndJoinChannel(o *Orderer, channelName string) {
	peers := n.PeersWithChannel(channelName)
	if len(peers) == 0 {
		return
	}
	creator := peers[0]

	tempFile, err := ioutil.TempFile("", "genesis-block")
	Expect(err).NotTo(HaveOccurred())
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	sess, err := n.PeerAdminSession(creator, commands.ChannelCreate{
		ChannelID:   channelName,
		Orderer:     n.OrdererAddress(o, ListenPort),
		File:        n.CreateChannelTxPath(channelName),
		OutputBlock: tempFile.Name(),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	for _, p := range peers {
		sess, err := n.PeerAdminSession(p, commands.ChannelJoin{
			BlockPath: tempFile.Name(),
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess).Should(gexec.Exit(0))
	}
}

// CreateChannel will submit an existing create channel transaction to the
// specified orderer. The channel transaction must exist at the location
// returned by CreateChannelTxPath.
//
// The orderer must be running when this is called.
func (n *Network) CreateChannel(name string, o *Orderer, p *Peer) {
	sess, err := n.PeerAdminSession(p, commands.ChannelCreate{
		ChannelID:   name,
		Orderer:     n.OrdererAddress(o, ListenPort),
		File:        n.CreateChannelTxPath(name),
		OutputBlock: "/dev/null",
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess).Should(gexec.Exit(0))
}

// JoinChannel will join peers to the specified channel. The orderer is used to
// obtain the current configuration block for the channel.
//
// The orderer and listed peers must be running before this is called.
func (n *Network) JoinChannel(name string, o *Orderer, peers ...*Peer) {
	if len(peers) == 0 {
		return
	}

	tempFile, err := ioutil.TempFile("", "genesis-block")
	Expect(err).NotTo(HaveOccurred())
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	sess, err := n.PeerAdminSession(peers[0], commands.ChannelFetch{
		Block:      "config",
		ChannelID:  name,
		Orderer:    n.OrdererAddress(o, ListenPort),
		OutputFile: tempFile.Name(),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess).Should(gexec.Exit(0))

	for _, p := range peers {
		sess, err := n.PeerAdminSession(p, commands.ChannelJoin{
			BlockPath: tempFile.Name(),
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess).Should(gexec.Exit(0))
	}
}

// InstallChaincode installs chaincode to the listed peers and verifies that
// the install has completed.
func (n *Network) InstallChaincode(peers []*Peer, install commands.ChaincodeInstall) {
	for _, p := range peers {
		sess, err := n.PeerAdminSession(p, install)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, time.Minute).Should(gexec.Exit(0))

		sess, err = n.PeerAdminSession(p, commands.ChaincodeListInstalled{})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, time.Minute).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say(fmt.Sprintf("Name: %s, Version: %s,", install.Name, install.Version)))
	}
}

// InstantiateChaincode instantiates chaincode at the specified peer and verifies that
// the instantiation is complete before returning.
func (n *Network) InstantiateChaincode(peer *Peer, instantiate commands.ChaincodeInstantiate) {
	sess, err := n.PeerAdminSession(peer, instantiate)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	Eventually(listInstantiated(n, peer, instantiate.ChannelID), time.Minute).Should(
		gbytes.Say(fmt.Sprintf("Name: %s, Version: %s,", instantiate.Name, instantiate.Version)),
	)
}

// Cryptogen starts a gexec.Session for the provided cryptogen command.
func (n *Network) Cryptogen(command Command) (*gexec.Session, error) {
	cmd := NewCommand(n.Components.Cryptogen(), command)
	return n.StartSession(cmd, command.SessionName())
}

// ConfigTxGen starts a gexec.Session for the provided configtxgen command.
func (n *Network) ConfigTxGen(command Command) (*gexec.Session, error) {
	cmd := NewCommand(n.Components.ConfigTxGen(), command)
	return n.StartSession(cmd, command.SessionName())
}

// ZooKeeperRunner returns a runner for a ZooKeeper instance.
func (n *Network) ZooKeeperRunner(idx int) *runner.ZooKeeper {
	colorCode := n.nextColor()
	name := fmt.Sprintf("zookeeper-%d-%s", idx, n.NetworkID)

	return &runner.ZooKeeper{
		ZooMyID:     idx + 1, //  IDs must be between 1 and 255
		Client:      n.DockerClient,
		Name:        name,
		NetworkName: n.NetworkID,
		OutputStream: gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[32m[o]\x1b[%s[%s]\x1b[0m ", colorCode, name),
			ginkgo.GinkgoWriter,
		),
		ErrorStream: gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[91m[e]\x1b[%s[%s]\x1b[0m ", colorCode, name),
			ginkgo.GinkgoWriter,
		),
	}
}

func (n *Network) minBrokersInSync() int {
	if n.Consensus.Brokers < 2 {
		return n.Consensus.Brokers
	}
	return 2
}

func (n *Network) defaultBrokerReplication() int {
	if n.Consensus.Brokers < 3 {
		return n.Consensus.Brokers
	}
	return 3
}

// BrokerRunner returns a runner for an kafka broker instance.
func (n *Network) BrokerRunner(id int, zookeepers []string) *runner.Kafka {
	colorCode := n.nextColor()
	name := fmt.Sprintf("kafka-%d-%s", id, n.NetworkID)

	return &runner.Kafka{
		BrokerID:                 id + 1,
		Client:                   n.DockerClient,
		AdvertisedListeners:      "127.0.0.1",
		HostPort:                 int(n.PortsByBrokerID[strconv.Itoa(id)][HostPort]),
		Name:                     name,
		NetworkName:              n.NetworkID,
		MinInsyncReplicas:        n.minBrokersInSync(),
		DefaultReplicationFactor: n.defaultBrokerReplication(),
		ZooKeeperConnect:         strings.Join(zookeepers, ","),
		OutputStream: gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[32m[o]\x1b[%s[%s]\x1b[0m ", colorCode, name),
			ginkgo.GinkgoWriter,
		),
		ErrorStream: gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[91m[e]\x1b[%s[%s]\x1b[0m ", colorCode, name),
			ginkgo.GinkgoWriter,
		),
	}
}

// BrokerGroupRunner returns a runner that manages the processes that make up
// the kafka broker network for fabric.
func (n *Network) BrokerGroupRunner() ifrit.Runner {
	members := grouper.Members{}
	zookeepers := []string{}

	for i := 0; i < n.Consensus.ZooKeepers; i++ {
		zk := n.ZooKeeperRunner(i)
		zookeepers = append(zookeepers, fmt.Sprintf("%s:2181", zk.Name))
		members = append(members, grouper.Member{Name: zk.Name, Runner: zk})
	}

	for i := 0; i < n.Consensus.Brokers; i++ {
		kafka := n.BrokerRunner(i, zookeepers)
		members = append(members, grouper.Member{Name: kafka.Name, Runner: kafka})
	}

	return grouper.NewOrdered(syscall.SIGTERM, members)
}

// OrdererRunner returns an ifrit.Runner for the specified orderer. The runner
// can be used to start and manage an orderer process.
func (n *Network) OrdererRunner(o *Orderer) ifrit.Runner {
	cmd := exec.Command(n.Components.Orderer())
	cmd.Env = append(cmd.Env, fmt.Sprintf("FABRIC_CFG_PATH=%s", n.OrdererDir(o)))

	config := ginkgomon.Config{
		AnsiColorCode:     n.nextColor(),
		Name:              o.ID(),
		Command:           cmd,
		StartCheck:        "Beginning to serve requests",
		StartCheckTimeout: 15 * time.Second,
	}

	if n.Consensus.Brokers != 0 {
		config.StartCheck = "Start phase completed successfully"
		config.StartCheckTimeout = 30 * time.Second
	}

	return ginkgomon.New(config)
}

// OrdererGroupRunner returns a runner that can be used to start and stop all
// orderers in a network.
func (n *Network) OrdererGroupRunner() ifrit.Runner {
	members := grouper.Members{}
	for _, o := range n.Orderers {
		members = append(members, grouper.Member{Name: o.ID(), Runner: n.OrdererRunner(o)})
	}
	return grouper.NewParallel(syscall.SIGTERM, members)
}

// PeerRunner returns an ifrit.Runner for the specified peer. The runner can be
// used to start and manage a peer process.
func (n *Network) PeerRunner(p *Peer) ifrit.Runner {
	cmd := n.peerCommand(
		commands.NodeStart{PeerID: p.ID()},
		fmt.Sprintf("FABRIC_CFG_PATH=%s", n.PeerDir(p)),
	)

	return ginkgomon.New(ginkgomon.Config{
		AnsiColorCode:     n.nextColor(),
		Name:              p.ID(),
		Command:           cmd,
		StartCheck:        `Started peer with ID=.*, .*, address=`,
		StartCheckTimeout: 15 * time.Second,
	})
}

// PeerGroupRunner returns a runner that can be used to start and stop all
// peers in a network.
func (n *Network) PeerGroupRunner() ifrit.Runner {
	members := grouper.Members{}
	for _, p := range n.Peers {
		members = append(members, grouper.Member{Name: p.ID(), Runner: n.PeerRunner(p)})
	}
	return grouper.NewParallel(syscall.SIGTERM, members)
}

// NetworkGroupRunner returns a runner that can be used to start and stop an
// entire fabric network.
func (n *Network) NetworkGroupRunner() ifrit.Runner {
	members := grouper.Members{
		{Name: "brokers", Runner: n.BrokerGroupRunner()},
		{Name: "orderers", Runner: n.OrdererGroupRunner()},
		{Name: "peers", Runner: n.PeerGroupRunner()},
	}
	return grouper.NewOrdered(syscall.SIGTERM, members)
}

func (n *Network) peerCommand(command Command, env ...string) *exec.Cmd {
	cmd := NewCommand(n.Components.Peer(), command)
	cmd.Env = append(env, cmd.Env...)
	return cmd
}

// PeerAdminSession starts a gexec.Session as a peer admin for the provided
// peer command. This is intended to be used by short running peer cli commands
// that execute in the context of a peer configuration.
func (n *Network) PeerAdminSession(p *Peer, command Command) (*gexec.Session, error) {
	return n.PeerUserSession(p, "Admin", command)
}

// PeerUserSession starts a gexec.Session as a peer user for the provided peer
// command. This is intended to be used by short running peer cli commands that
// execute in the context of a peer configuration.
func (n *Network) PeerUserSession(p *Peer, user string, command Command) (*gexec.Session, error) {
	cmd := n.peerCommand(
		command,
		fmt.Sprintf("FABRIC_CFG_PATH=%s", n.PeerDir(p)),
		fmt.Sprintf("CORE_PEER_MSPCONFIGPATH=%s", n.PeerUserMSPDir(p, user)),
	)
	return n.StartSession(cmd, command.SessionName())
}

// Peer returns the information about the named Peer in the named organization.
func (n *Network) Peer(orgName, peerName string) *Peer {
	for _, p := range n.PeersInOrg(orgName) {
		if p.Name == peerName {
			return p
		}
	}
	return nil
}

// Orderer returns the information about the named Orderer.
func (n *Network) Orderer(name string) *Orderer {
	for _, o := range n.Orderers {
		if o.Name == name {
			return o
		}
	}
	return nil
}

// Organization returns the information about the named Organization.
func (n *Network) Organization(orgName string) *Organization {
	for _, org := range n.Organizations {
		if org.Name == orgName {
			return org
		}
	}
	return nil
}

// Consortium returns information about the named Consortium.
func (n *Network) Consortium(name string) *Consortium {
	for _, c := range n.Consortiums {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// PeerOrgs returns all Organizations associated with at least one Peer.
func (n *Network) PeerOrgs() []*Organization {
	orgsByName := map[string]*Organization{}
	for _, p := range n.Peers {
		orgsByName[p.Organization] = n.Organization(p.Organization)
	}

	orgs := []*Organization{}
	for _, org := range orgsByName {
		orgs = append(orgs, org)
	}
	return orgs
}

// PeersWithChannel returns all Peer instances that have joined the named
// channel.
func (n *Network) PeersWithChannel(chanName string) []*Peer {
	peers := []*Peer{}
	for _, p := range n.Peers {
		for _, c := range p.Channels {
			if c.Name == chanName {
				peers = append(peers, p)
			}
		}
	}
	return peers
}

// AnchorsForChannel returns all Peer instances that are anchors for the
// named channel.
func (n *Network) AnchorsForChannel(chanName string) []*Peer {
	anchors := []*Peer{}
	for _, p := range n.Peers {
		for _, pc := range p.Channels {
			if pc.Name == chanName {
				anchors = append(anchors, p)
			}
		}
	}
	return anchors
}

// AnchorsInOrg returns all peers that are an anchor for at least one channel
// in the named organization.
func (n *Network) AnchorsInOrg(orgName string) []*Peer {
	anchors := []*Peer{}
	for _, p := range n.PeersInOrg(orgName) {
		if p.Anchor() {
			anchors = append(anchors, p)
			break
		}
	}

	// No explicit anchor means all peers are anchors.
	if len(anchors) == 0 {
		anchors = n.PeersInOrg(orgName)
	}

	return anchors
}

// OrderersInOrg returns all Orderer instances owned by the named organaiztion.
func (n *Network) OrderersInOrg(orgName string) []*Orderer {
	orderers := []*Orderer{}
	for _, o := range n.Orderers {
		if o.Organization == orgName {
			orderers = append(orderers, o)
		}
	}
	return orderers
}

// OrgsForOrderers returns all Organization instances that own at least one of
// the named orderers.
func (n *Network) OrgsForOrderers(ordererNames []string) []*Organization {
	orgsByName := map[string]*Organization{}
	for _, name := range ordererNames {
		orgName := n.Orderer(name).Organization
		orgsByName[orgName] = n.Organization(orgName)
	}
	orgs := []*Organization{}
	for _, org := range orgsByName {
		orgs = append(orgs, org)
	}
	return orgs
}

// OrdererOrgs returns all Organization instances that own at least one
// orderer.
func (n *Network) OrdererOrgs() []*Organization {
	orgsByName := map[string]*Organization{}
	for _, o := range n.Orderers {
		orgsByName[o.Organization] = n.Organization(o.Organization)
	}

	orgs := []*Organization{}
	for _, org := range orgsByName {
		orgs = append(orgs, org)
	}
	return orgs
}

// PeersInOrg returns all Peer instances that are owned by the named
// organization.
func (n *Network) PeersInOrg(orgName string) []*Peer {
	peers := []*Peer{}
	for _, o := range n.Peers {
		if o.Organization == orgName {
			peers = append(peers, o)
		}
	}
	return peers
}

// ReservePort allocates the next available port.
func (n *Network) ReservePort() uint16 {
	n.StartPort++
	return n.StartPort - 1
}

type PortName string
type Ports map[PortName]uint16

const (
	ChaincodePort PortName = "Chaincode"
	EventsPort    PortName = "Events"
	HostPort      PortName = "HostPort"
	ListenPort    PortName = "Listen"
	ProfilePort   PortName = "Profile"
)

// PeerPortNames returns the list of ports that need to be reserved for a Peer.
func PeerPortNames() []PortName {
	return []PortName{ListenPort, ChaincodePort, EventsPort, ProfilePort}
}

// OrdererPortNames  returns the list of ports that need to be reserved for an
// Orderer.
func OrdererPortNames() []PortName {
	return []PortName{ListenPort, ProfilePort}
}

// BrokerPortNames returns the list of ports that need to be reserved for a
// Kafka broker.
func BrokerPortNames() []PortName {
	return []PortName{HostPort}
}

// BrokerAddresses returns the list of broker addresses for the network.
func (n *Network) BrokerAddresses(portName PortName) []string {
	addresses := []string{}
	for _, ports := range n.PortsByBrokerID {
		addresses = append(addresses, fmt.Sprintf("127.0.0.1:%d", ports[portName]))
	}
	return addresses
}

// OrdererAddress returns the address (host and port) exposed by the Orderer
// for the named port. Command line tools should use the returned address when
// connecting to the orderer.
//
// This assumes that the orderer is listening on 0.0.0.0 or 127.0.0.1 and is
// available on the loopback address.
func (n *Network) OrdererAddress(o *Orderer, portName PortName) string {
	return fmt.Sprintf("127.0.0.1:%d", n.OrdererPort(o, portName))
}

// OrdererPort returns the named port reserved for the Orderer instance.
func (n *Network) OrdererPort(o *Orderer, portName PortName) uint16 {
	ordererPorts := n.PortsByOrdererID[o.ID()]
	Expect(ordererPorts).NotTo(BeNil())
	return ordererPorts[portName]
}

// PeerAddress returns the address (host and port) exposed by the Peer for the
// named port. Command line tools should use the returned address when
// connecting to a peer.
//
// This assumes that the peer is listening on 0.0.0.0 and is available on the
// loopback address.
func (n *Network) PeerAddress(p *Peer, portName PortName) string {
	return fmt.Sprintf("127.0.0.1:%d", n.PeerPort(p, portName))
}

// PeerPort returns the named port reserved for the Peer instance.
func (n *Network) PeerPort(p *Peer, portName PortName) uint16 {
	peerPorts := n.PortsByPeerID[p.ID()]
	Expect(peerPorts).NotTo(BeNil())
	return peerPorts[portName]
}

func (n *Network) nextColor() string {
	color := n.colorIndex%14 + 31
	if color > 37 {
		color = color + 90 - 37
	}

	n.colorIndex++
	return fmt.Sprintf("%dm", color)
}

// StartSession executes a command session. This should be used to launch
// command line tools that are expected to run to completion.
func (n *Network) StartSession(cmd *exec.Cmd, name string) (*gexec.Session, error) {
	ansiColorCode := n.nextColor()
	return gexec.Start(
		cmd,
		gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[32m[o]\x1b[%s[%s]\x1b[0m ", ansiColorCode, name),
			ginkgo.GinkgoWriter,
		),
		gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[91m[e]\x1b[%s[%s]\x1b[0m ", ansiColorCode, name),
			ginkgo.GinkgoWriter,
		),
	)
}

func (n *Network) GenerateCryptoConfig() {
	crypto, err := os.Create(n.CryptoConfigPath())
	Expect(err).NotTo(HaveOccurred())
	defer crypto.Close()

	t, err := template.New("crypto").Parse(CryptoTemplate)
	Expect(err).NotTo(HaveOccurred())

	pw := gexec.NewPrefixedWriter("[crypto-config.yaml] ", ginkgo.GinkgoWriter)
	err = t.Execute(io.MultiWriter(crypto, pw), n)
	Expect(err).NotTo(HaveOccurred())
}

func (n *Network) GenerateConfigTxConfig() {
	config, err := os.Create(n.ConfigTxConfigPath())
	Expect(err).NotTo(HaveOccurred())
	defer config.Close()

	t, err := template.New("configtx").Parse(ConfigTxTemplate)
	Expect(err).NotTo(HaveOccurred())

	pw := gexec.NewPrefixedWriter("[configtx.yaml] ", ginkgo.GinkgoWriter)
	err = t.Execute(io.MultiWriter(config, pw), n)
	Expect(err).NotTo(HaveOccurred())
}

func (n *Network) GenerateOrdererConfig(o *Orderer) {
	err := os.MkdirAll(n.OrdererDir(o), 0755)
	Expect(err).NotTo(HaveOccurred())

	orderer, err := os.Create(n.OrdererConfigPath(o))
	Expect(err).NotTo(HaveOccurred())
	defer orderer.Close()

	t, err := template.New("orderer").Funcs(template.FuncMap{
		"Orderer": func() *Orderer { return o },
	}).Parse(OrdererTemplate)
	Expect(err).NotTo(HaveOccurred())

	pw := gexec.NewPrefixedWriter(fmt.Sprintf("[%s#orderer.yaml] ", o.ID()), ginkgo.GinkgoWriter)
	err = t.Execute(io.MultiWriter(orderer, pw), n)
	Expect(err).NotTo(HaveOccurred())
}

func (n *Network) GenerateCoreConfig(p *Peer) {
	err := os.MkdirAll(n.PeerDir(p), 0755)
	Expect(err).NotTo(HaveOccurred())

	core, err := os.Create(n.PeerConfigPath(p))
	Expect(err).NotTo(HaveOccurred())
	defer core.Close()

	t, err := template.New("orderer").Funcs(template.FuncMap{
		"Peer": func() *Peer { return p },
	}).Parse(CoreTemplate)
	Expect(err).NotTo(HaveOccurred())

	pw := gexec.NewPrefixedWriter(fmt.Sprintf("[%s#core.yaml] ", p.ID()), ginkgo.GinkgoWriter)
	err = t.Execute(io.MultiWriter(core, pw), n)
	Expect(err).NotTo(HaveOccurred())
}
