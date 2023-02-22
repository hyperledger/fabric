/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/nwo/fabricconfig"
	"github.com/hyperledger/fabric/integration/nwo/runner"
	"github.com/hyperledger/fabric/protoutil"
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"github.com/tedsuo/ifrit/grouper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/yaml.v2"
)

// Blocks defines block cutting config.
type Blocks struct {
	BatchTimeout      int `yaml:"batch_timeout,omitempty"`
	MaxMessageCount   int `yaml:"max_message_count,omitempty"`
	AbsoluteMaxBytes  int `yaml:"absolute_max_bytes,omitempty"`
	PreferredMaxBytes int `yaml:"preferred_max_bytes,omitempty"`
}

// Organization models information about an Organization. It includes
// the information needed to populate an MSP with cryptogen.
type Organization struct {
	MSPID         string `yaml:"msp_id,omitempty"`
	MSPType       string `yaml:"msp_type,omitempty"`
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

// Consensus indicates the orderer types.
type Consensus struct {
	Type                        string `yaml:"type,omitempty"`
	BootstrapMethod             string `yaml:"bootstrap_method,omitempty"`
	ChannelParticipationEnabled bool   `yaml:"channel_participation_enabled,omitempty"`
}

// The SystemChannel declares the name of the network system channel and its
// associated configtxgen profile name.
// Deprecated: will be removed soon
type SystemChannel struct {
	Name    string `yaml:"name,omitempty"`
	Profile string `yaml:"profile,omitempty"`
}

// Channel associates a channel name with a configtxgen profile name.
type Channel struct {
	Name        string `yaml:"name,omitempty"`
	Profile     string `yaml:"profile,omitempty"`
	BaseProfile string `yaml:"baseprofile,omitempty"`
}

// Orderer defines an orderer instance and its owning organization.
type Orderer struct {
	Name         string `yaml:"name,omitempty"`
	Organization string `yaml:"organization,omitempty"`
	Id           int    `yaml:"id,omitempty"`
}

// ID provides a unique identifier for an orderer instance.
func (o Orderer) ID() string {
	return fmt.Sprintf("%s.%s", o.Organization, o.Name)
}

// Peer defines a peer instance, it's owning organization, and the list of
// channels that the peer should be joined to.
type Peer struct {
	Name         string         `yaml:"name,omitempty"`
	DevMode      bool           `yaml:"devmode,omitempty"`
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
	Name                string   `yaml:"name,omitempty"`
	Orderers            []string `yaml:"orderers,omitempty"`
	Consortium          string   `yaml:"consortium,omitempty"`
	Organizations       []string `yaml:"organizations,omitempty"`
	AppCapabilities     []string `yaml:"app_capabilities,omitempty"`
	ChannelCapabilities []string `yaml:"channel_capabilities,omitempty"`
	Blocks              *Blocks  `yaml:"blocks,omitempty"`
}

// Network holds information about a fabric network.
type Network struct {
	RootDir               string
	StartPort             uint16
	Components            *Components
	DockerClient          *docker.Client
	ExternalBuilders      []fabricconfig.ExternalBuilder
	NetworkID             string
	EventuallyTimeout     time.Duration
	SessionCreateInterval time.Duration
	MetricsProvider       string
	StatsdEndpoint        string
	ClientAuthRequired    bool
	TLSEnabled            bool
	GatewayEnabled        bool

	PortsByOrdererID map[string]Ports
	PortsByPeerID    map[string]Ports
	Organizations    []*Organization
	// Deprecated: will soon be removed
	SystemChannel *SystemChannel
	Channels      []*Channel
	Consensus     *Consensus
	Orderers      []*Orderer
	Peers         []*Peer
	Profiles      []*Profile
	// Deprecated: will soon be removed
	Consortiums []*Consortium
	Templates   *Templates

	mutex        sync.Locker
	colorIndex   uint
	lastExecuted map[string]time.Time
}

// New creates a Network from a simple configuration. All generated or managed
// artifacts for the network will be located under rootDir. Ports will be
// allocated sequentially from the specified startPort.
func New(c *Config, rootDir string, dockerClient *docker.Client, startPort int, components *Components) *Network {
	network := &Network{
		StartPort:    uint16(startPort),
		RootDir:      rootDir,
		Components:   components,
		DockerClient: dockerClient,

		NetworkID:         runner.UniqueName(),
		EventuallyTimeout: time.Minute,
		MetricsProvider:   "prometheus",
		PortsByOrdererID:  map[string]Ports{},
		PortsByPeerID:     map[string]Ports{},

		Organizations:  c.Organizations,
		Consensus:      c.Consensus,
		Orderers:       c.Orderers,
		Peers:          c.Peers,
		SystemChannel:  c.SystemChannel,
		Channels:       c.Channels,
		Profiles:       c.Profiles,
		Consortiums:    c.Consortiums,
		Templates:      c.Templates,
		TLSEnabled:     true, // Set TLS enabled as true for default
		GatewayEnabled: true, // Set Gateway enabled as true for default

		mutex:        &sync.Mutex{},
		lastExecuted: make(map[string]time.Time),
	}

	// add the ccaas builder as well; that is built into the release directory
	// so work that out based on current runtime.
	// make integration-preqreqs have been updated to ensure this is built
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	network.ExternalBuilders = []fabricconfig.ExternalBuilder{{
		Path:                 filepath.Join(cwd, "..", "externalbuilders", "binary"),
		Name:                 "binary",
		PropagateEnvironment: []string{"GOPROXY"},
	}, {
		Path:                 filepath.Join(cwd, "..", "..", "release", fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH), "builders", "ccaas"),
		Name:                 "ccaas",
		PropagateEnvironment: []string{"CHAINCODE_AS_A_SERVICE_BUILDER_CONFIG"},
	}}

	if network.Templates == nil {
		network.Templates = &Templates{}
	}
	if network.SessionCreateInterval == 0 {
		network.SessionCreateInterval = time.Second
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

	if dockerClient != nil {
		assertImagesExist(dockerClient, RequiredImages...)
	}

	return network
}

func assertImagesExist(dockerClient *docker.Client, images ...string) {
	for _, imageName := range images {
		images, err := dockerClient.ListImages(docker.ListImagesOptions{
			Filters: map[string][]string{"reference": {imageName}},
		})
		Expect(err).NotTo(HaveOccurred())

		if len(images) != 1 {
			ginkgo.Fail(fmt.Sprintf("missing required image: %s", imageName), 1)
		}
	}
}

// AddOrg adds an organization to a network.
func (n *Network) AddOrg(o *Organization, peers ...*Peer) {
	for _, p := range peers {
		ports := Ports{}
		for _, portName := range PeerPortNames() {
			ports[portName] = n.ReservePort()
		}
		n.PortsByPeerID[p.ID()] = ports
		n.Peers = append(n.Peers, p)
	}

	n.Organizations = append(n.Organizations, o)
	if n.Consortiums != nil {
		n.Consortiums[0].Organizations = append(n.Consortiums[0].Organizations, o.Name)
	}
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
func (n *Network) OutputBlockPath(channelName string) string {
	return filepath.Join(n.RootDir, fmt.Sprintf("%s_block.pb", channelName))
}

// CreateChannelTxPath returns the path to the create channel transaction for
// the named channel.
func (n *Network) CreateChannelTxPath(channelName string) string {
	return filepath.Join(n.RootDir, fmt.Sprintf("%s_tx.pb", channelName))
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

// ReadOrdererConfig  unmarshals an orderer's orderer.yaml and returns an
// object approximating its contents.
func (n *Network) ReadOrdererConfig(o *Orderer) *fabricconfig.Orderer {
	var orderer fabricconfig.Orderer
	ordererBytes, err := ioutil.ReadFile(n.OrdererConfigPath(o))
	Expect(err).NotTo(HaveOccurred())

	err = yaml.Unmarshal(ordererBytes, &orderer)
	Expect(err).NotTo(HaveOccurred())

	return &orderer
}

// WriteOrdererConfig serializes the provided configuration as the specified
// orderer's orderer.yaml document.
func (n *Network) WriteOrdererConfig(o *Orderer, config *fabricconfig.Orderer) {
	ordererBytes, err := yaml.Marshal(config)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(n.OrdererConfigPath(o), ordererBytes, 0o644)
	Expect(err).NotTo(HaveOccurred())

	pw := gexec.NewPrefixedWriter(fmt.Sprintf("[updated-%s#orderer.yaml] ", o.ID()), ginkgo.GinkgoWriter)
	_, err = pw.Write(ordererBytes)
	Expect(err).NotTo(HaveOccurred())
}

// ReadConfigTxConfig  unmarshals the configtx.yaml and returns an
// object approximating its contents.
func (n *Network) ReadConfigTxConfig() *fabricconfig.ConfigTx {
	var configtx fabricconfig.ConfigTx
	configtxBytes, err := ioutil.ReadFile(n.ConfigTxConfigPath())
	Expect(err).NotTo(HaveOccurred())

	err = yaml.Unmarshal(configtxBytes, &configtx)
	Expect(err).NotTo(HaveOccurred())

	return &configtx
}

// WriteConfigTxConfig serializes the provided configuration to configtx.yaml.
func (n *Network) WriteConfigTxConfig(config *fabricconfig.ConfigTx) {
	configtxBytes, err := yaml.Marshal(config)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(n.ConfigTxConfigPath(), configtxBytes, 0o644)
	Expect(err).NotTo(HaveOccurred())
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

// PeerLedgerDir returns the ledger root directory for the specified peer.
func (n *Network) PeerLedgerDir(p *Peer) string {
	return filepath.Join(n.PeerDir(p), "filesystem/ledgersData")
}

// ReadPeerConfig unmarshals a peer's core.yaml and returns an object
// approximating its contents.
func (n *Network) ReadPeerConfig(p *Peer) *fabricconfig.Core {
	var core fabricconfig.Core
	coreBytes, err := ioutil.ReadFile(n.PeerConfigPath(p))
	Expect(err).NotTo(HaveOccurred())

	err = yaml.Unmarshal(coreBytes, &core)
	Expect(err).NotTo(HaveOccurred())

	return &core
}

// WritePeerConfig serializes the provided configuration as the specified
// peer's core.yaml document.
func (n *Network) WritePeerConfig(p *Peer, config *fabricconfig.Core) {
	coreBytes, err := yaml.Marshal(config)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(n.PeerConfigPath(p), coreBytes, 0o644)
	Expect(err).NotTo(HaveOccurred())

	pw := gexec.NewPrefixedWriter(fmt.Sprintf("[updated-%s#core.yaml] ", p.ID()), ginkgo.GinkgoWriter)
	_, err = pw.Write(coreBytes)
	Expect(err).NotTo(HaveOccurred())
}

// peerUserCryptoDir returns the path to the directory containing the
// certificates and keys for the specified user of the peer.
func (n *Network) peerUserCryptoDir(p *Peer, user, cryptoMaterialType string) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return n.userCryptoDir(org, "peerOrganizations", user, cryptoMaterialType)
}

// ordererUserCryptoDir returns the path to the directory containing the
// certificates and keys for the specified user of the orderer.
func (n *Network) ordererUserCryptoDir(o *Orderer, user, cryptoMaterialType string) string {
	org := n.Organization(o.Organization)
	Expect(org).NotTo(BeNil())

	return n.userCryptoDir(org, "ordererOrganizations", user, cryptoMaterialType)
}

// userCryptoDir returns the path to the folder with crypto materials for either peers or orderer organizations
// specific user
func (n *Network) userCryptoDir(org *Organization, nodeOrganizationType, user, cryptoMaterialType string) string {
	return filepath.Join(
		n.RootDir,
		"crypto",
		nodeOrganizationType,
		org.Domain,
		"users",
		fmt.Sprintf("%s@%s", user, org.Domain),
		cryptoMaterialType,
	)
}

// PeerOrgCADir returns the path to the folder containing the CA certificate(s) and/or
// keys for the specified peer organization.
func (n *Network) PeerOrgCADir(o *Organization) string {
	return filepath.Join(
		n.CryptoPath(),
		"peerOrganizations",
		o.Domain,
		"ca",
	)
}

// OrdererOrgCADir returns the path to the folder containing the CA certificate(s) and/or
// keys for the specified orderer organization.
func (n *Network) OrdererOrgCADir(o *Organization) string {
	return filepath.Join(
		n.CryptoPath(),
		"ordererOrganizations",
		o.Domain,
		"ca",
	)
}

// PeerUserMSPDir returns the path to the MSP directory containing the
// certificates and keys for the specified user of the peer.
func (n *Network) PeerUserMSPDir(p *Peer, user string) string {
	return n.peerUserCryptoDir(p, user, "msp")
}

// IdemixUserMSPDir returns the path to the MSP directory containing the
// idemix-related crypto material for the specified user of the organization.
func (n *Network) IdemixUserMSPDir(o *Organization, user string) string {
	return n.userCryptoDir(o, "peerOrganizations", user, "")
}

// OrdererUserMSPDir returns the path to the MSP directory containing the
// certificates and keys for the specified user of the peer.
func (n *Network) OrdererUserMSPDir(o *Orderer, user string) string {
	return n.ordererUserCryptoDir(o, user, "msp")
}

// PeerUserTLSDir returns the path to the TLS directory containing the
// certificates and keys for the specified user of the peer.
func (n *Network) PeerUserTLSDir(p *Peer, user string) string {
	return n.peerUserCryptoDir(p, user, "tls")
}

// PeerUserCert returns the path to the certificate for the specified user in
// the peer organization.
func (n *Network) PeerUserCert(p *Peer, user string) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.PeerUserMSPDir(p, user),
		"signcerts",
		fmt.Sprintf("%s@%s-cert.pem", user, org.Domain),
	)
}

// PeerCACert returns the path to the CA certificate for the peer
// organization.
func (n *Network) PeerCACert(p *Peer) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.PeerOrgMSPDir(org),
		"cacerts",
		fmt.Sprintf("ca.%s-cert.pem", org.Domain),
	)
}

