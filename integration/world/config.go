/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package world

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/template"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/integration/runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
	yaml "gopkg.in/yaml.v2"
)

type Profile struct {
	Profiles map[string]localconfig.Profile `yaml:"Profiles"`
}

type OrdererConfig struct {
	OrganizationName              string
	Domain                        string
	OrdererNames                  []string
	BrokerCount                   int // 0 is solo
	ZookeeperCount                int
	KafkaMinInsyncReplicas        int
	KafkaDefaultReplicationFactor int
}

type PeerOrgConfig struct {
	OrganizationName string
	Domain           string
	EnableNodeOUs    bool
	UserCount        int
	PeerCount        int
}

type Stopper interface {
	Stop() error
}

type World struct {
	Rootpath           string
	Components         *Components
	Network            *docker.Network
	OrdererProfileName string
	ChannelProfileName string
	OrdererOrgs        []OrdererConfig
	PeerOrgs           []PeerOrgConfig
	Profiles           map[string]localconfig.Profile
	Cryptogen          runner.Cryptogen
	Idemixgen          runner.Idemixgen
	SystemChannel      string

	LocalStoppers []Stopper
	LocalProcess  []ifrit.Process
}

type Chaincode struct {
	Name     string
	Path     string
	Version  string
	GoPath   string
	ExecPath string
}

type Deployment struct {
	Chaincode Chaincode
	Channel   string
	InitArgs  string
	Policy    string
	Orderer   string
}

func GenerateBasicConfig(ordererType string, numPeers, numPeerOrgs int, testDir string, components *Components) (w World) {
	var (
		err     error
		client  *docker.Client
		network *docker.Network
	)

	client, err = docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())

	pOrg := []*localconfig.Organization{}
	peerOrgs := []PeerOrgConfig{}
	for orgCount := 1; orgCount <= numPeerOrgs; orgCount++ {
		pOrg = append(pOrg,
			&localconfig.Organization{
				Name:   fmt.Sprintf("Org%d", orgCount),
				ID:     fmt.Sprintf("Org%dMSP", orgCount),
				MSPDir: fmt.Sprintf("crypto/peerOrganizations/org%d.example.com/msp", orgCount),
				AnchorPeers: []*localconfig.AnchorPeer{{
					Host: "0.0.0.0",
					Port: 7051 + ((orgCount - 1) * 1000),
				}},
			})
		peerOrgs = append(peerOrgs,
			PeerOrgConfig{
				OrganizationName: fmt.Sprintf("Org%d", orgCount),
				Domain:           fmt.Sprintf("org%d.example.com", orgCount),
				EnableNodeOUs:    false,
				UserCount:        1,
				PeerCount:        numPeers,
			})
	}

	brokerCount := 0
	zookeeperCount := 0
	brokers := []string{}
	if ordererType == "kafka" {
		brokerCount = 2
		zookeeperCount = 1
		brokers = []string{
			"127.0.0.1:9092",
			"127.0.0.1:8092",
		}
	}

	ordererOrgs := []OrdererConfig{{
		OrganizationName: "OrdererOrg",
		Domain:           "example.com",
		OrdererNames:     []string{"orderer"},
		BrokerCount:      brokerCount,
		ZookeeperCount:   zookeeperCount,
	}}

	oOrg := []*localconfig.Organization{{
		Name:   "OrdererOrg",
		ID:     "OrdererMSP",
		MSPDir: filepath.Join("crypto", "ordererOrganizations", "example.com", "orderers", "orderer.example.com", "msp"),
	}}

	peerProfile := localconfig.Profile{
		Consortium: "SampleConsortium",
		Application: &localconfig.Application{
			Organizations: pOrg,
			Capabilities: map[string]bool{
				"V1_2": true,
			},
		},
		Capabilities: map[string]bool{
			"V1_1": true,
		},
	}

	orderer := &localconfig.Orderer{
		BatchTimeout: 1 * time.Second,
		BatchSize: localconfig.BatchSize{
			MaxMessageCount:   1,
			AbsoluteMaxBytes:  (uint32)(98 * 1024 * 1024),
			PreferredMaxBytes: (uint32)(512 * 1024),
		},
		Kafka: localconfig.Kafka{
			Brokers: brokers,
		},
		Organizations: oOrg,
		OrdererType:   ordererType,
		Addresses:     []string{"0.0.0.0:7050"},
		Capabilities:  map[string]bool{"V1_1": true},
	}

	ordererProfile := localconfig.Profile{
		Application: &localconfig.Application{
			Organizations: oOrg,
			Capabilities:  map[string]bool{"V1_2": true},
		},
		Orderer: orderer,
		Consortiums: map[string]*localconfig.Consortium{
			"SampleConsortium": &localconfig.Consortium{
				Organizations: append(oOrg, pOrg...),
			},
		},
		Capabilities: map[string]bool{"V1_1": true},
	}

	profiles := map[string]localconfig.Profile{
		"TwoOrgsOrdererGenesis": ordererProfile,
		"TwoOrgsChannel":        peerProfile,
	}

	// Create a network
	networkName := runner.UniqueName()
	network, err = client.CreateNetwork(
		docker.CreateNetworkOptions{
			Name:   networkName,
			Driver: "bridge",
		},
	)
	Expect(err).NotTo(HaveOccurred())

	crypto := runner.Cryptogen{
		Config: filepath.Join(testDir, "crypto.yaml"),
		Output: filepath.Join(testDir, "crypto"),
	}

	w = World{
		Rootpath:           testDir,
		Components:         components,
		Cryptogen:          crypto,
		Network:            network,
		SystemChannel:      "systestchannel",
		OrdererOrgs:        ordererOrgs,
		PeerOrgs:           peerOrgs,
		OrdererProfileName: "TwoOrgsOrdererGenesis",
		ChannelProfileName: "TwoOrgsChannel",
		Profiles:           profiles,
	}
	return w
}

