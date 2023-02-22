/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/container/externalbuilder"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("ChaincodeAsExternalServer", func() {
	var (
		testDir                     string
		network                     *nwo.Network
		chaincode                   nwo.Chaincode
		chaincodeServerAddress      string
		assetDir                    string
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process
		ccserver                    ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "external-chaincode-server")
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), testDir, nil, StartPort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		// Create the configuration
		chaincodeServerAddress = fmt.Sprintf("127.0.0.1:%d", network.ReservePort())
		connData, serverKeyPair := generateChaincodeConfig(chaincodeServerAddress)

		// Create directory for configuration files
		assetDir, err = ioutil.TempDir(testDir, "assets")
		Expect(err).NotTo(HaveOccurred())

		// Write the config files
		connJSON, err := json.Marshal(connData)
		Expect(err).NotTo(HaveOccurred())
		err = ioutil.WriteFile(filepath.Join(assetDir, "connection.json"), connJSON, 0o644)
		Expect(err).NotTo(HaveOccurred())

		configJSON, err := json.Marshal(serverKeyPair)
		Expect(err).NotTo(HaveOccurred())
		err = ioutil.WriteFile(filepath.Join(assetDir, "config.json"), configJSON, 0o644)
		Expect(err).NotTo(HaveOccurred())

		// Setup the network
		ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")

		channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", network.Orderer("orderer"), ordererRunner)

		nwo.EnableCapabilities(
			network,
			"testchannel",
			"Application", "V2_0",
			network.Orderer("orderer"),
			network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"),
		)

		// set to use the 'ccaas' builder rathern than binary
		// binary build remains as an example of a scripted approach
		chaincode = nwo.Chaincode{
			Name:            "mycc",
			Version:         "0.0",
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/server"),
			Lang:            "ccaas",
			PackageFile:     filepath.Join(testDir, "server.tar.gz"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			InitRequired:    true,
			SignaturePolicy: `AND ('Org1MSP.member','Org2MSP.member')`,
			Sequence:        "1",
			Label:           "my_server_chaincode",
			CodeFiles: map[string]string{
				filepath.Join(assetDir, "connection.json"): "connection.json",
				filepath.Join(assetDir, "config.json"):     "config.json",
			},
		}
	})

	AfterEach(func() {
		if ccserver != nil {
			ccserver.Signal(syscall.SIGTERM)
			Eventually(ccserver.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if peerProcess != nil {
			peerProcess.Signal(syscall.SIGTERM)
			Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if ordererProcess != nil {
			ordererProcess.Signal(syscall.SIGTERM)
			Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	It("executes a basic etcdraft network with 2 orgs and external chaincode server", func() {
		orderer := network.Orderer("orderer")
		peer := network.Peer("Org1", "peer0")
		peers := network.PeersWithChannel("testchannel")

		By("packaging and installing the chaincode")
		nwo.PackageAndInstallChaincode(network, chaincode, peers...)
		chaincode.SetPackageIDFromPackageFile()

		By("approving and committing the chaincode")
		nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, peers...)
		nwo.CheckCommitReadinessUntilReady(network, "testchannel", chaincode, network.PeerOrgs(), peers...)
		nwo.CommitChaincode(network, "testchannel", orderer, chaincode, peer, peers...)

		By("starting the chaincode server")
		ccserver = ifrit.Invoke(ginkgomon.New(ginkgomon.Config{
			Name: chaincode.PackageID,
			Command: &exec.Cmd{
				Path: chaincode.Path,
				Args: []string{chaincode.Path, chaincode.PackageID, chaincodeServerAddress},
				Dir:  assetDir,
			},
			StartCheck:        `Starting chaincode ` + chaincode.PackageID + ` at ` + chaincodeServerAddress,
			StartCheckTimeout: 15 * time.Second,
		}))
		Eventually(ccserver.Ready(), network.EventuallyTimeout).Should(BeClosed())

		By("exercising the chaincode")
		nwo.InitChaincode(network, "testchannel", orderer, chaincode, network.PeersWithChannel("testchannel")...)
		RunQueryInvokeQuery(network, orderer, peer, "testchannel")
		RunRespondWith(network, orderer, peer, "testchannel")
	})
})

type chaincodeConfig struct {
	ListenAddress string `json:"listen_address,omitempty"`
	Key           string `json:"key,omitempty"`  // PEM encoded key
	Cert          string `json:"cert,omitempty"` // PEM encoded certificate
	CA            string `json:"ca,omitempty"`   // PEM encode CA certificate
}

func generateChaincodeConfig(chaincodeAddress string) (externalbuilder.ChaincodeServerUserData, chaincodeConfig) {
	tlsCA, err := tlsgen.NewCA()
	Expect(err).NotTo(HaveOccurred())

	serverPair, err := tlsCA.NewServerCertKeyPair("127.0.0.1")
	Expect(err).NotTo(HaveOccurred())
	clientPair, err := tlsCA.NewClientCertKeyPair()
	Expect(err).NotTo(HaveOccurred())

	config := chaincodeConfig{
		ListenAddress: chaincodeAddress,
		Key:           string(serverPair.Key),
		Cert:          string(serverPair.Cert),
		CA:            string(tlsCA.CertBytes()),
	}

	connData := externalbuilder.ChaincodeServerUserData{
		Address:            chaincodeAddress,
		DialTimeout:        externalbuilder.Duration(10 * time.Second),
		TLSRequired:        true,
		ClientAuthRequired: true,
		ClientKey:          string(clientPair.Key),
		ClientCert:         string(clientPair.Cert),
		RootCert:           string(tlsCA.CertBytes()),
	}

	return connData, config
}