// OrdererUserCert returns the path to the certificate for the specified user in
// the orderer organization.
func (n *Network) OrdererUserCert(o *Orderer, user string) string {
	org := n.Organization(o.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.OrdererUserMSPDir(o, user),
		"signcerts",
		fmt.Sprintf("%s@%s-cert.pem", user, org.Domain),
	)
}

// OrdererCACert returns the path to the CA certificate for the orderer
// organization.
func (n *Network) OrdererCACert(o *Orderer) string {
	org := n.Organization(o.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.OrdererOrgMSPDir(org),
		"cacerts",
		fmt.Sprintf("ca.%s-cert.pem", org.Domain),
	)
}

// PeerUserKey returns the path to the private key for the specified user in
// the peer organization.
func (n *Network) PeerUserKey(p *Peer, user string) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	keystore := filepath.Join(
		n.PeerUserMSPDir(p, user),
		"keystore",
	)

	// file names are the SKI and non-deterministic
	keys, err := ioutil.ReadDir(keystore)
	Expect(err).NotTo(HaveOccurred())
	Expect(keys).To(HaveLen(1))

	return filepath.Join(keystore, keys[0].Name())
}

// OrdererUserKey returns the path to the private key for the specified user in
// the orderer organization.
func (n *Network) OrdererUserKey(o *Orderer, user string) string {
	org := n.Organization(o.Organization)
	Expect(org).NotTo(BeNil())

	keystore := filepath.Join(
		n.OrdererUserMSPDir(o, user),
		"keystore",
	)

	// file names are the SKI and non-deterministic
	keys, err := ioutil.ReadDir(keystore)
	Expect(err).NotTo(HaveOccurred())
	Expect(keys).To(HaveLen(1))

	return filepath.Join(keystore, keys[0].Name())
}