func (w *World) Construct() {
	var ordererCrypto = `
OrdererOrgs:{{range .OrdererOrgs}}
  - Name: {{.OrganizationName}}
    Domain: {{.Domain}}
    CA:
        Country: US
        Province: California
        Locality: San Francisco
    Specs:{{range .OrdererNames}}
      - Hostname: {{.}}{{end}}
{{end}}`

	var peerCrypto = `
PeerOrgs:{{range .PeerOrgs}}
  - Name: {{.OrganizationName}}
    Domain: {{.Domain}}
    EnableNodeOUs: {{.EnableNodeOUs}}
    CA:
        Country: US
        Province: California
        Locality: San Francisco
    Template:
      Count: {{.PeerCount}}
    Users:
      Count: {{.UserCount}}
{{end}}`

	// Generates the crypto config
	buf := &bytes.Buffer{}
	w.buildTemplate(buf, ordererCrypto)
	w.buildTemplate(buf, peerCrypto)
	err := ioutil.WriteFile(filepath.Join(w.Rootpath, "crypto.yaml"), buf.Bytes(), 0644)
	Expect(err).NotTo(HaveOccurred())

	// Generates the configtx config
	type profiles struct {
		Profiles map[string]localconfig.Profile `yaml:"Profiles"`
	}
	profileData, err := yaml.Marshal(&profiles{w.Profiles})
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(w.Rootpath, "configtx.yaml"), profileData, 0644)
	Expect(err).NotTo(HaveOccurred())
}

func (w *World) buildTemplate(writer io.Writer, orgTemplate string) {
	tmpl, err := template.New("org").Parse(orgTemplate)
	Expect(err).NotTo(HaveOccurred())
	err = tmpl.Execute(writer, w)
	Expect(err).NotTo(HaveOccurred())
}

