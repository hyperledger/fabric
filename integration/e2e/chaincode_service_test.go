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
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/fabricconfig"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("ChaincodeAsExternalService", func() {
	var (
		testDir                 string
		network                 *nwo.Network
		extcc                   nwo.Chaincode
		chaincodeServerAddrress string
		certFiles               []string
		process                 ifrit.Process
		extbldr                 fabricconfig.ExternalBuilder
		ccserver                ifrit.Process
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
			Label:           "my_extcc_chaincode",
		}

		extbldr = fabricconfig.ExternalBuilder{
			Path: filepath.Join("..", "externalbuilders", "extcc"),
			Name: "extcc",
		}

		network = nwo.New(nwo.BasicSolo(), testDir, nil, StartPort(), components)

		chaincodeServerAddrress = fmt.Sprintf("127.0.0.1:%d", network.ReservePort())

		tlsCA, err := tlsgen.NewCA()
		Expect(err).NotTo(HaveOccurred())
		certFiles = generateCertKeysAndConnectionFiles(tlsCA, testDir, extcc.Name)
		chaincodeConnectionsFile := generateClientConnectionFile(tlsCA, chaincodeServerAddrress, testDir, extcc.Name)

		//add extcc builder
		network.ExternalBuilders = append(network.ExternalBuilders, extbldr)

		network.GenerateConfigTree()

		// package connection.json
		extcc.CodeFiles = map[string]string{
			chaincodeConnectionsFile: "connection.json",
		}

		network.Bootstrap()

		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
	})

	AfterEach(func() {
		if ccserver != nil {
			ccserver.Signal(syscall.SIGTERM)
			Eventually(ccserver.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	It("executes a basic solo network with 2 orgs and external chaincode service", func() {
		orderer := network.Orderer("orderer")

		By("setting up the channel")
		network.CreateAndJoinChannel(orderer, "testchannel")
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))

		By("deploying the chaincode")
		nwo.DeployChaincode(network, "testchannel", orderer, extcc)

		By("starting the chaincode service")
		extcc.SetPackageIDFromPackageFile()

		// start external chain code service
		ccrunner := chaincodeServerRunner(extcc.Path, extcc.PackageID, append([]string{extcc.PackageID, chaincodeServerAddrress}, certFiles...))
		ccserver = ifrit.Invoke(ccrunner)
		Eventually(ccserver.Ready(), network.EventuallyTimeout).Should(BeClosed())

		peer := network.Peer("Org1", "peer0")

		RunRespondWith(network, orderer, peer, "testchannel")
	})
})

func generateCertKeysAndConnectionFiles(tlsCA tlsgen.CA, testDir string, chaincodeID string) []string {
	certsDir := filepath.Join(testDir, "certs", chaincodeID)

	// Generate key files for chaincode server
	err := os.MkdirAll(certsDir, 0755)
	Expect(err).NotTo(HaveOccurred())

	serverKeyFile := filepath.Join(certsDir, "key.pem")
	serverCertFile := filepath.Join(certsDir, "cert.pem")
	clientCAFile := filepath.Join(certsDir, "clientCA.pem")

	serverPair, err := tlsCA.NewServerCertKeyPair("127.0.0.1")
	cert := serverPair.Cert
	key := serverPair.Key

	err = ioutil.WriteFile(serverKeyFile, key, 0644)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(serverCertFile, cert, 0644)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(clientCAFile, tlsCA.CertBytes(), 0644)
	Expect(err).NotTo(HaveOccurred())

	return []string{serverKeyFile, serverCertFile, clientCAFile}
}

func generateClientConnectionFile(tlsCA tlsgen.CA, chaincodeServerAddrress string, testDir string, chaincodeID string) string {
	clientPair, err := tlsCA.NewClientCertKeyPair()
	Expect(err).NotTo(HaveOccurred())
	clientKey := clientPair.Key
	clientCert := clientPair.Cert

	connectionsDir := filepath.Join(testDir, "chaincode-connections", chaincodeID)
	err = os.MkdirAll(connectionsDir, 0755)
	Expect(err).NotTo(HaveOccurred())

	data := externalbuilder.ChaincodeServerUserData{
		Address:            chaincodeServerAddrress,
		DialTimeout:        externalbuilder.Duration{Duration: 10 * time.Second},
		TLSRequired:        true,
		ClientAuthRequired: true,
		ClientKey:          string(clientKey),
		ClientCert:         string(clientCert),
		RootCert:           string(tlsCA.CertBytes()),
	}

	bdata, err := json.Marshal(data)
	Expect(err).NotTo(HaveOccurred())

	chaincodeConnectionsFile := filepath.Join(connectionsDir, "connection.json")
	ioutil.WriteFile(chaincodeConnectionsFile, bdata, 0644)
	Expect(err).NotTo(HaveOccurred())

	return chaincodeConnectionsFile
}

func chaincodeServerRunner(path string, packageID string, args []string) *ginkgomon.Runner {
	cmd := exec.Command(path, args...)
	cmd.Env = os.Environ()

	return ginkgomon.New(ginkgomon.Config{
		Name:              packageID,
		Command:           cmd,
		StartCheck:        `Starting chaincode .* at .*`,
		StartCheckTimeout: 15 * time.Second,
	})
}