// PeerUserSigner returns a SigningIdentity representing the specified user in
// the peer organization.
func (n *Network) PeerUserSigner(p *Peer, user string) *SigningIdentity {
	return &SigningIdentity{
		CertPath: n.PeerUserCert(p, user),
		KeyPath:  n.PeerUserKey(p, user),
		MSPID:    n.Organization(p.Organization).MSPID,
	}
}

// OrdererUserSigner returns a SigningIdentity representing the specified user in
// the orderer organization.
func (n *Network) OrdererUserSigner(o *Orderer, user string) *SigningIdentity {
	return &SigningIdentity{
		CertPath: n.OrdererUserCert(o, user),
		KeyPath:  n.OrdererUserKey(o, user),
		MSPID:    n.Organization(o.Organization).MSPID,
	}
}

// peerLocalCryptoDir returns the path to the local crypto directory for the peer.
func (n *Network) peerLocalCryptoDir(p *Peer, cryptoType string) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.RootDir,
		"crypto",
		"peerOrganizations",
		org.Domain,
		"peers",
		fmt.Sprintf("%s.%s", p.Name, org.Domain),
		cryptoType,
	)
}

// PeerLocalMSPDir returns the path to the local MSP directory for the peer.
func (n *Network) PeerLocalMSPDir(p *Peer) string {
	return n.peerLocalCryptoDir(p, "msp")
}

// PeerLocalTLSDir returns the path to the local TLS directory for the peer.
func (n *Network) PeerLocalTLSDir(p *Peer) string {
	return n.peerLocalCryptoDir(p, "tls")
}

// PeerCert returns the path to the peer's certificate.
func (n *Network) PeerCert(p *Peer) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.PeerLocalMSPDir(p),
		"signcerts",
		fmt.Sprintf("%s.%s-cert.pem", p.Name, org.Domain),
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

