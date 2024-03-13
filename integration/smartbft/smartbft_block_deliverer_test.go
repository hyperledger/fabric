/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hyperledger/fabric/integration/nwo/commands"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	"github.com/hyperledger/fabric-protos-go/common"
	ordererProtos "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/ordererclient"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("Smart BFT Block Deliverer", func() {
	var (
		testDir          string
		client           *docker.Client
		network          *nwo.Network
		_                ifrit.Process
		ordererProcesses []ifrit.Process
		peerProcesses    ifrit.Process
		mocksArray       []*MockOrderer
	)

	BeforeEach(func() {
		ordererProcesses = nil
		peerProcesses = nil
		var err error
		testDir, err = os.MkdirTemp("", "smartbft-block-deliverer-test")
		Expect(err).NotTo(HaveOccurred())

		/* Create a client*/
		client, err = docker.NewClientFromEnv()

		Expect(err).NotTo(HaveOccurred())

		/* Create networkConfig for multiple orderers */
		networkConfig := nwo.MultiNodeSmartBFT()
		networkConfig.Channels = nil

		channel := "testchannel1"

		/* Create a network from the networkConfig */
		startPort := StartPort()
		network = nwo.New(networkConfig, testDir, client, startPort, components)
		network.GenerateConfigTree()
		network.Bootstrap()

		var ordererRunners []*ginkgomon.Runner
		for _, orderer := range network.Orderers {
			runner := network.OrdererRunner(orderer)
			runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=debug")
			ordererRunners = append(ordererRunners, runner)
			proc := ifrit.Invoke(runner)
			ordererProcesses = append(ordererProcesses, proc)
			Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
		}

		joinChannel(network, channel)

		By("Waiting for followers to see the leader")
		Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
		Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
		Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

		/* Create a stream with client for each orderer*/
		var allStreams []ordererProtos.AtomicBroadcast_BroadcastClient
		for _, o := range network.Orderers {
			conn := network.OrdererClientConn(o)
			broadcaster, err := ordererProtos.NewAtomicBroadcastClient(conn).Broadcast(context.Background())
			Expect(err).ToNot(HaveOccurred())
			allStreams = append(allStreams, broadcaster)
		}

		/* Fill the ledger with blocks */
		By("Filling ledger with blocks")

		for i := 0; i < 10; i++ {
			buff := make([]byte, 8)
			binary.BigEndian.PutUint64(buff, uint64(i))

			/* Create transaction envelope which will be broadcast to all orderers */
			env := ordererclient.CreateBroadcastEnvelope(network, network.Orderers[0], channel, buff, common.HeaderType_ENDORSER_TRANSACTION)

			for _, stream := range allStreams {
				err := stream.Send(env)
				Expect(err).NotTo(HaveOccurred())
			}
		}
		Eventually(ordererRunners[0].Err(), 20*time.Second).Should(gbytes.Say("Delivering proposal, writing block 10"))
		Eventually(ordererRunners[1].Err(), 20*time.Second).Should(gbytes.Say("Delivering proposal, writing block 10"))
		Eventually(ordererRunners[2].Err(), 20*time.Second).Should(gbytes.Say("Delivering proposal, writing block 10"))
		Eventually(ordererRunners[3].Err(), 20*time.Second).Should(gbytes.Say("Delivering proposal, writing block 10"))

		/* Shut down the orderers */
		By("Taking down all the orderers")
		for _, proc := range ordererProcesses {
			proc.Signal(syscall.SIGINT)
			Eventually(proc.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		/* Extract ledger from one of the orderers */
		By("Extracting blocks from ledger")
		ordererFabricConfig := network.ReadOrdererConfig(network.Orderers[0])
		blockStoreProvider, err := blkstorage.NewProvider(
			blkstorage.NewConf(ordererFabricConfig.FileLedger.Location, -1),
			&blkstorage.IndexConfig{
				AttrsToIndex: []blkstorage.IndexableAttr{blkstorage.IndexableAttrBlockNum},
			},
			&disabled.Provider{},
		)
		Expect(err).NotTo(HaveOccurred())
		blockStore, err := blockStoreProvider.Open(channel)
		Expect(err).NotTo(HaveOccurred())
		fileLedger := fileledger.NewFileLedger(blockStore)
		var ledgerArray []*common.Block
		for i := uint64(0); i < fileLedger.Height(); i++ {
			block, err := fileLedger.RetrieveBlockByNumber(i)
			Expect(err).NotTo(HaveOccurred())
			ledgerArray = append(ledgerArray, block)
		}
		/* The ledger now saved and can be accessed at ledgerArray */

		/* Create orderer mocks and make sure they can access the ledger */

		for _, orderer := range network.Orderers {
			serverTLSCerts := map[string][]byte{}
			tlsCertPath := filepath.Join(network.OrdererLocalTLSDir(orderer), "server.crt")
			serverTLSCerts[tlsCertPath], err = os.ReadFile(tlsCertPath)
			Expect(err).NotTo(HaveOccurred())

			serverTLSKeys := map[string][]byte{}
			tlsKeyPath := filepath.Join(network.OrdererLocalTLSDir(orderer), "server.key")
			serverTLSKeys[tlsKeyPath], err = os.ReadFile(tlsKeyPath)
			Expect(err).NotTo(HaveOccurred())

			secOpts := comm.SecureOptions{
				UseTLS:      true,
				Certificate: serverTLSCerts[tlsCertPath],
				Key:         serverTLSKeys[tlsKeyPath],
			}
			mo, err := NewMockOrderer(network.OrdererAddress(orderer, nwo.ListenPort), ledgerArray, secOpts)
			Expect(err).NotTo(HaveOccurred())
			mocksArray = append(mocksArray, mo)
			mocksArray[len(mocksArray)-1].logger.Infof("Mock orderer started at port %v", network.OrdererAddress(orderer, nwo.ListenPort))
		}
	})

	AfterEach(func() {
		for _, mock := range mocksArray {
			mock.grpcServer.Stop()
		}
		if peerProcesses != nil {
			peerProcesses.Signal(syscall.SIGTERM)
			Eventually(peerProcesses.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		for _, ordererInstance := range ordererProcesses {
			ordererInstance.Signal(syscall.SIGTERM)
			Eventually(ordererInstance.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		_ = os.RemoveAll(testDir)
	})

	It("correct mock", func() {
		var err error
		channel := "testchannel1"

		/* Create peer */
		By("Create a peer and join to channel")
		p0 := network.Peers[0]
		peerRunner := network.PeerRunner(p0)
		peerProcesses = ifrit.Invoke(peerRunner)
		Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())

		_, err = network.PeerAdminSession(p0, commands.ChannelJoin{
			BlockPath:  network.OutputBlockPath(channel),
			ClientAuth: network.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())

		nwo.WaitUntilEqualLedgerHeight(network, channel, 11, network.Peers[0])
	})

	It("block censorship", func() {
		var err error
		channel := "testchannel1"

		for _, mock := range mocksArray {
			mock.censorDataMode = true
			mock.stopDeliveryChannel = make(chan struct{})
		}

		/* Create peer */
		By("Create a peer and join to channel")
		p0 := network.Peers[0]
		peerRunner := network.PeerRunner(p0)
		peerRunner.Command.Env = append(peerRunner.Command.Env, "FABRIC_LOGGING_SPEC=debug")
		peerProcesses = ifrit.Invoke(peerRunner)
		Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())

		_, err = network.PeerAdminSession(p0, commands.ChannelJoin{
			BlockPath:  network.OutputBlockPath(channel),
			ClientAuth: network.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(peerRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Block censorship detected"))
		nwo.WaitUntilEqualLedgerHeight(network, channel, 11, network.Peers[0])

		for _, mock := range mocksArray {
			close(mock.stopDeliveryChannel)
		}
	})
})
