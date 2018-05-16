/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package world_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/integration/runner"
	. "github.com/hyperledger/fabric/integration/world"
	"github.com/tedsuo/ifrit"

	docker "github.com/fsouza/go-dockerclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Config", func() {
	var (
		tempDir string
		w       World
		client  *docker.Client
		network *docker.Network
		err     error
	)

	BeforeEach(func() {
		tempDir, err = ioutil.TempDir("", "crypto")
		Expect(err).NotTo(HaveOccurred())
		client, err = docker.NewClientFromEnv()
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)

		// Stop the docker constainers for zookeeper and kafka
		for _, cont := range w.LocalStoppers {
			cont.Stop()
		}

		// Stop the running chaincode containers
		// We should not need to find the chaincode containers to remove. Opened FAB-10044 to track the cleanup of chaincode.
		// Remove this once this is fixed.
		filters := map[string][]string{}
		filters["name"] = []string{fmt.Sprintf("%s-%s", w.Deployment.Chaincode.Name, w.Deployment.Chaincode.Version)}
		allContainers, _ := client.ListContainers(docker.ListContainersOptions{
			Filters: filters,
		})
		if len(allContainers) > 0 {
			for _, peerOrg := range w.PeerOrgs {
				for peer := 0; peer < peerOrg.PeerCount; peer++ {
					peerName := fmt.Sprintf("peer%d.%s", peer, peerOrg.Domain)
					id := fmt.Sprintf("%s-%s-%s-%s", network.Name, peerName, w.Deployment.Chaincode.Name, w.Deployment.Chaincode.Version)
					client.RemoveContainer(docker.RemoveContainerOptions{
						ID:    id,
						Force: true,
					})
				}
			}
		}

		// Remove chaincode image
		filters = map[string][]string{}
		filters["label"] = []string{fmt.Sprintf("org.hyperledger.fabric.chaincode.id.name=%s", w.Deployment.Chaincode.Name)}
		images, _ := client.ListImages(docker.ListImagesOptions{
			Filters: filters,
		})
		if len(images) > 0 {
			for _, image := range images {
				client.RemoveImage(image.ID)
			}
		}

		// Stop the orderers and peers
		for _, localProc := range w.LocalProcess {
			localProc.Signal(syscall.SIGTERM)
		}

		// Remove any started networks
		if network != nil {
			client.RemoveNetwork(network.Name)
		}
	})

	It("creates the crypto config file for use with cryptogen", func() {
		pOrg := []*localconfig.Organization{{
			Name:   "Org1",
			ID:     "Org1MSP",
			MSPDir: "some dir",
			AnchorPeers: []*localconfig.AnchorPeer{{
				Host: "some host",
				Port: 1111,
			}, {
				Host: "some host",
				Port: 2222,
			}},
		}, {
			Name:   "Org2",
			ID:     "Org2MSP",
			MSPDir: "some other dir",
			AnchorPeers: []*localconfig.AnchorPeer{{
				Host: "my host",
				Port: 3333,
			}, {
				Host: "some host",
				Port: 4444,
			}},
		}}

		ordererOrgs := []OrdererConfig{{
			OrganizationName:              "OrdererOrg0",
			Domain:                        "OrdererMSP",
			OrdererNames:                  []string{"orderer0"},
			BrokerCount:                   0,
			ZookeeperCount:                1,
			KafkaMinInsyncReplicas:        2,
			KafkaDefaultReplicationFactor: 3,
		}, {
			OrganizationName:              "OrdererOrg1",
			Domain:                        "OrdererMSP",
			OrdererNames:                  []string{"orderer1"},
			BrokerCount:                   0,
			ZookeeperCount:                2,
			KafkaMinInsyncReplicas:        2,
			KafkaDefaultReplicationFactor: 3,
		}}

		peerOrgs := []PeerOrgConfig{{
			OrganizationName: pOrg[0].Name,
			Domain:           pOrg[0].ID,
			EnableNodeOUs:    true,
			UserCount:        2,
			PeerCount:        2,
		}, {
			OrganizationName: pOrg[1].Name,
			Domain:           pOrg[1].ID,
			EnableNodeOUs:    true,
			UserCount:        2,
			PeerCount:        2,
		}}

		oOrg := []*localconfig.Organization{{
			Name:   ordererOrgs[0].OrganizationName,
			ID:     ordererOrgs[0].Domain,
			MSPDir: "orderer dir",
		}, {
			Name:   ordererOrgs[1].OrganizationName,
			ID:     ordererOrgs[1].Domain,
			MSPDir: "orderer2 dir",
		}}

		crypto := runner.Cryptogen{
			Config: filepath.Join(tempDir, "crypto.yaml"),
			Output: filepath.Join(tempDir, "crypto"),
		}

		deployment := Deployment{
			SystemChannel: "syschannel",
			Channel:       "mychannel",
			Chaincode: Chaincode{
				Name:     "mycc",
				Version:  "1.0",
				Path:     filepath.Join("simple", "cmd"),
				GoPath:   filepath.Join("testdata", "chaincode"),
				ExecPath: os.Getenv("PATH"),
			},
			InitArgs: `{"Args":["init","a","100","b","200"]}`,
			Peers:    []string{"peer0.org1.example.com", "peer0.org2.example.com"},
			Policy:   `OR ('Org1MSP.member','Org2MSP.member')`,
			Orderer:  "127.0.0.1:9050",
		}

		peerProfile := localconfig.Profile{
			Consortium: "MyConsortium",
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
				Brokers: []string{},
			},
			Organizations: oOrg,
			OrdererType:   "solo",
			Addresses:     []string{"0.0.0.0:9050"},
			Capabilities:  map[string]bool{"V1_1": true},
		}

		ordererProfile := localconfig.Profile{
			Application: &localconfig.Application{
				Organizations: oOrg,
				Capabilities:  map[string]bool{"V1_2": true}},
			Orderer: orderer,
			Consortiums: map[string]*localconfig.Consortium{
				"MyConsortium": &localconfig.Consortium{Organizations: pOrg},
			},
			Capabilities: map[string]bool{"V1_1": true},
		}

		profiles := map[string]localconfig.Profile{
			"TwoOrgsChannel":        peerProfile,
			"TwoOrgsOrdererGenesis": ordererProfile,
		}

		w = World{
			Rootpath:           tempDir,
			Components:         components,
			Cryptogen:          crypto,
			Network:            &docker.Network{},
			Deployment:         deployment,
			OrdererOrgs:        ordererOrgs,
			PeerOrgs:           peerOrgs,
			OrdererProfileName: "TwoOrgsOrdererGenesis",
			ChannelProfileName: "TwoOrgsChannel",
			Profiles:           profiles,
		}

		w.Construct()
		Expect(filepath.Join(tempDir, "crypto.yaml")).To(BeARegularFile())

		//Verify that the contents of the files are "golden"
		golden, err := ioutil.ReadFile(filepath.Join("testdata", "crypto.yaml.golden"))
		Expect(err).NotTo(HaveOccurred())
		actual, err := ioutil.ReadFile(filepath.Join(tempDir, "crypto.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(golden)).To(Equal(string(actual)))

		Expect(filepath.Join(tempDir, "configtx.yaml")).To(BeARegularFile())
		golden, err = ioutil.ReadFile(filepath.Join("testdata", "configtx.yaml.golden"))
		Expect(err).NotTo(HaveOccurred())
		actual, err = ioutil.ReadFile(filepath.Join(tempDir, "configtx.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(golden)).To(Equal(string(actual)))
	})

	Context("when world is defined", func() {
		BeforeEach(func() {
			pOrg := []*localconfig.Organization{{
				Name: "Org1ExampleCom",
				ID:   "Org1ExampleCom",
				//ID:     "org1.example.com",
				MSPDir: "crypto/peerOrganizations/org1.example.com/msp",
				AnchorPeers: []*localconfig.AnchorPeer{{
					Host: "127.0.0.1",
					Port: 11051,
				}},
			}, {
				Name: "Org2ExampleCom",
				ID:   "Org2ExampleCom",
				//ID:     "org2.example.com",
				MSPDir: "crypto/peerOrganizations/org2.example.com/msp",
				AnchorPeers: []*localconfig.AnchorPeer{{
					Host: "127.0.0.1",
					Port: 8051,
				}},
			}}

			ordererOrgs := OrdererConfig{
				OrganizationName:              "ExampleCom",
				Domain:                        "example.com",
				OrdererNames:                  []string{"orderer0"},
				BrokerCount:                   0,
				ZookeeperCount:                1,
				KafkaMinInsyncReplicas:        2,
				KafkaDefaultReplicationFactor: 3,
			}

			peerOrgs := []PeerOrgConfig{{
				OrganizationName: pOrg[0].Name,
				Domain:           "org1.example.com",
				EnableNodeOUs:    true,
				UserCount:        2,
				PeerCount:        2,
			}, {
				OrganizationName: pOrg[1].Name,
				Domain:           "org2.example.com",
				EnableNodeOUs:    true,
				UserCount:        2,
				PeerCount:        2,
			}}

			oOrg := []*localconfig.Organization{{
				Name:   ordererOrgs.OrganizationName,
				ID:     "ExampleCom",
				MSPDir: "crypto/ordererOrganizations/example.com/orderers/orderer0.example.com/msp",
			}}

			deployment := Deployment{
				SystemChannel: "syschannel",
				Channel:       "mychannel",
				Chaincode: Chaincode{
					Name:     "mycc",
					Version:  "1.0",
					Path:     filepath.Join("simple", "cmd"),
					GoPath:   filepath.Join(tempDir, "chaincode"),
					ExecPath: os.Getenv("PATH"),
				},
				Orderer:  "127.0.0.1:9050",
				InitArgs: `{"Args":["init","a","100","b","200"]}`,
				Peers:    []string{"peer0.org1.example.com", "peer0.org2.example.com"},
				Policy:   `OR ('Org1ExampleCom.member','Org2ExampleCom.member')`,
			}

			peerProfile := localconfig.Profile{
				Consortium: "MyConsortium",
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
					Brokers: []string{
						"127.0.0.1:9092",
						"127.0.0.1:8092",
						"127.0.0.1:7092",
						"127.0.0.1:6092",
					},
				},
				Organizations: oOrg,
				OrdererType:   "solo",
				Addresses:     []string{"0.0.0.0:9050"},
				Capabilities:  map[string]bool{"V1_1": true},
			}

			ordererProfile := localconfig.Profile{
				Application: &localconfig.Application{
					Organizations: oOrg,
					Capabilities:  map[string]bool{"V1_2": true}},
				Orderer: orderer,
				Consortiums: map[string]*localconfig.Consortium{
					"MyConsortium": &localconfig.Consortium{
						Organizations: append(oOrg, pOrg...),
					},
				},
				Capabilities: map[string]bool{"V1_1": true},
			}

			profiles := map[string]localconfig.Profile{
				"TwoOrgsChannel":        peerProfile,
				"TwoOrgsOrdererGenesis": ordererProfile,
			}

			crypto := runner.Cryptogen{
				Config: filepath.Join(tempDir, "crypto.yaml"),
				Output: filepath.Join(tempDir, "crypto"),
			}

			network, err = client.CreateNetwork(
				docker.CreateNetworkOptions{
					Name:   "mytestnet",
					Driver: "bridge",
				},
			)
			Expect(err).NotTo(HaveOccurred())

			w = World{
				Rootpath:           tempDir,
				Components:         components,
				Cryptogen:          crypto,
				Network:            network,
				Deployment:         deployment,
				OrdererOrgs:        []OrdererConfig{ordererOrgs},
				PeerOrgs:           peerOrgs,
				OrdererProfileName: "TwoOrgsOrdererGenesis",
				ChannelProfileName: "TwoOrgsChannel",
				Profiles:           profiles,
			}
		})

		It("boostraps network", func() {
			w.BootstrapNetwork()
			Expect(filepath.Join(tempDir, "configtx.yaml")).To(BeARegularFile())
			Expect(filepath.Join(tempDir, "crypto.yaml")).To(BeARegularFile())
			Expect(filepath.Join(tempDir, "crypto", "peerOrganizations")).To(BeADirectory())
			Expect(filepath.Join(tempDir, "crypto", "ordererOrganizations")).To(BeADirectory())
			Expect(filepath.Join(tempDir, "syschannel_block.pb")).To(BeARegularFile())
			Expect(filepath.Join(tempDir, "mychannel_tx.pb")).To(BeARegularFile())
			Expect(filepath.Join(tempDir, "Org1ExampleCom_anchors_update_tx.pb")).To(BeARegularFile())
			Expect(filepath.Join(tempDir, "Org2ExampleCom_anchors_update_tx.pb")).To(BeARegularFile())
		})

		It("builds network and sets up channel", func() {
			By("generating the files used to bootstrap the network")
			w.BootstrapNetwork()

			By("setting up the directory structure for peer and orderer configs")
			copyFile(filepath.Join("testdata", "orderer.yaml"), filepath.Join(tempDir, "orderer.yaml"))
			for _, peerOrg := range w.PeerOrgs {
				for peer := 0; peer < peerOrg.PeerCount; peer++ {
					err = os.Mkdir(filepath.Join(tempDir, fmt.Sprintf("%s_%d", peerOrg.Domain, peer)), 0755)
					Expect(err).NotTo(HaveOccurred())
					copyFile(filepath.Join("testdata", fmt.Sprintf("%s_%d-core.yaml", peerOrg.Domain, peer)), filepath.Join(tempDir, fmt.Sprintf("%s_%d/core.yaml", peerOrg.Domain, peer)))
				}
			}
			By("building the network")
			w.BuildNetwork()

			By("setting up and joining the channel")
			copyDir(filepath.Join("testdata", "chaincode"), filepath.Join(tempDir, "chaincode"))
			err = w.SetupChannel()
			Expect(err).NotTo(HaveOccurred())

			By("verifying the chaincode is installed")
			adminPeer := components.Peer()
			adminPeer.LogLevel = "debug"
			adminPeer.ConfigDir = filepath.Join(tempDir, "org1.example.com_0")
			adminPeer.MSPConfigPath = filepath.Join(tempDir, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
			adminRunner := adminPeer.ChaincodeListInstalled()
			adminProcess := ifrit.Invoke(adminRunner)
			Eventually(adminProcess.Ready(), 2*time.Second).Should(BeClosed())
			Eventually(adminProcess.Wait(), 5*time.Second).ShouldNot(Receive(BeNil()))
			Eventually(adminRunner.Buffer()).Should(gbytes.Say("Path: simple/cmd"))
		})
	})
})

func copyFile(src, dest string) {
	data, err := ioutil.ReadFile(src)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(dest, data, 0774)
	Expect(err).NotTo(HaveOccurred())
}

func copyDir(src, dest string) {
	os.MkdirAll(dest, 0755)
	objects, err := ioutil.ReadDir(src)
	for _, obj := range objects {
		srcfileptr := src + "/" + obj.Name()
		destfileptr := dest + "/" + obj.Name()
		if obj.IsDir() {
			copyDir(srcfileptr, destfileptr)
		} else {
			copyFile(srcfileptr, destfileptr)
		}
	}
	Expect(err).NotTo(HaveOccurred())
}