func (n *Network) IdemixOrgMSPDir(org *Organization) string {
	return filepath.Join(
		n.RootDir,
		"crypto",
		"peerOrganizations",
		org.Domain,
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

// OrdererLocalCryptoDir returns the path to the local crypto directory for the
// Orderer.
func (n *Network) OrdererLocalCryptoDir(o *Orderer, cryptoType string) string {
	org := n.Organization(o.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.RootDir,
		"crypto",
		"ordererOrganizations",
		org.Domain,
		"orderers",
		fmt.Sprintf("%s.%s", o.Name, org.Domain),
		cryptoType,
	)
}

// OrdererLocalMSPDir returns the path to the local MSP directory for the
// Orderer.
func (n *Network) OrdererLocalMSPDir(o *Orderer) string {
	return n.OrdererLocalCryptoDir(o, "msp")
}

func (n *Network) OrdererSignCert(o *Orderer) string {
	dirName := filepath.Join(n.OrdererLocalCryptoDir(o, "msp"), "signcerts")
	fileName := fmt.Sprintf("%s.%s-cert.pem", o.Name, n.Organization(o.Organization).Domain)
	return filepath.Join(dirName, fileName)
}

// OrdererLocalTLSDir returns the path to the local TLS directory for the
// Orderer.
func (n *Network) OrdererLocalTLSDir(o *Orderer) string {
	return n.OrdererLocalCryptoDir(o, "tls")
}

// ProfileForChannel gets the configtxgen profile name associated with the
// specified channel.
func (n *Network) ProfileForChannel(channelName string) string {
	for _, ch := range n.Channels {
		if ch.Name == channelName {
			return ch.Profile
		}
	}
	return ""
}

// CACertsBundlePath returns the path to the bundle of CA certificates for the
// network. This bundle is used when connecting to peers.
func (n *Network) CACertsBundlePath() string {
	return filepath.Join(
		n.RootDir,
		"crypto",
		"ca-certs.pem",
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
//	${rootDir}/configtx.yaml
//	${rootDir}/crypto-config.yaml
//	${rootDir}/orderers/orderer0.orderer-org/orderer.yaml
//	${rootDir}/peers/peer0.org1/core.yaml
//	${rootDir}/peers/peer0.org2/core.yaml
//	${rootDir}/peers/peer1.org1/core.yaml
//	${rootDir}/peers/peer1.org2/core.yaml
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
	if n.DockerClient != nil {
		n.CreateDockerNetwork()
	}

	sess, err := n.Cryptogen(commands.Generate{
		Config: n.CryptoConfigPath(),
		Output: n.CryptoPath(),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	n.bootstrapIdemix()

	if n.SystemChannel != nil { // TODO this entire block could be removed once we finish using the system channel
		sess, err = n.ConfigTxGen(commands.OutputBlock{
			ChannelID:   n.SystemChannel.Name,
			Profile:     n.SystemChannel.Profile,
			ConfigPath:  n.RootDir,
			OutputBlock: n.OutputBlockPath(n.SystemChannel.Name),
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		for _, c := range n.Channels {
			sess, err := n.ConfigTxGen(commands.CreateChannelTx{
				ChannelID:             c.Name,
				Profile:               c.Profile,
				BaseProfile:           c.BaseProfile,
				ConfigPath:            n.RootDir,
				OutputCreateChannelTx: n.CreateChannelTxPath(c.Name),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		}

		n.ConcatenateTLSCACertificates()
		return
	}

	for _, c := range n.Channels {
		sess, err := n.ConfigTxGen(commands.OutputBlock{
			ChannelID:   c.Name,
			Profile:     c.Profile,
			ConfigPath:  n.RootDir,
			OutputBlock: n.OutputBlockPath(c.Name),
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}

	n.ConcatenateTLSCACertificates()
}

func (n *Network) CreateDockerNetwork() {
	_, err := n.DockerClient.CreateNetwork(
		docker.CreateNetworkOptions{
			Name:   n.NetworkID,
			Driver: "bridge",
		},
	)
	Expect(err).NotTo(HaveOccurred())

	if runtime.GOOS == "darwin" {
		n.checkDockerNetworks()
	}
}

// checkDockerNetworks attempts to discover if the docker network configuration
// will prevent a container from accessing the host. This commonly happens when
// using Docker for Mac on home networks because most home routers provide DHCP
// addresses from 192.168.1.0/24 and the default Docker daemon config uses
// 192.168.0.0/20 as one of the default local address pools.
//
// https://github.com/moby/libnetwork/blob/1a17fb36132631a95fe6bb055b91e24a516ad81d/ipamutils/utils.go#L18-L20
//
// Docker can be configured to use different addresses by adding an
// appropriate default-address-pools configuration element to "daemon.json".
//
// For example:
//
//	"default-address-pools":[
//	    {"base":"172.30.0.0/16","size":24},
//	    {"base":"172.31.0.0/16","size":24}
//	]
func (n *Network) checkDockerNetworks() {
	hostAddrs := hostIPv4Addrs()
	for _, nw := range n.dockerIPNets() {
		for _, a := range hostAddrs {
			if nw.Contains(a) {
				fmt.Fprintf(ginkgo.GinkgoWriter, "\x1b[01;37;41mWARNING: docker network %s overlaps with host address %s.\x1b[0m\n", nw, a)
				fmt.Fprintf(ginkgo.GinkgoWriter, "\x1b[01;37;41mDocker containers may not have connectivity causing chaincode registration to fail with 'no route to host'.\x1b[0m\n")
			}
		}
	}
}

func (n *Network) dockerIPNets() []*net.IPNet {
	dockerNetworks, err := n.DockerClient.ListNetworks()
	Expect(err).NotTo(HaveOccurred())

	var nets []*net.IPNet
	for _, nw := range dockerNetworks {
		for _, ipconf := range nw.IPAM.Config {
			if ipconf.Subnet != "" {
				_, ipn, err := net.ParseCIDR(ipconf.Subnet)
				Expect(err).NotTo(HaveOccurred())
				nets = append(nets, ipn)
			}
		}
	}
	return nets
}

func hostIPv4Addrs() []net.IP {
	interfaces, err := net.Interfaces()
	Expect(err).NotTo(HaveOccurred())

	var addresses []net.IP
	for _, i := range interfaces {
		addrs, err := i.Addrs()
		Expect(err).NotTo(HaveOccurred())

		for _, a := range addrs {
			a := a
			switch v := a.(type) {
			case *net.IPAddr:
				if v.IP.To4() != nil {
					addresses = append(addresses, v.IP)
				}
			case *net.IPNet:
				if v.IP.To4() != nil {
					addresses = append(addresses, v.IP)
				}
			}
		}
	}
	return addresses
}

// bootstrapIdemix creates the idemix-related crypto material
func (n *Network) bootstrapIdemix() {
	for j, org := range n.IdemixOrgs() {
		output := n.IdemixOrgMSPDir(org)
		// - ca-keygen
		sess, err := n.Idemixgen(commands.CAKeyGen{
			Output: output,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		// - signerconfig
		usersOutput := filepath.Join(n.IdemixOrgMSPDir(org), "users")
		userOutput := filepath.Join(usersOutput, "User1@"+org.Domain)
		sess, err = n.Idemixgen(commands.SignerConfig{
			CAInput:          output,
			Output:           userOutput,
			OrgUnit:          org.Domain,
			EnrollmentID:     "User1",
			RevocationHandle: fmt.Sprintf("11%d", j),
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}
}

// ConcatenateTLSCACertificates concatenates all TLS CA certificates into a
// single file to be used by peer CLI.
func (n *Network) ConcatenateTLSCACertificates() {
	bundle := &bytes.Buffer{}
	for _, tlsCertPath := range n.listTLSCACertificates() {
		certBytes, err := ioutil.ReadFile(tlsCertPath)
		Expect(err).NotTo(HaveOccurred())
		bundle.Write(certBytes)
	}
	err := ioutil.WriteFile(n.CACertsBundlePath(), bundle.Bytes(), 0o660)
	Expect(err).NotTo(HaveOccurred())
}

// listTLSCACertificates returns the paths of all TLS CA certificates in the
// network, across all organizations.
func (n *Network) listTLSCACertificates() []string {
	fileName2Path := make(map[string]string)
	filepath.Walk(filepath.Join(n.RootDir, "crypto"), func(path string, info os.FileInfo, err error) error {
		// File starts with "tlsca" and has "-cert.pem" in it
		if strings.HasPrefix(info.Name(), "tlsca") && strings.Contains(info.Name(), "-cert.pem") {
			fileName2Path[info.Name()] = path
		}
		return nil
	})

	var tlsCACertificates []string
	for _, path := range fileName2Path {
		tlsCACertificates = append(tlsCACertificates, path)
	}
	return tlsCACertificates
}

// Cleanup attempts to cleanup docker related artifacts that may
// have been created by the network.
func (n *Network) Cleanup() {
	if n == nil || n.DockerClient == nil {
		return
	}

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

	n.CreateChannel(channelName, o, peers[0])
	n.JoinChannel(channelName, o, peers...)
}

// UpdateChannelAnchors determines the anchor peers for the specified channel,
// creates an anchor peer update transaction for each organization, and submits
// the update transactions to the orderer.
//
// TODO using configtxgen with -outputAnchorPeersUpdate to update the anchor peers is deprecated and does not work
// with channel participation API. We'll have to generate the channel update explicitly (see UpdateOrgAnchorPeers).
func (n *Network) UpdateChannelAnchors(o *Orderer, channelName string) {
	tempFile, err := ioutil.TempFile("", "update-anchors")
	Expect(err).NotTo(HaveOccurred())
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	peersByOrg := map[string]*Peer{}
	for _, p := range n.AnchorsForChannel(channelName) {
		peersByOrg[p.Organization] = p
	}

	for orgName, p := range peersByOrg {
		anchorUpdate := commands.OutputAnchorPeersUpdate{
			OutputAnchorPeersUpdate: tempFile.Name(),
			ChannelID:               channelName,
			Profile:                 n.ProfileForChannel(channelName),
			ConfigPath:              n.RootDir,
			AsOrg:                   orgName,
		}
		sess, err := n.ConfigTxGen(anchorUpdate)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		sess, err = n.PeerAdminSession(p, commands.ChannelUpdate{
			ChannelID:  channelName,
			Orderer:    n.OrdererAddress(o, ListenPort),
			File:       tempFile.Name(),
			ClientAuth: n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}
}

// UpdateOrgAnchorPeers sets the anchor peers of an organization on a channel using a config update tx, and waits for
// the update to be complete.
func (n *Network) UpdateOrgAnchorPeers(o *Orderer, channelName, orgName string, anchorPeersForOrg []*Peer) {
	peersInOrg := n.PeersInOrg(orgName)
	Expect(peersInOrg).ToNot(BeEmpty())
	currentConfig := GetConfig(n, peersInOrg[0], o, channelName)
	updatedConfig := proto.Clone(currentConfig).(*common.Config)
	orgConfigGroup := updatedConfig.ChannelGroup.Groups["Application"].GetGroups()[orgName]
	Expect(orgConfigGroup).NotTo(BeNil())

	updatedAnchorPeers := &pb.AnchorPeers{}
	for _, p := range anchorPeersForOrg {
		updatedAnchorPeers.AnchorPeers = append(updatedAnchorPeers.AnchorPeers, &pb.AnchorPeer{
			Host: "127.0.0.1",
			Port: int32(n.PeerPort(p, ListenPort)),
		})
	}

	value, err := protoutil.Marshal(updatedAnchorPeers)
	Expect(err).NotTo(HaveOccurred())
	updatedConfig.ChannelGroup.Groups["Application"].GetGroups()[orgName].GetValues()["AnchorPeers"] = &common.ConfigValue{
		Value:     value,
		ModPolicy: "Admins",
	}

	UpdateConfig(n, o, channelName, currentConfig, updatedConfig, false, peersInOrg[0], peersInOrg[0])
}

// VerifyMembership checks that each peer has discovered the expected peers in
// the network.
func (n *Network) VerifyMembership(expectedPeers []*Peer, channel string, chaincodes ...string) {
	// all peers currently include _lifecycle as an available chaincode
	chaincodes = append(chaincodes, "_lifecycle")
	expectedDiscoveredPeerMatchers := make([]types.GomegaMatcher, len(expectedPeers))
	for i, peer := range expectedPeers {
		expectedDiscoveredPeerMatchers[i] = n.discoveredPeerMatcher(peer, chaincodes...)
	}
	for _, peer := range expectedPeers {
		Eventually(DiscoverPeers(n, peer, "User1", channel), n.EventuallyTimeout).Should(ConsistOf(expectedDiscoveredPeerMatchers))
	}
}

func (n *Network) discoveredPeerMatcher(p *Peer, chaincodes ...string) types.GomegaMatcher {
	peerCert, err := ioutil.ReadFile(n.PeerCert(p))
	Expect(err).NotTo(HaveOccurred())

	var ccs []interface{}
	for _, cc := range chaincodes {
		ccs = append(ccs, cc)
	}
	return gstruct.MatchAllFields(gstruct.Fields{
		"MSPID":      Equal(n.Organization(p.Organization).MSPID),
		"Endpoint":   Equal(fmt.Sprintf("127.0.0.1:%d", n.PeerPort(p, ListenPort))),
		"Identity":   Equal(string(peerCert)),
		"Chaincodes": ContainElements(ccs...),
	})
}

// CreateChannel will submit an existing create channel transaction to the
// specified orderer. The channel transaction must exist at the location
// returned by CreateChannelTxPath. Optionally, additional signers may be
// included in the case where the channel creation tx modifies other aspects of
// the channel config for the new channel.
//
// The orderer must be running when this is called.
func (n *Network) CreateChannel(channelName string, o *Orderer, p *Peer, additionalSigners ...interface{}) {
	channelCreateTxPath := n.CreateChannelTxPath(channelName)
	n.signConfigTransaction(channelCreateTxPath, p, additionalSigners...)

	createChannel := func() int {
		sess, err := n.PeerAdminSession(p, commands.ChannelCreate{
			ChannelID:   channelName,
			Orderer:     n.OrdererAddress(o, ListenPort),
			File:        channelCreateTxPath,
			OutputBlock: "/dev/null",
			ClientAuth:  n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		return sess.Wait(n.EventuallyTimeout).ExitCode()
	}
	Eventually(createChannel, n.EventuallyTimeout).Should(Equal(0))
}

// CreateChannelExitCode will submit an existing create channel transaction to
// the specified orderer, wait for the operation to complete, and return the
// exit status of the "peer channel create" command.
//
// The channel transaction must exist at the location returned by
// CreateChannelTxPath and the orderer must be running when this is called.
func (n *Network) CreateChannelExitCode(channelName string, o *Orderer, p *Peer, additionalSigners ...interface{}) int {
	channelCreateTxPath := n.CreateChannelTxPath(channelName)
	n.signConfigTransaction(channelCreateTxPath, p, additionalSigners...)

	sess, err := n.PeerAdminSession(p, commands.ChannelCreate{
		ChannelID:   channelName,
		Orderer:     n.OrdererAddress(o, ListenPort),
		File:        channelCreateTxPath,
		OutputBlock: "/dev/null",
		ClientAuth:  n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	return sess.Wait(n.EventuallyTimeout).ExitCode()
}

func (n *Network) signConfigTransaction(channelTxPath string, submittingPeer *Peer, signers ...interface{}) {
	for _, signer := range signers {
		switch signer := signer.(type) {
		case *Peer:
			sess, err := n.PeerAdminSession(signer, commands.SignConfigTx{
				File:       channelTxPath,
				ClientAuth: n.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		case *Orderer:
			sess, err := n.OrdererAdminSession(signer, submittingPeer, commands.SignConfigTx{
				File:       channelTxPath,
				ClientAuth: n.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		default:
			panic(fmt.Sprintf("unknown signer type %T, expect Peer or Orderer", signer))
		}
	}
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
		Block:      "0",
		ChannelID:  name,
		Orderer:    n.OrdererAddress(o, ListenPort),
		OutputFile: tempFile.Name(),
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	for _, p := range peers {
		sess, err := n.PeerAdminSession(p, commands.ChannelJoin{
			BlockPath:  tempFile.Name(),
			ClientAuth: n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}
}

func (n *Network) JoinChannelBySnapshot(snapshotDir string, peers ...*Peer) {
	if len(peers) == 0 {
		return
	}

	for _, p := range peers {
		sess, err := n.PeerAdminSession(p, commands.ChannelJoinBySnapshot{
			SnapshotPath: snapshotDir,
			ClientAuth:   n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}
}

func (n *Network) JoinBySnapshotStatus(p *Peer) string {
	sess, err := n.PeerAdminSession(p, commands.ChannelJoinBySnapshotStatus{
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	return string(sess.Out.Contents())
}

// Cryptogen starts a gexec.Session for the provided cryptogen command.
func (n *Network) Cryptogen(command Command) (*gexec.Session, error) {
	cmd := NewCommand(n.Components.Cryptogen(), command)
	return n.StartSession(cmd, command.SessionName())
}

// Idemixgen starts a gexec.Session for the provided idemixgen command.
func (n *Network) Idemixgen(command Command) (*gexec.Session, error) {
	cmd := NewCommand(n.Components.Idemixgen(), command)
	return n.StartSession(cmd, command.SessionName())
}

// ConfigTxGen starts a gexec.Session for the provided configtxgen command.
func (n *Network) ConfigTxGen(command Command) (*gexec.Session, error) {
	cmd := NewCommand(n.Components.ConfigTxGen(), command)
	return n.StartSession(cmd, command.SessionName())
}

// Discover starts a gexec.Session for the provided discover command.
func (n *Network) Discover(command Command) (*gexec.Session, error) {
	cmd := NewCommand(n.Components.Discover(), command)
	cmd.Args = append(cmd.Args, "--peerTLSCA", n.CACertsBundlePath())
	return n.StartSession(cmd, command.SessionName())
}

// Osnadmin starts a gexec.Session for the provided osnadmin command.
func (n *Network) Osnadmin(command Command) (*gexec.Session, error) {
	cmd := NewCommand(n.Components.Osnadmin(), command)
	return n.StartSession(cmd, command.SessionName())
}

// OrdererRunner returns an ifrit.Runner for the specified orderer. The runner
// can be used to start and manage an orderer process.
func (n *Network) OrdererRunner(o *Orderer, env ...string) *ginkgomon.Runner {
	cmd := exec.Command(n.Components.Orderer())
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("FABRIC_CFG_PATH=%s", n.OrdererDir(o)))
	cmd.Env = append(cmd.Env, env...)

	config := ginkgomon.Config{
		AnsiColorCode:     n.nextColor(),
		Name:              o.ID(),
		Command:           cmd,
		StartCheck:        "Beginning to serve requests",
		StartCheckTimeout: 15 * time.Second,
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
func (n *Network) PeerRunner(p *Peer, env ...string) *ginkgomon.Runner {
	cmd := n.peerCommand(
		commands.NodeStart{PeerID: p.ID(), DevMode: p.DevMode},
		"",
		"FABRIC_CFG_PATH="+n.PeerDir(p),
		"CORE_LEDGER_STATE_COUCHDBCONFIG_USERNAME=admin",
		"CORE_LEDGER_STATE_COUCHDBCONFIG_PASSWORD=adminpw",
	)
	cmd.Env = append(cmd.Env, env...)

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
		{Name: "orderers", Runner: n.OrdererGroupRunner()},
		{Name: "peers", Runner: n.PeerGroupRunner()},
	}
	return grouper.NewOrdered(syscall.SIGTERM, members)
}

func (n *Network) peerCommand(command Command, tlsDir string, env ...string) *exec.Cmd {
	cmd := NewCommand(n.Components.Peer(), command)
	cmd.Env = append(cmd.Env, env...)

	if connectsToOrderer(command) && n.TLSEnabled {
		cmd.Args = append(cmd.Args, "--tls")
		cmd.Args = append(cmd.Args, "--cafile", n.CACertsBundlePath())
	}

	if clientAuthEnabled(command) {
		certfilePath := filepath.Join(tlsDir, "client.crt")
		keyfilePath := filepath.Join(tlsDir, "client.key")

		cmd.Args = append(cmd.Args, "--certfile", certfilePath)
		cmd.Args = append(cmd.Args, "--keyfile", keyfilePath)
	}

	// In case we have a peer invoke with multiple certificates, we need to mimic
	// the correct peer CLI usage, so we count the number of --peerAddresses
	// usages we have, and add the same (concatenated TLS CA certificates file)
	// the same number of times to bypass the peer CLI sanity checks
	requiredPeerAddresses := flagCount("--peerAddresses", cmd.Args)
	for i := 0; i < requiredPeerAddresses; i++ {
		cmd.Args = append(cmd.Args, "--tlsRootCertFiles")
		cmd.Args = append(cmd.Args, n.CACertsBundlePath())
	}

	// If there is --peerAddress, add --tlsRootCertFile parameter
	requiredPeerAddress := flagCount("--peerAddress", cmd.Args)
	if requiredPeerAddress > 0 {
		cmd.Args = append(cmd.Args, "--tlsRootCertFile")
		cmd.Args = append(cmd.Args, n.CACertsBundlePath())
	}
	return cmd
}

func connectsToOrderer(c Command) bool {
	for _, arg := range c.Args() {
		if arg == "--orderer" {
			return true
		}
	}
	return false
}

func clientAuthEnabled(c Command) bool {
	for _, arg := range c.Args() {
		if arg == "--clientauth" {
			return true
		}
	}
	return false
}

func flagCount(flag string, args []string) int {
	var c int
	for _, arg := range args {
		if arg == flag {
			c++
		}
	}
	return c
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
		n.PeerUserTLSDir(p, user),
		fmt.Sprintf("FABRIC_CFG_PATH=%s", n.PeerDir(p)),
		fmt.Sprintf("CORE_PEER_MSPCONFIGPATH=%s", n.PeerUserMSPDir(p, user)),
	)
	return n.StartSession(cmd, command.SessionName())
}

// PeerClientConn returns a grpc.ClientConn configured to connect to the
// provided peer. This connection can be used to create clients for the peer
// services. The client connection should be closed when the tests are done
// using it.
func (n *Network) PeerClientConn(p *Peer) *grpc.ClientConn {
	return n.newClientConn(
		n.PeerAddress(p, ListenPort),
		filepath.Join(n.PeerLocalTLSDir(p), "ca.crt"),
	)
}

// OrdererClientConn returns a grpc.ClientConn configured to connect to the
// provided orderer. This connection can be used to create clients for the
// orderer services. The client connection should be closed when the tests are
// done using it.
func (n *Network) OrdererClientConn(o *Orderer) *grpc.ClientConn {
	return n.newClientConn(
		n.OrdererAddress(o, ListenPort),
		filepath.Join(n.OrdererLocalTLSDir(o), "ca.crt"),
	)
}

func (n *Network) newClientConn(address, ca string) *grpc.ClientConn {
	fingerprint := "grpc::" + address + "::" + ca
	if d := n.throttleDuration(fingerprint); d > 0 {
		time.Sleep(d)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	creds, err := credentials.NewClientTLSFromFile(ca, "")
	Expect(err).NotTo(HaveOccurred())

	conn, err := grpc.DialContext(
		ctx,
		address,
		grpc.WithBlock(),
		grpc.FailOnNonTempDialError(true),
		grpc.WithTransportCredentials(creds),
	)
	Expect(err).NotTo(HaveOccurred())

	return conn
}

// IdemixUserSession starts a gexec.Session as a idemix user for the provided peer
// command. This is intended to be used by short running peer cli commands that
// execute in the context of a peer configuration.
func (n *Network) IdemixUserSession(p *Peer, idemixOrg *Organization, user string, command Command) (*gexec.Session, error) {
	cmd := n.peerCommand(
		command,
		n.PeerUserTLSDir(p, user),
		fmt.Sprintf("FABRIC_CFG_PATH=%s", n.PeerDir(p)),
		fmt.Sprintf("CORE_PEER_MSPCONFIGPATH=%s", n.IdemixUserMSPDir(idemixOrg, user)),
		fmt.Sprintf("CORE_PEER_LOCALMSPTYPE=%s", "idemix"),
		fmt.Sprintf("CORE_PEER_LOCALMSPID=%s", idemixOrg.MSPID),
	)
	return n.StartSession(cmd, command.SessionName())
}

// OrdererAdminSession starts a gexec.Session as an orderer admin user. This
// is used primarily to generate orderer configuration updates.
func (n *Network) OrdererAdminSession(o *Orderer, p *Peer, command Command) (*gexec.Session, error) {
	cmd := n.peerCommand(
		command,
		n.ordererUserCryptoDir(o, "Admin", "tls"),
		fmt.Sprintf("CORE_PEER_LOCALMSPID=%s", n.Organization(o.Organization).MSPID),
		fmt.Sprintf("FABRIC_CFG_PATH=%s", n.PeerDir(p)),
		fmt.Sprintf("CORE_PEER_MSPCONFIGPATH=%s", n.OrdererUserMSPDir(o, "Admin")),
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

// DiscoveredPeer creates a new DiscoveredPeer from the peer and chaincodes
// passed as arguments.
func (n *Network) DiscoveredPeer(p *Peer, chaincodes ...string) DiscoveredPeer {
	peerCert, err := ioutil.ReadFile(n.PeerCert(p))
	Expect(err).NotTo(HaveOccurred())

	return DiscoveredPeer{
		MSPID:      n.Organization(p.Organization).MSPID,
		Endpoint:   fmt.Sprintf("127.0.0.1:%d", n.PeerPort(p, ListenPort)),
		Identity:   string(peerCert),
		Chaincodes: chaincodes,
	}
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
		if n.Organization(p.Organization).MSPType != "idemix" {
			orgsByName[p.Organization] = n.Organization(p.Organization)
		}
	}

	orgs := []*Organization{}
	for _, org := range orgsByName {
		orgs = append(orgs, org)
	}
	return orgs
}

// IdemixOrgs returns all Organizations of type idemix.
func (n *Network) IdemixOrgs() []*Organization {
	orgs := []*Organization{}
	for _, org := range n.Organizations {
		if org.MSPType == "idemix" {
			orgs = append(orgs, org)
		}
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

	// This is a bit of a hack to make the output of this function deterministic.
	// When this function's output is supplied as input to functions such as ApproveChaincodeForMyOrg
	// it causes a different subset of peers to be picked, which can create flakiness in tests.
	sort.Slice(peers, func(i, j int) bool {
		if peers[i].Organization < peers[j].Organization {
			return true
		}

		return peers[i].Organization == peers[j].Organization && peers[i].Name < peers[j].Name
	})
	return peers
}

// AnchorsForChannel returns all Peer instances that are anchors for the
// named channel.
func (n *Network) AnchorsForChannel(chanName string) []*Peer {
	anchors := []*Peer{}
	for _, p := range n.Peers {
		for _, pc := range p.Channels {
			if pc.Name == chanName && pc.Anchor {
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
		}
	}

	return anchors
}

// OrderersInOrg returns all Orderer instances owned by the named organization.
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

type (
	PortName string
	Ports    map[PortName]uint16
)

const (
	ChaincodePort  PortName = "Chaincode"
	EventsPort     PortName = "Events"
	HostPort       PortName = "HostPort"
	ListenPort     PortName = "Listen"
	ProfilePort    PortName = "Profile"
	OperationsPort PortName = "Operations"
	ClusterPort    PortName = "Cluster"
	AdminPort      PortName = "Admin"
)

// PeerPortNames returns the list of ports that need to be reserved for a Peer.
func PeerPortNames() []PortName {
	return []PortName{ListenPort, ChaincodePort, EventsPort, ProfilePort, OperationsPort}
}

// OrdererPortNames  returns the list of ports that need to be reserved for an
// Orderer.
func OrdererPortNames() []PortName {
	return []PortName{ListenPort, ProfilePort, OperationsPort, ClusterPort, AdminPort}
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
	n.mutex.Lock()
	defer n.mutex.Unlock()
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
	if d := n.throttleDuration(commandFingerprint(cmd)); d > 0 {
		time.Sleep(d)
	}

	ansiColorCode := n.nextColor()
	fmt.Fprintf(
		ginkgo.GinkgoWriter,
		"\x1b[33m[d]\x1b[%s[%s]\x1b[0m starting %s %s\n",
		ansiColorCode,
		name,
		filepath.Base(cmd.Args[0]),
		strings.Join(cmd.Args[1:], " "),
	)
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

// commandFingerprint creates a string containing the program, args, and environment
// of an exec.Cmd. This fingerprint is used to identify repeated calls to the
// same command for throttling.
func commandFingerprint(cmd *exec.Cmd) string {
	buf := bytes.NewBuffer(nil)
	_, err := buf.WriteString(cmd.Dir)
	Expect(err).NotTo(HaveOccurred())
	_, err = buf.WriteString(cmd.Path)
	Expect(err).NotTo(HaveOccurred())

	// sort the environment since it's not positional
	env := append([]string(nil), cmd.Env...)
	sort.Strings(env)
	for _, e := range env {
		_, err := buf.WriteString(e)
		Expect(err).NotTo(HaveOccurred())
	}

	// grab the args but ignore references to temporary files
	for _, arg := range cmd.Args {
		if strings.HasPrefix(arg, os.TempDir()) {
			continue
		}
		_, err := buf.WriteString(arg)
		Expect(err).NotTo(HaveOccurred())
	}

	return buf.String()
}

// throttleDuration returns the time to wait before performing some operation
// representted by the fingerprint.
//
// The duration is determined by looking at when the fingerprinted operation
// was last executed by the network. If the time between now and the last
// execution was less than SessionCreateInterval, the difference between now
// and (execution + SessionCreateInterval) is returned. If more than
// SessionCreateInterval has elapsed since the last command execution, a
// duration of 0 is returned.
func (n *Network) throttleDuration(fingerprint string) time.Duration {
	now := time.Now()
	n.mutex.Lock()
	last := n.lastExecuted[fingerprint]
	n.lastExecuted[fingerprint] = now
	n.mutex.Unlock()

	if diff := last.Add(n.SessionCreateInterval).Sub(now); diff > 0 {
		return diff
	}

	return 0
}

// GenerateCryptoConfig creates the `crypto-config.yaml` configuration file
// provided to `cryptogen` when running Bootstrap. The path to the generated
// file can be obtained from CryptoConfigPath.
func (n *Network) GenerateCryptoConfig() {
	crypto, err := os.Create(n.CryptoConfigPath())
	Expect(err).NotTo(HaveOccurred())
	defer crypto.Close()

	t, err := template.New("crypto").Parse(n.Templates.CryptoTemplate())
	Expect(err).NotTo(HaveOccurred())

	pw := gexec.NewPrefixedWriter("[crypto-config.yaml] ", ginkgo.GinkgoWriter)
	err = t.Execute(io.MultiWriter(crypto, pw), n)
	Expect(err).NotTo(HaveOccurred())
}

// GenerateConfigTxConfig creates the `configtx.yaml` configuration file
// provided to `configtxgen` when running Bootstrap. The path to the generated
// file can be obtained from ConfigTxConfigPath.
func (n *Network) GenerateConfigTxConfig() {
	config, err := os.Create(n.ConfigTxConfigPath())
	Expect(err).NotTo(HaveOccurred())
	defer config.Close()

	t, err := template.New("configtx").Parse(n.Templates.ConfigTxTemplate())
	Expect(err).NotTo(HaveOccurred())

	pw := gexec.NewPrefixedWriter("[configtx.yaml] ", ginkgo.GinkgoWriter)
	err = t.Execute(io.MultiWriter(config, pw), n)
	Expect(err).NotTo(HaveOccurred())
}

// GenerateOrdererConfig creates the `orderer.yaml` configuration file for the
// specified orderer. The path to the generated file can be obtained from
// OrdererConfigPath.
func (n *Network) GenerateOrdererConfig(o *Orderer) {
	err := os.MkdirAll(n.OrdererDir(o), 0o755)
	Expect(err).NotTo(HaveOccurred())

	orderer, err := os.Create(n.OrdererConfigPath(o))
	Expect(err).NotTo(HaveOccurred())
	defer orderer.Close()

	t, err := template.New("orderer").Funcs(template.FuncMap{
		"Orderer":    func() *Orderer { return o },
		"ToLower":    func(s string) string { return strings.ToLower(s) },
		"ReplaceAll": func(s, old, new string) string { return strings.Replace(s, old, new, -1) },
	}).Parse(n.Templates.OrdererTemplate())
	Expect(err).NotTo(HaveOccurred())

	pw := gexec.NewPrefixedWriter(fmt.Sprintf("[%s#orderer.yaml] ", o.ID()), ginkgo.GinkgoWriter)
	err = t.Execute(io.MultiWriter(orderer, pw), n)
	Expect(err).NotTo(HaveOccurred())
}

// GenerateCoreConfig creates the `core.yaml` configuration file for the
// specified peer. The path to the generated file can be obtained from
// PeerConfigPath.
func (n *Network) GenerateCoreConfig(p *Peer) {
	err := os.MkdirAll(n.PeerDir(p), 0o755)
	Expect(err).NotTo(HaveOccurred())

	core, err := os.Create(n.PeerConfigPath(p))
	Expect(err).NotTo(HaveOccurred())
	defer core.Close()

	t, err := template.New("peer").Funcs(template.FuncMap{
		"Peer":       func() *Peer { return p },
		"ToLower":    func(s string) string { return strings.ToLower(s) },
		"ReplaceAll": func(s, old, new string) string { return strings.Replace(s, old, new, -1) },
	}).Parse(n.Templates.CoreTemplate())
	Expect(err).NotTo(HaveOccurred())

	pw := gexec.NewPrefixedWriter(fmt.Sprintf("[%s#core.yaml] ", p.ID()), ginkgo.GinkgoWriter)
	err = t.Execute(io.MultiWriter(core, pw), n)
	Expect(err).NotTo(HaveOccurred())
}

func (n *Network) LoadAppChannelGenesisBlock(channelID string) *common.Block {
	appGenesisPath := n.OutputBlockPath(channelID)
	appGenesisBytes, err := ioutil.ReadFile(appGenesisPath)
	Expect(err).NotTo(HaveOccurred())
	appGenesisBlock, err := protoutil.UnmarshalBlock(appGenesisBytes)
	Expect(err).NotTo(HaveOccurred())
	return appGenesisBlock
}

// StartSingleOrdererNetwork starts the fabric processes assuming a single orderer.
func (n *Network) StartSingleOrdererNetwork(ordererName string) (*ginkgomon.Runner, ifrit.Process, ifrit.Process) {
	ordererRunner, ordererProcess := n.StartOrderer(ordererName)

	peerGroupRunner := n.PeerGroupRunner()
	peerProcess := ifrit.Invoke(peerGroupRunner)
	Eventually(peerProcess.Ready(), n.EventuallyTimeout).Should(BeClosed())

	return ordererRunner, ordererProcess, peerProcess
}

func RestartSingleOrdererNetwork(ordererProcess, peerProcess ifrit.Process, network *Network) (*ginkgomon.Runner, ifrit.Process, ifrit.Process) {
	peerProcess.Signal(syscall.SIGTERM)
	Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
	ordererProcess.Signal(syscall.SIGTERM)
	Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())

	ordererRunner := network.OrdererRunner(network.Orderer("orderer"))
	ordererProcess = ifrit.Invoke(ordererRunner)
	Eventually(ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
	Eventually(ordererRunner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Raft leader changed: 0 -> 1 channel=testchannel node=1"))

	peerGroupRunner := network.PeerGroupRunner()
	peerProcess = ifrit.Invoke(peerGroupRunner)
	Eventually(peerProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

	return ordererRunner, ordererProcess, peerProcess
}

func (n *Network) StartOrderer(ordererName string) (*ginkgomon.Runner, ifrit.Process) {
	ordererRunner := n.OrdererRunner(n.Orderer(ordererName))
	ordererProcess := ifrit.Invoke(ordererRunner)
	Eventually(ordererProcess.Ready(), n.EventuallyTimeout).Should(BeClosed())

	return ordererRunner, ordererProcess
}

// OrdererCert returns the path to the orderer's certificate.
func (n *Network) OrdererCert(o *Orderer) string {
	org := n.Organization(o.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.OrdererLocalMSPDir(o),
		"signcerts",
		fmt.Sprintf("%s.%s-cert.pem", o.Name, org.Domain),
	)
}
