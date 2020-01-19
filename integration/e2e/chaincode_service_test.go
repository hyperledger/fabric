/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/fabricconfig"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("Chaincode Service", func() {
	var (
		testDir string
		network *nwo.Network
		extcc   nwo.Chaincode
		process ifrit.Process
		extbldr fabricconfig.ExternalBuilder
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e-chaincode-service")
		Expect(err).NotTo(HaveOccurred())
		extcc = nwo.Chaincode{
			Name:            "mycc",
			Version:         "0.0",
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/extcc"),
			Lang:            "extcc",
			PackageFile:     filepath.Join(testDir, "extcc.tar.gz"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			SignaturePolicy: `AND ('Org1MSP.member','Org2MSP.member')`,
			Sequence:        "1",
			InitRequired:    true,
			Label:           "my_extcc_chaincode",
			Server:          true,
		}

		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		extbldr = fabricconfig.ExternalBuilder{
			Path: filepath.Join(cwd, "..", "externalbuilders", "extcc"),
			Name: "extcc",
		}
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	Describe("basic solo network with 2 orgs and external chaincode service", func() {
		var (
			datagramReader *DatagramReader
			ccserver       ifrit.Process
		)

		BeforeEach(func() {
			datagramReader = NewDatagramReader()
			go datagramReader.Start()

			network = nwo.New(nwo.BasicSoloWithChaincodeServers(&extcc), testDir, nil, StartPort(), components)

			//add extcc builder
			network.ExternalBuilders = append(network.ExternalBuilders, extbldr)

			network.MetricsProvider = "statsd"
			network.StatsdEndpoint = datagramReader.Address()
			network.Profiles = append(network.Profiles, &nwo.Profile{
				Name:          "TwoOrgsBaseProfileChannel",
				Consortium:    "SampleConsortium",
				Orderers:      []string{"orderer"},
				Organizations: []string{"Org1", "Org2"},
			})
			network.Channels = append(network.Channels, &nwo.Channel{
				Name:        "baseprofilechannel",
				Profile:     "TwoOrgsBaseProfileChannel",
				BaseProfile: "TwoOrgsOrdererGenesis",
			})

			network.GenerateConfigTree()

			// package connection.json
			extcc.CodeFiles = map[string]string{
				network.ChaincodeConnectionPath(&extcc): "connection.json",
			}

			for _, peer := range network.PeersWithChannel("testchannel") {
				core := network.ReadPeerConfig(peer)
				core.VM = nil
				network.WritePeerConfig(peer, core)
			}
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		AfterEach(func() {
			if datagramReader != nil {
				datagramReader.Close()
			}
			if ccserver != nil {
				ccserver.Signal(syscall.SIGTERM)
				Eventually(ccserver.Wait(), network.EventuallyTimeout).Should(Receive())
			}
		})

		It("executes a basic solo network with 2 orgs and external chaincode service", func() {
			By("getting the orderer by name")
			orderer := network.Orderer("orderer")

			By("setting up the channel")
			network.CreateAndJoinChannel(orderer, "testchannel")
			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))

			By("deploying the chaincode")
			nwo.DeployChaincode(network, "testchannel", orderer, extcc)

			By("starting the chaincode service")
			extcc.SetPackageIDFromPackageFile()
			address := network.ChaincodeServerAddress(&extcc)
			Expect(address).NotTo(Equal(""))

			// start external chaincode service
			ccrunner := network.ChaincodeServerRunner(extcc, []string{extcc.PackageID, address})
			ccserver = ifrit.Invoke(ccrunner)
			Eventually(ccserver.Ready(), network.EventuallyTimeout).Should(BeClosed())

			// init the chaincode, if required
			if extcc.InitRequired {
				By("initing the chaincode")
				nwo.InitChaincode(network, "testchannel", orderer, extcc, network.PeersWithChannel("testchannel")...)
			}

			By("getting the client peer by name")
			peer := network.Peer("Org1", "peer0")

			RunQueryInvokeQuery(network, orderer, peer, "testchannel")
			RunRespondWith(network, orderer, peer, "testchannel")

			By("waiting for DeliverFiltered stats to be emitted")
			metricsWriteInterval := 5 * time.Second
			Eventually(datagramReader, 2*metricsWriteInterval).Should(gbytes.Say("stream_request_duration.protos_Deliver.DeliverFiltered."))

			CheckPeerStatsdStreamMetrics(datagramReader.String())
			CheckPeerStatsdMetrics(datagramReader.String(), "org1_peer0")
			CheckPeerStatsdMetrics(datagramReader.String(), "org2_peer0")
			CheckOrdererStatsdMetrics(datagramReader.String(), "ordererorg_orderer")

			By("setting up a channel from a base profile")
			additionalPeer := network.Peer("Org2", "peer0")
			network.CreateChannel("baseprofilechannel", orderer, peer, additionalPeer)
		})
	})
})