func (w *World) BootstrapNetwork(channel string) {
	w.Construct()

	w.Cryptogen.Path = w.Components.Paths["cryptogen"]
	r := w.Cryptogen.Generate()
	execute(r)

	configtxgen := runner.Configtxgen{
		Path:      w.Components.Paths["configtxgen"],
		ChannelID: w.SystemChannel,
		Profile:   w.OrdererProfileName,
		ConfigDir: w.Rootpath,
		Output:    filepath.Join(w.Rootpath, fmt.Sprintf("%s_block.pb", w.SystemChannel)),
	}
	r = configtxgen.OutputBlock()
	execute(r)

	configtxgen = runner.Configtxgen{
		Path:      w.Components.Paths["configtxgen"],
		ChannelID: channel,
		Profile:   w.ChannelProfileName,
		ConfigDir: w.Rootpath,
		Output:    filepath.Join(w.Rootpath, fmt.Sprintf("%s_tx.pb", channel)),
	}
	r = configtxgen.OutputCreateChannelTx()
	execute(r)

	for _, peer := range w.PeerOrgs {
		configtxgen = runner.Configtxgen{
			Path:      w.Components.Paths["configtxgen"],
			ChannelID: channel,
			AsOrg:     peer.OrganizationName,
			Profile:   w.ChannelProfileName,
			ConfigDir: w.Rootpath,
			Output:    filepath.Join(w.Rootpath, fmt.Sprintf("%s_anchors_update_tx.pb", peer.OrganizationName)),
		}
		r = configtxgen.OutputAnchorPeersUpdate()
		execute(r)
	}
}

func (w *World) BuildNetwork() {
	w.ordererNetwork()
	w.peerNetwork()
}

func (w *World) ordererNetwork() {
	var (
		zookeepers []string
		z          *runner.Zookeeper
		kafkas     []*runner.Kafka
		o          *runner.Orderer
	)

	o = w.Components.Orderer()
	o.ConfigDir = w.Rootpath
	o.LedgerLocation = filepath.Join(w.Rootpath, "ledger")
	o.LogLevel = "debug"
	for _, orderer := range w.OrdererOrgs {
		if orderer.BrokerCount != 0 {
			for id := 1; id <= orderer.ZookeeperCount; id++ {
				// Start zookeeper
				z = w.Components.Zookeeper(id, w.Network)
				outBuffer := gbytes.NewBuffer()
				z.OutputStream = io.MultiWriter(outBuffer, GinkgoWriter)
				err := z.Start()
				Expect(err).NotTo(HaveOccurred())
				Eventually(outBuffer, 5*time.Second).Should(gbytes.Say(`binding to port 0.0.0.0/0.0.0.0:2181`))
				zookeepers = append(zookeepers, fmt.Sprintf("%s:2181", z.Name))
				w.LocalStoppers = append(w.LocalStoppers, z)
			}

			for id := 1; id <= orderer.BrokerCount; id++ {
				var err error
				// Start Kafka Broker
				k := w.Components.Kafka(id, w.Network)
				localKafkaAddress := w.Profiles[w.OrdererProfileName].Orderer.Kafka.Brokers[id-1]
				k.HostPort, err = strconv.Atoi(strings.Split(localKafkaAddress, ":")[1])
				Expect(err).NotTo(HaveOccurred())
				k.MinInsyncReplicas = orderer.KafkaMinInsyncReplicas
				k.DefaultReplicationFactor = orderer.KafkaDefaultReplicationFactor
				k.AdvertisedListeners = localKafkaAddress
				k.ZookeeperConnect = strings.Join(zookeepers, ",")
				k.LogLevel = "debug"
				err = k.Start()
				Expect(err).NotTo(HaveOccurred())

				w.LocalStoppers = append(w.LocalStoppers, k)
				kafkas = append(kafkas, k)
				o.ConfigtxOrdererKafkaBrokers = fmt.Sprintf("%s %s", o.ConfigtxOrdererKafkaBrokers, k.HostAddress)
			}
		}

		ordererRunner := o.New()
		ordererProcess := ifrit.Invoke(ordererRunner)
		Eventually(ordererProcess.Ready()).Should(BeClosed())
		Consistently(ordererProcess.Wait()).ShouldNot(Receive())
		if orderer.BrokerCount != 0 {
			Eventually(ordererRunner.Err(), 90*time.Second).Should(gbytes.Say("Start phase completed successfully"))
		}
		w.LocalProcess = append(w.LocalProcess, ordererProcess)
	}
}

