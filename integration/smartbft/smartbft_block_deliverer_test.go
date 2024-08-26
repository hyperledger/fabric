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
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	ordererProtos "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
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

func extractLedger(network *nwo.Network, orderer *nwo.Orderer, channelId string) []*common.Block {
	By("Extracting blocks from ledger")
	ordererFabricConfig := network.ReadOrdererConfig(orderer)
	blockStoreProvider, err := blkstorage.NewProvider(
		blkstorage.NewConf(ordererFabricConfig.FileLedger.Location, -1),
		&blkstorage.IndexConfig{
			AttrsToIndex: []blkstorage.IndexableAttr{blkstorage.IndexableAttrBlockNum},
		},
		&disabled.Provider{},
	)
	Expect(err).NotTo(HaveOccurred())
	blockStore, err := blockStoreProvider.Open(channelId)
	Expect(err).NotTo(HaveOccurred())
	fileLedger := fileledger.NewFileLedger(blockStore)
	var ledgerArray []*common.Block
	for i := uint64(0); i < fileLedger.Height(); i++ {
		block, err := fileLedger.RetrieveBlockByNumber(i)
		Expect(err).NotTo(HaveOccurred())
		ledgerArray = append(ledgerArray, block)
	}
	return ledgerArray
}

var _ = Describe("Smart BFT Block Deliverer", func() {
	var (
		testDir          string
		client           *docker.Client
		network          *nwo.Network
		_                ifrit.Process
		ordererProcesses []ifrit.Process
		peerProcesses    ifrit.Process
		mocksArray       []*MockOrderer
		ledgerArray      []*common.Block
		ordererRunners   []*ginkgomon.Runner
		allStreams       []ordererProtos.AtomicBroadcast_BroadcastClient
		channel          string
		peer             *nwo.Peer
	)

	BeforeEach(func() {
		ordererProcesses = nil
		peerProcesses = nil
		mocksArray = nil
		ledgerArray = nil
		peer = nil
		ordererRunners = nil
		allStreams = nil
		var err error
		testDir, err = os.MkdirTemp("", "smartbft-block-deliverer-test")
		Expect(err).NotTo(HaveOccurred())

		/* Create a client*/
		client, err = docker.NewClientFromEnv()

		Expect(err).NotTo(HaveOccurred())

		/* Create networkConfig for multiple orderers */
		networkConfig := nwo.MultiNodeSmartBFT()
		networkConfig.Channels = nil
		channel = "testchannel1"

		/* Create a network from the networkConfig */
		network = nwo.New(networkConfig, testDir, client, StartPort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		for _, orderer := range network.Orderers {
			runner := network.OrdererRunner(orderer,
				"FABRIC_LOGGING_SPEC=debug",
				"ORDERER_GENERAL_BACKOFF_MAXDELAY=20s",
			)
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

		peer = network.Peers[0]

		/* Create a stream with client for each orderer*/
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
		assertBlockReception(map[string]int{channel: 10}, network.Orderers, peer, network)
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

		ledgerArray = nil

		for _, ordererInstance := range ordererProcesses {
			ordererInstance.Signal(syscall.SIGTERM)
			Eventually(ordererInstance.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		_ = os.RemoveAll(testDir)
	})

	It("correct mock", func() {
		/* Create orderer mocks and make sure they can access the ledger */
		var err error

		/* Shut down the orderers */
		By("Taking down all the orderers")
		for _, proc := range ordererProcesses {
			proc.Signal(syscall.SIGINT)
			Eventually(proc.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		ledgerArray = extractLedger(network, network.Orderers[0], channel)

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

	It("block censorship peer", func() {
		var err error

		/* Shut down the orderers */
		By("Taking down all the orderers")
		for _, proc := range ordererProcesses {
			proc.Signal(syscall.SIGINT)
			Eventually(proc.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		ledgerArray = extractLedger(network, network.Orderers[0], channel)

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

		for _, mock := range mocksArray {
			mock.StartCensoring(5)
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
		// After delivery is stopped due to censoring, stop the censoring on all the other mocks
		var censoringOrderer *MockOrderer
	censoringMockDetection:
		for {
			for _, mock := range mocksArray {
				if mock.StopDelivery {
					censoringOrderer = mock
					for _, otherMock := range mocksArray {
						if mock.address == otherMock.address {
							continue
						}
						otherMock.StopCensoring()
					}
					break censoringMockDetection
				}
			}
			time.Sleep(time.Second)
		}
		Eventually(peerRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Block censorship detected"))
		nwo.WaitUntilEqualLedgerHeight(network, channel, 11, network.Peers[0])

		close(censoringOrderer.StopDeliveryChannel)
	})

	It("block censorship orderer", func() {
		var err error

		By("Taking down the last orderer")
		o4Proc := ordererProcesses[3]
		o4Proc.Signal(syscall.SIGINT)
		Eventually(o4Proc.Wait(), network.EventuallyTimeout).Should(Receive())

		By("Send 10 more blocks in network")
		for i := 0; i < 10; i++ {
			buff := make([]byte, 8)
			binary.BigEndian.PutUint64(buff, uint64(i))

			/* Create transaction envelope which will be broadcast to all orderers */
			env := ordererclient.CreateBroadcastEnvelope(network, network.Orderers[0], channel, buff, common.HeaderType_ENDORSER_TRANSACTION)

			for i, stream := range allStreams {
				if i == 3 {
					continue
				}
				err := stream.Send(env)
				Expect(err).NotTo(HaveOccurred())
			}
		}
		assertBlockReception(map[string]int{channel: 20}, network.Orderers[:3], peer, network)

		By("Stop the rest of the orderers")
		for i, proc := range ordererProcesses {
			if i == 3 {
				continue
			}
			proc.Signal(syscall.SIGINT)
			Eventually(proc.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		By("Copy ledger of the first orderer")
		ledgerArray = extractLedger(network, network.Orderers[0], channel)

		By("Create mocks instead of orderers 1-3")
		for _, orderer := range network.Orderers[:3] {
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
			ordererAddress := network.OrdererAddress(orderer, nwo.ListenPort)
			mo, err := NewMockOrderer(ordererAddress, ledgerArray, secOpts)
			Expect(err).NotTo(HaveOccurred())
			mocksArray = append(mocksArray, mo)
			mocksArray[len(mocksArray)-1].logger.Infof("Mock orderer started at port %v", ordererAddress)
		}

		By("Making mocks of orderers 1-3 censor blocks")
		for _, mock := range mocksArray {
			mock.StartCensoring(5)
		}

		By("Bring up the last orderer")
		o4Runner := network.OrdererRunner(network.Orderers[3], "FABRIC_LOGGING_SPEC=debug")
		o4Proc = ifrit.Invoke(o4Runner)
		ordererProcesses[3] = o4Proc

		Eventually(o4Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

		By("Assert block censorship detected")
		// After delivery is stopped due to censoring, stop the censoring on all the other mocks
		var censoringOrderer *MockOrderer
	censoringMockDetection:
		for {
			for _, mock := range mocksArray {
				if mock.StopDelivery {
					censoringOrderer = mock
					for _, otherMock := range mocksArray {
						if mock.address == otherMock.address {
							continue
						}
						otherMock.StopCensoring()
					}
					break censoringMockDetection
				}
			}
			time.Sleep(time.Second)
		}
		Eventually(o4Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Block censorship detected"))

		By("Assert all block are received")
		assertBlockReception(map[string]int{channel: 20}, []*nwo.Orderer{network.Orderers[3]}, peer, network)

		close(censoringOrderer.StopDeliveryChannel)
	})
})
