package smartbft

import (
	"context"
	"encoding/binary"
	"fmt"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/onsi/gomega/gexec"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"github.com/tedsuo/ifrit/grouper"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	ordererProtos "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/ordererclient"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
)

func smartBftBlockDelivererTest(
	testDir string,
	client *docker.Client,
	network *nwo.Network,
	networkProcess ifrit.Process,
	ordererProcesses []ifrit.Process,
	peerProcesses ifrit.Process) {

	/* Create the ledger: start an ordering service, submit TXs to make blocks, shut down, copy the ledger */
	networkProcess = nil
	ordererProcesses = nil
	peerProcesses = nil
	var err error

	/* Create a client*/
	client, err = docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())

	/* Create networkConfig for multiple orderers */
	networkConfig := nwo.MultiNodeSmartBFT()
	networkConfig.Channels = nil

	channel := "testchannel1"

	fmt.Println(client.Endpoint())

	/* Create a network from the networkConfig */
	startPort := StartPort()
	print(startPort)
	network = nwo.New(networkConfig, testDir, client, startPort, components)
	network.GenerateConfigTree()
	network.Bootstrap()

	var ordererRunners []*ginkgomon.Runner
	for _, orderer := range network.Orderers {
		runner := network.OrdererRunner(orderer)
		runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
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
	allStreams := []ordererProtos.AtomicBroadcast_BroadcastClient{}
	for _, o := range network.Orderers {
		conn := network.OrdererClientConn(o)
		broadcaster, err := ordererProtos.NewAtomicBroadcastClient(conn).Broadcast(context.Background())
		Expect(err).ToNot(HaveOccurred())
		allStreams = append(allStreams, broadcaster)
	}

	/* Fill the ledger with blocks */
	By("Filling ledger with blocks")
	closeTxProducer := make(chan bool)
	go func() {
		defer GinkgoRecover()
		var i uint64
		for {
			buff := make([]byte, 8)
			binary.BigEndian.PutUint64(buff, i)

			/* Create transaction envelope which will be broadcast to all orderers */
			env := ordererclient.CreateBroadcastEnvelope(network, network.Orderers[1], channel, buff)

			for _, stream := range allStreams {
				err := stream.Send(env)
				Expect(err).NotTo(HaveOccurred())
			}
			i++

			finishSending := false
			select {
			case <-closeTxProducer:
				finishSending = true
			case <-time.After(time.Millisecond):
			}
			if finishSending {
				return
			}
		}
	}()
	Eventually(ordererRunners[1].Err(), 10*time.Second).Should(gbytes.Say("Delivering proposal, writing block 10"))
	closeTxProducer <- true

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
	ledgerArray := []*common.Block{}
	for i := uint64(0); i < fileLedger.Height(); i++ {
		block, err := fileLedger.RetrieveBlockByNumber(i)
		Expect(err).NotTo(HaveOccurred())
		ledgerArray = append(ledgerArray, block)
	}

	for i, block := range ledgerArray {
		if i == 5 {
			println(block.Data)
			println(block.Header)
			println(block.Metadata)
			println(block.String())
		}
	}

	/* Start the mocks */
	By("Starting mock orderer servers")
	for _, orderer := range network.Orderers {
		/* Extract keys to pass to mock */
		certPath := filepath.Join(network.OrdererLocalTLSDir(orderer), "server.crt")
		keyPath := filepath.Join(network.OrdererLocalTLSDir(orderer), "server.key")
		rootCaPath := filepath.Join(network.OrdererLocalTLSDir(orderer), "ca.crt")

		certMspPath := filepath.Join(network.OrdererLocalMSPDir(orderer), "server.crt")
		println(certMspPath)

		rootCaCert, err := os.ReadFile(rootCaPath)
		Expect(err).NotTo(HaveOccurred())

		key, err := os.ReadFile(keyPath)
		Expect(err).NotTo(HaveOccurred())

		cert, err := os.ReadFile(certPath)
		Expect(err).NotTo(HaveOccurred())

		fmt.Printf("\n\nrootCaCert=%v\nkey=%v\ncert=%v\n\n", rootCaCert, key, cert)

		ms, err := NewMockServer(ledgerArray,
			fileLedger,
			cert,
			key,
			[][]byte{rootCaCert},
			network.OrdererAddress(orderer, nwo.ListenPort),
		)
		ms.logger.Info("")
	}
	By("Create a peer and join to channel")

	p0 := network.Peers[0]
	fmt.Println(network.PeerAddress(p0, nwo.ListenPort))
	peerRunner := grouper.NewParallel(syscall.SIGTERM, grouper.Members{grouper.Member{Name: p0.ID(), Runner: network.PeerRunner(p0)}})
	peerProcesses = ifrit.Invoke(peerRunner)
	Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())
	/* make peer join channel */
	peer := network.Peer("Org1", "peer0")
	sess, err := network.PeerAdminSession(peer, commands.ChannelJoin{
		BlockPath:  network.OutputBlockPath(channel),
		ClientAuth: network.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

	<-time.After(600 * time.Second)

	panic("!")
}