func (w *World) peerNetwork() {
	var p *runner.Peer

	for _, peerOrg := range w.PeerOrgs {
		for peer := 0; peer < peerOrg.PeerCount; peer++ {
			p = w.Components.Peer()
			p.ConfigDir = filepath.Join(w.Rootpath, fmt.Sprintf("peer%d.%s", peer, peerOrg.Domain))
			peerProcess := ifrit.Invoke(p.NodeStart(peer))
			Eventually(peerProcess.Ready()).Should(BeClosed())
			Consistently(peerProcess.Wait()).ShouldNot(Receive())
			w.LocalProcess = append(w.LocalProcess, peerProcess)
		}
	}
}

func (w *World) SetupChannel(d Deployment, peers []string) error {
	var p *runner.Peer

	p = w.Components.Peer()
	p.ConfigDir = filepath.Join(w.Rootpath, "peer0.org1.example.com")
	p.MSPConfigPath = filepath.Join(w.Rootpath, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
	adminRunner := p.CreateChannel(d.Channel, filepath.Join(w.Rootpath, fmt.Sprintf("%s_tx.pb", d.Channel)), d.Orderer)
	execute(adminRunner)

	for _, peer := range peers {
		p = w.Components.Peer()
		peerDir := peer
		peerOrg := strings.SplitN(peer, ".", 2)[1]
		p.ConfigDir = filepath.Join(w.Rootpath, peerDir)
		p.MSPConfigPath = filepath.Join(w.Rootpath, "crypto", "peerOrganizations", peerOrg, "users", fmt.Sprintf("Admin@%s", peerOrg), "msp")
		adminRunner = p.FetchChannel(d.Channel, filepath.Join(w.Rootpath, peerDir, fmt.Sprintf("%s_block.pb", d.Channel)), "0", d.Orderer)
		execute(adminRunner)
		Expect(adminRunner.Err()).To(gbytes.Say("Received block: 0"))

		adminRunner = p.JoinChannel(filepath.Join(w.Rootpath, peerDir, fmt.Sprintf("%s_block.pb", d.Channel)))
		execute(adminRunner)
		Expect(adminRunner.Err()).To(gbytes.Say("Successfully submitted proposal to join channel"))

		p.ExecPath = d.Chaincode.ExecPath
		p.GoPath = d.Chaincode.GoPath
		adminRunner = p.InstallChaincode(d.Chaincode.Name, d.Chaincode.Version, d.Chaincode.Path)
		execute(adminRunner)
		Expect(adminRunner.Err()).To(gbytes.Say(`\QInstalled remotely response:<status:200 payload:"OK" >\E`))
	}

	p = w.Components.Peer()
	p.ConfigDir = filepath.Join(w.Rootpath, "peer0.org1.example.com")
	p.MSPConfigPath = filepath.Join(w.Rootpath, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
	adminRunner = p.InstantiateChaincode(d.Chaincode.Name, d.Chaincode.Version, d.Orderer, d.Channel, d.InitArgs, d.Policy)
	execute(adminRunner)

	listInstantiated := func() bool {
		p = w.Components.Peer()
		p.ConfigDir = filepath.Join(w.Rootpath, "peer0.org1.example.com")
		p.MSPConfigPath = filepath.Join(w.Rootpath, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		adminRunner = p.ChaincodeListInstantiated(d.Channel)
		execute(adminRunner)
		return strings.Contains(string(adminRunner.Buffer().Contents()), fmt.Sprintf("Path: %s", d.Chaincode.Path))
	}
	Eventually(listInstantiated, 90*time.Second, 500*time.Millisecond).Should(BeTrue())

	return nil
}
