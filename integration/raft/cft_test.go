/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package raft

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hyperledger/fabric/integration/channelparticipation"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	conftx "github.com/hyperledger/fabric-config/configtx"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/ordererclient"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"github.com/tedsuo/ifrit/grouper"
)

var _ = Describe("EndToEnd Crash Fault Tolerance", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network
		peer    *nwo.Peer

		o1Runner, o2Runner, o3Runner        *ginkgomon.Runner
		ordererProc, o1Proc, o2Proc, o3Proc ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		for _, oProc := range []ifrit.Process{o1Proc, o2Proc, o3Proc, ordererProc} {
			if oProc != nil {
				oProc.Signal(syscall.SIGTERM)
				Eventually(oProc.Wait(), network.EventuallyTimeout).Should(Receive())
			}
		}

		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	When("orderer stops and restarts", func() {
		It("keeps network up and running", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaftNoSysChan(), testDir, client, StartPort(), components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			network.GenerateConfigTree()
			network.Bootstrap()

			o1Runner = network.OrdererRunner(o1)
			// Enable debug log for orderer2 so we could assert its content later
			o2Runner = network.OrdererRunner(o2, "FABRIC_LOGGING_SPEC=orderer.consensus.etcdraft=debug:info")
			o3Runner = network.OrdererRunner(o3)

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)
			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", o1, o2, o3)
			FindLeader([]*ginkgomon.Runner{o1Runner, o2Runner, o3Runner})

			By("performing operation with orderer1")
			env := CreateBroadcastEnvelope(network, o1, "testchannel", []byte("foo"))
			resp, err := ordererclient.Broadcast(network, o1, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			block := FetchBlock(network, o1, 1, "testchannel")
			Expect(block).NotTo(BeNil())

			By("killing orderer1")
			o1Proc.Signal(syscall.SIGKILL)
			Eventually(o1Proc.Wait(), network.EventuallyTimeout).Should(Receive(MatchError("exit status 137")))

			By("observing active nodes to shrink")
			Eventually(o2Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Current active nodes in cluster are: \\[2 3\\]"))

			By("broadcasting envelope to running orderer")
			resp, err = ordererclient.Broadcast(network, o2, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			block = FetchBlock(network, o2, 2, "testchannel")
			Expect(block).NotTo(BeNil())

			By("restarting orderer1")
			o1Runner = network.OrdererRunner(o1)
			o1Proc = ifrit.Invoke(o1Runner)
			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			FindLeader([]*ginkgomon.Runner{o1Runner})

			By("broadcasting envelope to restarted orderer")
			resp, err = ordererclient.Broadcast(network, o1, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			blko1 := FetchBlock(network, o1, 3, "testchannel")
			blko2 := FetchBlock(network, o2, 3, "testchannel")

			Expect(blko1.Header.DataHash).To(Equal(blko2.Header.DataHash))
		})
	})

	When("an orderer is behind the latest snapshot on leader", func() {
		It("catches up using the block stored in snapshot", func() {
			// Steps:
			// - start o2 & o3
			// - send several transactions so snapshot is created
			// - kill o2 & o3, so that entries prior to snapshot are not in memory upon restart
			// - start o1 & o2
			// - assert that o1 can catch up with o2 using snapshot
			network = nwo.New(nwo.MultiNodeEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			network.GenerateConfigTree()
			network.Bootstrap()

			By("Starting o2 and o3")
			o2Runner = network.OrdererRunner(o2)
			o3Runner = network.OrdererRunner(o3)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Creating and joining the channel")
			channelID := "testchannel"
			channelparticipation.JoinOrderersAppChannelCluster(network, channelID, o2, o3)
			FindLeader([]*ginkgomon.Runner{o2Runner, o3Runner})

			By("Submitting several transactions to trigger snapshot")
			o2SnapDir := path.Join(network.RootDir, "orderers", o2.ID(), "etcdraft", "snapshot")

			env := CreateBroadcastEnvelope(network, o2, channelID, make([]byte, 2000))
			for i := 1; i <= 4; i++ { // 4 < MaxSnapshotFiles(5), so that no snapshot is pruned
				// Note that MaxMessageCount is 1 be default, so every tx results in a new block
				resp, err := ordererclient.Broadcast(network, o2, env)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Status).To(Equal(common.Status_SUCCESS))

				// Assert that new snapshot file is created before broadcasting next tx,
				// so that number of snapshots is deterministic. Otherwise, it is not
				// guaranteed that every block triggers a snapshot file being created,
				// due to the mechanism to prevent excessive snapshotting.
				Eventually(func() int {
					files, err := ioutil.ReadDir(path.Join(o2SnapDir, channelID))
					Expect(err).NotTo(HaveOccurred())
					return len(files)
				}, network.EventuallyTimeout).Should(Equal(i)) // snapshot interval is 1 KB, every block triggers snapshot
			}

			By("Killing orderers so they don't have blocks prior to latest snapshot in the memory")
			o2Proc.Signal(syscall.SIGKILL)
			Eventually(o2Proc.Wait(), network.EventuallyTimeout).Should(Receive())
			o3Proc.Signal(syscall.SIGKILL)
			Eventually(o3Proc.Wait(), network.EventuallyTimeout).Should(Receive())

			By("Starting lagged orderer and one of up-to-date orderers")
			o1Runner = network.OrdererRunner(o1)
			o2Runner = network.OrdererRunner(o2)
			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			channelparticipation.JoinOrderersAppChannelCluster(network, channelID, o1)

			o1SnapDir := path.Join(network.RootDir, "orderers", o1.ID(), "etcdraft", "snapshot")

			By("Asserting that orderer1 has snapshot dir for application channel")
			Eventually(func() int {
				files, err := ioutil.ReadDir(o1SnapDir)
				Expect(err).NotTo(HaveOccurred())
				return len(files)
			}, network.EventuallyTimeout).Should(Equal(1))

			By("Asserting that orderer1 receives and persists snapshot")
			Eventually(func() int {
				files, err := ioutil.ReadDir(path.Join(o1SnapDir, channelID))
				Expect(err).NotTo(HaveOccurred())
				return len(files)
			}, network.EventuallyTimeout).Should(Equal(1))

			By("Asserting cluster is still functional")
			env = CreateBroadcastEnvelope(network, o1, channelID, make([]byte, 1000))
			resp, err := ordererclient.Broadcast(network, o1, env)
			Expect(err).NotTo(HaveOccurred())
			Eventually(resp.Status, network.EventuallyTimeout).Should(Equal(common.Status_SUCCESS))

			for i := 1; i <= 5; i++ {
				blko1 := FetchBlock(network, o1, uint64(i), channelID)
				blko2 := FetchBlock(network, o2, uint64(i), channelID)

				Expect(blko1.Header.DataHash).To(Equal(blko2.Header.DataHash))
				metao1, err := protoutil.GetConsenterMetadataFromBlock(blko1)
				Expect(err).NotTo(HaveOccurred())
				metao2, err := protoutil.GetConsenterMetadataFromBlock(blko2)
				Expect(err).NotTo(HaveOccurred())

				bmo1 := &etcdraft.BlockMetadata{}
				proto.Unmarshal(metao1.Value, bmo1)
				bmo2 := &etcdraft.BlockMetadata{}
				proto.Unmarshal(metao2.Value, bmo2)

				Expect(bmo2).To(Equal(bmo1))
			}
		})

		It("catches up and replicates consenters metadata", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
			orderers := []*nwo.Orderer{network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")}
			peer = network.Peer("Org1", "peer0")

			network.GenerateConfigTree()
			network.Bootstrap()

			ordererRunners := []*ginkgomon.Runner{}
			orderersMembers := grouper.Members{}
			for _, o := range orderers {
				runner := network.OrdererRunner(o)
				ordererRunners = append(ordererRunners, runner)
				orderersMembers = append(orderersMembers, grouper.Member{
					Name:   o.ID(),
					Runner: runner,
				})
			}

			By("Starting ordering service cluster")
			ordererGroup := grouper.NewParallel(syscall.SIGTERM, orderersMembers)
			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", orderers...)

			By("Setting up new OSN to be added to the cluster")
			o4 := &nwo.Orderer{
				Name:         "orderer4",
				Organization: "OrdererOrg",
			}
			ports := nwo.Ports{}
			for _, portName := range nwo.OrdererPortNames() {
				ports[portName] = network.ReservePort()
			}

			network.PortsByOrdererID[o4.ID()] = ports
			network.Orderers = append(network.Orderers, o4)
			network.GenerateOrdererConfig(o4)
			extendNetwork(network)

			ordererCertificatePath := filepath.Join(network.OrdererLocalTLSDir(o4), "server.crt")
			ordererCert, err := ioutil.ReadFile(ordererCertificatePath)
			Expect(err).NotTo(HaveOccurred())

			By("Adding new ordering service node")
			addConsenter(network, peer, orderers[0], "testchannel", etcdraft.Consenter{
				ServerTlsCert: ordererCert,
				ClientTlsCert: ordererCert,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(o4, nwo.ClusterPort)),
			})

			By("Starting new ordering service node")
			r4 := network.OrdererRunner(o4)
			orderers = append(orderers, o4)
			ordererRunners = append(ordererRunners, r4)
			o4process := ifrit.Invoke(r4)
			Eventually(o4process.Ready(), network.EventuallyTimeout).Should(BeClosed())

			// Get the last config block of the channel
			By("Starting joining new ordering service node to the channel")
			configBlock := nwo.GetConfigBlock(network, peer, orderers[0], "testchannel")
			expectedChannelInfo := channelparticipation.ChannelInfo{
				Name:              "testchannel",
				URL:               "/participation/v1/channels/testchannel",
				Status:            "onboarding",
				ConsensusRelation: "consenter",
				Height:            0,
			}
			channelparticipation.Join(network, o4, "testchannel", configBlock, expectedChannelInfo)

			By("Pick ordering service node to be evicted")
			victimIdx := FindLeader(ordererRunners) - 1
			victim := orderers[victimIdx]
			victimCertBytes, err := ioutil.ReadFile(filepath.Join(network.OrdererLocalTLSDir(victim), "server.crt"))
			Expect(err).NotTo(HaveOccurred())

			assertBlockReception(map[string]int{
				"testchannel": 1,
			}, orderers, peer, network)

			By("Removing OSN from the channel")
			remainedOrderers := []*nwo.Orderer{}
			remainedRunners := []*ginkgomon.Runner{}

			for i, o := range orderers {
				if i == victimIdx {
					continue
				}
				remainedOrderers = append(remainedOrderers, o)
				remainedRunners = append(remainedRunners, ordererRunners[i])
			}

			removeConsenter(network, peer, remainedOrderers[0], "testchannel", victimCertBytes)

			By("Asserting all remaining nodes got last block")
			assertBlockReception(map[string]int{
				"testchannel": 2,
			}, remainedOrderers, peer, network)
			By("Making sure OSN was evicted and configuration applied")
			FindLeader(remainedRunners)

			By("Restarting all nodes")
			o4process.Signal(syscall.SIGTERM)
			Eventually(o4process.Wait(), network.EventuallyTimeout).Should(Receive())
			ordererProc.Signal(syscall.SIGTERM)
			Eventually(ordererProc.Wait(), network.EventuallyTimeout).Should(Receive())

			r1 := network.OrdererRunner(remainedOrderers[1])
			r2 := network.OrdererRunner(remainedOrderers[2])
			orderersMembers = grouper.Members{
				{Name: remainedOrderers[1].ID(), Runner: r1},
				{Name: remainedOrderers[2].ID(), Runner: r2},
			}

			ordererGroup = grouper.NewParallel(syscall.SIGTERM, orderersMembers)
			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			FindLeader([]*ginkgomon.Runner{r1, r2})

			By("Submitting several transactions to trigger snapshot")
			env := CreateBroadcastEnvelope(network, remainedOrderers[1], "testchannel", make([]byte, 2000))
			for i := 3; i <= 10; i++ {
				// Note that MaxMessageCount is 1 be default, so every tx results in a new block
				resp, err := ordererclient.Broadcast(network, remainedOrderers[1], env)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Status).To(Equal(common.Status_SUCCESS))
			}

			assertBlockReception(map[string]int{
				"testchannel": 10,
			}, []*nwo.Orderer{remainedOrderers[2]}, peer, network)

			By("Clean snapshot folder of lagging behind node")
			snapDir := path.Join(network.RootDir, "orderers", remainedOrderers[0].ID(), "etcdraft", "snapshot")
			snapshots, err := ioutil.ReadDir(snapDir)
			Expect(err).NotTo(HaveOccurred())

			for _, snap := range snapshots {
				os.RemoveAll(path.Join(snapDir, snap.Name()))
			}

			ordererProc.Signal(syscall.SIGTERM)
			Eventually(ordererProc.Wait(), network.EventuallyTimeout).Should(Receive())

			r0 := network.OrdererRunner(remainedOrderers[0])
			r1 = network.OrdererRunner(remainedOrderers[1])
			orderersMembers = grouper.Members{
				{Name: remainedOrderers[0].ID(), Runner: r0},
				{Name: remainedOrderers[1].ID(), Runner: r1},
			}

			ordererGroup = grouper.NewParallel(syscall.SIGTERM, orderersMembers)
			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			FindLeader([]*ginkgomon.Runner{r0, r1})

			By("Asserting that orderer1 receives and persists snapshot")
			Eventually(func() int {
				files, err := ioutil.ReadDir(path.Join(snapDir, "testchannel"))
				Expect(err).NotTo(HaveOccurred())
				return len(files)
			}, network.EventuallyTimeout).Should(BeNumerically(">", 0))

			assertBlockReception(map[string]int{
				"testchannel": 10,
			}, []*nwo.Orderer{remainedOrderers[0]}, peer, network)

			By("Make sure we can restart and connect to orderer1 with orderer4")
			ordererProc.Signal(syscall.SIGTERM)
			Eventually(ordererProc.Wait(), network.EventuallyTimeout).Should(Receive())

			r0 = network.OrdererRunner(remainedOrderers[0])
			r2 = network.OrdererRunner(remainedOrderers[2])
			orderersMembers = grouper.Members{
				{Name: remainedOrderers[0].ID(), Runner: r0},
				{Name: remainedOrderers[2].ID(), Runner: r2},
			}

			ordererGroup = grouper.NewParallel(syscall.SIGTERM, orderersMembers)
			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			FindLeader([]*ginkgomon.Runner{r0, r2})

			for i := 1; i <= 10; i++ {
				blko1 := FetchBlock(network, remainedOrderers[0], uint64(i), "testchannel")
				blko2 := FetchBlock(network, remainedOrderers[2], uint64(i), "testchannel")
				Expect(blko1.Header.DataHash).To(Equal(blko2.Header.DataHash))
				metao1, err := protoutil.GetConsenterMetadataFromBlock(blko1)
				Expect(err).NotTo(HaveOccurred())
				metao2, err := protoutil.GetConsenterMetadataFromBlock(blko2)
				Expect(err).NotTo(HaveOccurred())

				bmo1 := &etcdraft.BlockMetadata{}
				proto.Unmarshal(metao1.Value, bmo1)
				bmo2 := &etcdraft.BlockMetadata{}
				proto.Unmarshal(metao2.Value, bmo2)

				Expect(bmo2).To(Equal(bmo1))
			}
		})
	})

	When("The leader dies", func() {
		It("Elects a new leader", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaftNoSysChan(), testDir, client, StartPort(), components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")

			network.GenerateConfigTree()
			network.Bootstrap()

			By("Running the orderer nodes")
			o1Runner := network.OrdererRunner(o1)
			o2Runner := network.OrdererRunner(o2)
			o3Runner := network.OrdererRunner(o3)

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", o1, o2, o3)

			By("Waiting for them to elect a leader")
			ordererProcesses := []ifrit.Process{o1Proc, o2Proc, o3Proc}
			remainingAliveRunners := []*ginkgomon.Runner{o1Runner, o2Runner, o3Runner}
			leader := FindLeader(remainingAliveRunners)

			leaderIndex := leader - 1
			By(fmt.Sprintf("Killing the leader (%d)", leader))
			ordererProcesses[leaderIndex].Signal(syscall.SIGTERM)
			By("Waiting for it to die")
			Eventually(ordererProcesses[leaderIndex].Wait(), network.EventuallyTimeout).Should(Receive())

			// Remove the leader from the orderer runners
			remainingAliveRunners = append(remainingAliveRunners[:leaderIndex], remainingAliveRunners[leaderIndex+1:]...)

			By("Waiting for a new leader to be elected")
			leader = FindLeader(remainingAliveRunners)
			By(fmt.Sprintf("Orderer %d took over as a leader", leader))
		})
	})

	When("Leader cannot reach quorum", func() {
		It("Steps down", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaftNoSysChan(), testDir, client, StartPort(), components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			orderers := []*nwo.Orderer{o1, o2, o3}
			peer = network.Peer("Org1", "peer0")
			network.GenerateConfigTree()
			network.Bootstrap()

			By("Running the orderer nodes")
			o1Runner := network.OrdererRunner(o1)
			o2Runner := network.OrdererRunner(o2)
			o3Runner := network.OrdererRunner(o3)
			oRunners := []*ginkgomon.Runner{o1Runner, o2Runner, o3Runner}

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", o1, o2, o3)

			By("Waiting for them to elect a leader")
			ordererProcesses := []ifrit.Process{o1Proc, o2Proc, o3Proc}
			remainingAliveRunners := []*ginkgomon.Runner{o1Runner, o2Runner, o3Runner}
			leaderID := FindLeader(remainingAliveRunners)
			leaderIndex := leaderID - 1
			leader := orderers[leaderIndex]

			followerIndices := func() []int {
				var f []int
				for i := range ordererProcesses {
					if leaderIndex != i {
						f = append(f, i)
					}
				}

				return f
			}()

			By(fmt.Sprintf("Killing two followers (%d and %d)", followerIndices[0]+1, followerIndices[1]+1))
			ordererProcesses[followerIndices[0]].Signal(syscall.SIGTERM)
			ordererProcesses[followerIndices[1]].Signal(syscall.SIGTERM)

			By("Waiting for followers to die")
			// This significantly slows test (~10s). However, reducing ElectionTimeout
			// would introduce some flakes when disk write is exceptionally slow.
			Eventually(ordererProcesses[followerIndices[0]].Wait(), network.EventuallyTimeout).Should(Receive())
			Eventually(ordererProcesses[followerIndices[1]].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Waiting for leader to step down")
			Eventually(oRunners[leaderIndex].Err(), time.Minute, time.Second).Should(gbytes.Say(fmt.Sprintf("%d stepped down to follower since quorum is not active", leaderID)))

			By("Submitting tx to leader")
			// This should fail because current leader steps down
			// and there is no leader at this point of time
			env := CreateBroadcastEnvelope(network, leader, "testchannel", []byte("foo"))
			resp, err := ordererclient.Broadcast(network, leader, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SERVICE_UNAVAILABLE))
		})
	})

	When("orderer TLS certificates expire", func() {
		It("is still possible to recover", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaftNoSysChan(), testDir, client, StartPort(), components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			peer = network.Peer("Org1", "peer0")

			network.GenerateConfigTree()
			network.Bootstrap()

			ordererDomain := network.Organization(o1.Organization).Domain
			ordererTLSCAKeyPath := filepath.Join(network.RootDir, "crypto", "ordererOrganizations",
				ordererDomain, "tlsca", "priv_sk")

			ordererTLSCAKey, err := ioutil.ReadFile(ordererTLSCAKeyPath)
			Expect(err).NotTo(HaveOccurred())

			ordererTLSCACertPath := filepath.Join(network.RootDir, "crypto", "ordererOrganizations",
				ordererDomain, "tlsca", fmt.Sprintf("tlsca.%s-cert.pem", ordererDomain))
			ordererTLSCACert, err := ioutil.ReadFile(ordererTLSCACertPath)
			Expect(err).NotTo(HaveOccurred())

			serverTLSCerts := make(map[string][]byte)
			for _, orderer := range []*nwo.Orderer{o1, o2, o3} {
				tlsCertPath := filepath.Join(network.OrdererLocalTLSDir(orderer), "server.crt")
				serverTLSCerts[tlsCertPath], err = ioutil.ReadFile(tlsCertPath)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Expiring orderer TLS certificates")
			for filePath, certPEM := range serverTLSCerts {
				expiredCert, earlyMadeCACert := expireCertificate(certPEM, ordererTLSCACert, ordererTLSCAKey, time.Now())
				err = ioutil.WriteFile(filePath, expiredCert, 0o600)
				Expect(err).NotTo(HaveOccurred())

				err = ioutil.WriteFile(ordererTLSCACertPath, earlyMadeCACert, 0o600)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Regenerating config")
			sess, err := network.ConfigTxGen(commands.OutputBlock{
				ChannelID:   "testchannel",
				Profile:     network.ProfileForChannel("testchannel"),
				ConfigPath:  network.RootDir,
				OutputBlock: network.OutputBlockPath("testchannel"),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			By("Running the orderer nodes")
			o1Runner := network.OrdererRunner(o1)
			o2Runner := network.OrdererRunner(o2)
			o3Runner := network.OrdererRunner(o3)

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Cannot call the operations endpoint to join orderers with expired certs")
			appGenesisBlock := network.LoadAppChannelGenesisBlock("testchannel")
			channelparticipationJoinConnectFailure(network, o1, "testchannel", appGenesisBlock, "participation/v1/channels\": tls: failed to verify certificate: x509: certificate has expired or is not yet valid:")
			channelparticipationJoinConnectFailure(network, o2, "testchannel", appGenesisBlock, "participation/v1/channels\": tls: failed to verify certificate: x509: certificate has expired or is not yet valid:")
			channelparticipationJoinConnectFailure(network, o3, "testchannel", appGenesisBlock, "participation/v1/channels\": tls: failed to verify certificate: x509: certificate has expired or is not yet valid:")

			By("Killing orderers #1")
			o1Proc.Signal(syscall.SIGTERM)
			o2Proc.Signal(syscall.SIGTERM)
			o3Proc.Signal(syscall.SIGTERM)
			Eventually(o1Proc.Wait(), network.EventuallyTimeout).Should(Receive())
			Eventually(o2Proc.Wait(), network.EventuallyTimeout).Should(Receive())
			Eventually(o3Proc.Wait(), network.EventuallyTimeout).Should(Receive())

			o1Runner = network.OrdererRunner(o1)
			o2Runner = network.OrdererRunner(o2)
			o3Runner = network.OrdererRunner(o3)

			By("Launching orderers with Admin TLS disabled")
			orderers := []*nwo.Orderer{o1, o2, o3}
			for _, orderer := range orderers {
				ordererConfig := network.ReadOrdererConfig(orderer)
				ordererConfig.Admin.TLS.Enabled = false
				network.WriteOrdererConfig(orderer, ordererConfig)
			}

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Joining orderers to channel with Admin TLS disabled")
			// TODO add a test case that ensures the admin client can connect with a time-shift as well
			network.TLSEnabled = false
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", o1, o2, o3)
			network.TLSEnabled = true

			By("Waiting for TLS handshakes to fail")
			Eventually(o1Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("tls: bad certificate"))
			Eventually(o2Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("tls: bad certificate"))
			Eventually(o3Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("tls: bad certificate"))

			By("Killing orderers #2")
			o1Proc.Signal(syscall.SIGTERM)
			o2Proc.Signal(syscall.SIGTERM)
			o3Proc.Signal(syscall.SIGTERM)
			Eventually(o1Proc.Wait(), network.EventuallyTimeout).Should(Receive())
			Eventually(o2Proc.Wait(), network.EventuallyTimeout).Should(Receive())
			Eventually(o3Proc.Wait(), network.EventuallyTimeout).Should(Receive())

			o1Runner = network.OrdererRunner(o1)
			o2Runner = network.OrdererRunner(o2)
			o3Runner = network.OrdererRunner(o3)

			By("Launching orderers with a General.Cluster timeshift")
			for _, orderer := range orderers {
				ordererConfig := network.ReadOrdererConfig(orderer)
				ordererConfig.General.Cluster.TLSHandshakeTimeShift = 5 * time.Minute
				network.WriteOrdererConfig(orderer, ordererConfig)
			}

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Waiting for a leader to be elected")
			FindLeader([]*ginkgomon.Runner{o1Runner, o2Runner, o3Runner})

			By("Killing orderers #3")
			o1Proc.Signal(syscall.SIGTERM)
			o2Proc.Signal(syscall.SIGTERM)
			o3Proc.Signal(syscall.SIGTERM)
			Eventually(o1Proc.Wait(), network.EventuallyTimeout).Should(Receive())
			Eventually(o2Proc.Wait(), network.EventuallyTimeout).Should(Receive())
			Eventually(o3Proc.Wait(), network.EventuallyTimeout).Should(Receive())

			o1Runner = network.OrdererRunner(o1)
			o2Runner = network.OrdererRunner(o2)
			o3Runner = network.OrdererRunner(o3)

			By("Launching orderers again without a General.Cluster timeshift re-using the cluster port")
			for _, orderer := range orderers {
				ordererConfig := network.ReadOrdererConfig(orderer)
				ordererConfig.General.ListenPort = ordererConfig.General.Cluster.ListenPort
				ordererConfig.General.TLS.Certificate = ordererConfig.General.Cluster.ServerCertificate
				ordererConfig.General.TLS.PrivateKey = ordererConfig.General.Cluster.ServerPrivateKey
				ordererConfig.General.Cluster.TLSHandshakeTimeShift = 0
				ordererConfig.General.Cluster.ListenPort = 0
				ordererConfig.General.Cluster.ListenAddress = ""
				ordererConfig.General.Cluster.ServerCertificate = ""
				ordererConfig.General.Cluster.ServerPrivateKey = ""
				ordererConfig.General.Cluster.ClientCertificate = ""
				ordererConfig.General.Cluster.ClientPrivateKey = ""
				network.WriteOrdererConfig(orderer, ordererConfig)
			}

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Waiting for TLS handshakes to fail")
			Eventually(o1Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("tls: bad certificate"))
			Eventually(o2Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("tls: bad certificate"))
			Eventually(o3Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("tls: bad certificate"))

			By("Killing orderers #4")
			o1Proc.Signal(syscall.SIGTERM)
			o2Proc.Signal(syscall.SIGTERM)
			o3Proc.Signal(syscall.SIGTERM)
			Eventually(o1Proc.Wait(), network.EventuallyTimeout).Should(Receive())
			Eventually(o2Proc.Wait(), network.EventuallyTimeout).Should(Receive())
			Eventually(o3Proc.Wait(), network.EventuallyTimeout).Should(Receive())

			o1Runner = network.OrdererRunner(o1)
			o2Runner = network.OrdererRunner(o2)
			o3Runner = network.OrdererRunner(o3)

			By("Launching orderers again with a general timeshift re-using the cluster port")
			for _, orderer := range orderers {
				ordererConfig := network.ReadOrdererConfig(orderer)
				ordererConfig.General.TLS.TLSHandshakeTimeShift = 5 * time.Minute
				network.WriteOrdererConfig(orderer, ordererConfig)
			}

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Waiting for a leader to be elected")
			FindLeader([]*ginkgomon.Runner{o1Runner, o2Runner, o3Runner})

			By("submitting config updates to orderers with expired TLS certs to replace the expired certs")
			timeShift := 5 * time.Minute
			for _, o := range orderers {
				channelConfig := fetchConfig(network, peer, o, nwo.ClusterPort, "testchannel", timeShift)
				c := conftx.New(channelConfig)
				err = c.Orderer().RemoveConsenter(consenterChannelConfig(network, o))
				Expect(err).NotTo(HaveOccurred())

				By("renewing the orderer TLS certificates for " + o.Name)
				renewOrdererCertificates(network, o)
				err = c.Orderer().AddConsenter(consenterChannelConfig(network, o))
				Expect(err).NotTo(HaveOccurred())

				By("updating the config for " + o.Name)
				updateOrdererConfig(network, o, nwo.ClusterPort, "testchannel", timeShift, c.OriginalConfig(), c.UpdatedConfig(), peer)
			}

			By("Killing orderers #5")
			o1Proc.Signal(syscall.SIGTERM)
			o2Proc.Signal(syscall.SIGTERM)
			o3Proc.Signal(syscall.SIGTERM)
			Eventually(o1Proc.Wait(), network.EventuallyTimeout).Should(Receive())
			Eventually(o2Proc.Wait(), network.EventuallyTimeout).Should(Receive())
			Eventually(o3Proc.Wait(), network.EventuallyTimeout).Should(Receive())

			o1Runner = network.OrdererRunner(o1)
			o2Runner = network.OrdererRunner(o2)
			o3Runner = network.OrdererRunner(o3)

			By("Launching orderers again without a general timeshift")
			for _, o := range orderers {
				ordererConfig := network.ReadOrdererConfig(o)
				ordererConfig.General.TLS.TLSHandshakeTimeShift = 0
				network.WriteOrdererConfig(o, ordererConfig)
			}

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Waiting for a leader to be elected")
			FindLeader([]*ginkgomon.Runner{o1Runner, o2Runner, o3Runner})
		})

		It("disregards certificate renewal if only the validity period changed", func() {
			config := nwo.MultiNodeEtcdRaftNoSysChan()
			config.Channels = append(config.Channels, &nwo.Channel{Name: "foo", Profile: "TwoOrgsAppChannelEtcdRaft"})
			config.Channels = append(config.Channels, &nwo.Channel{Name: "bar", Profile: "TwoOrgsAppChannelEtcdRaft"})
			network = nwo.New(config, testDir, client, StartPort(), components)

			network.GenerateConfigTree()
			network.Bootstrap()

			peer = network.Peer("Org1", "peer0")

			o1 := network.Orderer("orderer1")
			o2 := network.Orderer("orderer2")
			o3 := network.Orderer("orderer3")

			orderers := []*nwo.Orderer{o1, o2, o3}

			o1Runner := network.OrdererRunner(o1)
			o2Runner := network.OrdererRunner(o2)
			o3Runner := network.OrdererRunner(o3)
			ordererRunners := []*ginkgomon.Runner{o1Runner, o2Runner, o3Runner}

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)
			ordererProcesses := []ifrit.Process{o1Proc, o2Proc, o3Proc}

			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", o1, o2, o3)
			By("Waiting for them to elect a leader")
			FindLeader(ordererRunners)

			By("Creating a channel")
			channelparticipation.JoinOrderersAppChannelCluster(network, "foo", o1, o2, o3)

			assertBlockReception(map[string]int{
				"foo":         0,
				"testchannel": 0,
			}, []*nwo.Orderer{o1, o2, o3}, peer, network)

			By("Killing all orderers")
			for i := range orderers {
				ordererProcesses[i].Signal(syscall.SIGTERM)
				Eventually(ordererProcesses[i].Wait(), network.EventuallyTimeout).Should(Receive())
			}

			By("Renewing the certificates for all orderers")
			renewOrdererCertificates(network, o1, o2, o3)

			By("Starting the orderers again")
			for i := range orderers {
				ordererRunner := network.OrdererRunner(orderers[i])
				ordererRunners[i] = ordererRunner
				ordererProcesses[i] = ifrit.Invoke(ordererRunner)
				Eventually(ordererProcesses[0].Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			o1Proc = ordererProcesses[0]
			o2Proc = ordererProcesses[1]
			o3Proc = ordererProcesses[2]

			By("Waiting for them to elect a leader once again")
			FindLeader(ordererRunners)

			By("Creating a channel again")
			channelparticipation.JoinOrderersAppChannelCluster(network, "bar", o1, o2, o3)

			assertBlockReception(map[string]int{
				"foo":         0,
				"bar":         0,
				"testchannel": 0,
			}, []*nwo.Orderer{o1, o2, o3}, peer, network)
		})
	})

	When("admin certificate expires", func() {
		It("is still possible to replace them", func() {
			network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			peer = network.Peer("Org1", "peer0")
			orderer := network.Orderer("orderer")

			ordererDomain := network.Organization(orderer.Organization).Domain
			ordererCAKeyPath := filepath.Join(network.RootDir, "crypto", "ordererOrganizations", ordererDomain, "ca", "priv_sk")

			ordererCAKey, err := ioutil.ReadFile(ordererCAKeyPath)
			Expect(err).NotTo(HaveOccurred())

			ordererCACertPath := network.OrdererCACert(orderer)
			ordererCACert, err := ioutil.ReadFile(ordererCACertPath)
			Expect(err).NotTo(HaveOccurred())

			adminCertPath := network.OrdererUserCert(orderer, "Admin")

			originalAdminCert, err := ioutil.ReadFile(adminCertPath)
			Expect(err).NotTo(HaveOccurred())

			expiredAdminCert, earlyCACert := expireCertificate(originalAdminCert, ordererCACert, ordererCAKey, time.Now())
			err = ioutil.WriteFile(adminCertPath, expiredAdminCert, 0o600)
			Expect(err).NotTo(HaveOccurred())

			adminPath := filepath.Join(network.RootDir, "crypto", "ordererOrganizations",
				ordererDomain, "msp", "admincerts", fmt.Sprintf("Admin@%s-cert.pem", ordererDomain))
			err = ioutil.WriteFile(adminPath, expiredAdminCert, 0o600)
			Expect(err).NotTo(HaveOccurred())

			err = ioutil.WriteFile(ordererCACertPath, earlyCACert, 0o600)
			Expect(err).NotTo(HaveOccurred())

			ordererCACertPath = network.OrdererCACert(orderer)
			err = ioutil.WriteFile(ordererCACertPath, earlyCACert, 0o600)
			Expect(err).NotTo(HaveOccurred())

			By("Regenerating config")
			sess, err := network.ConfigTxGen(commands.OutputBlock{
				ChannelID:   "testchannel",
				Profile:     network.ProfileForChannel("testchannel"),
				ConfigPath:  network.RootDir,
				OutputBlock: network.OutputBlockPath("testchannel"),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			runner := network.OrdererRunner(orderer)
			runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=debug")
			ordererProc = ifrit.Invoke(runner)

			By("Joining a channel and waiting for orderer to elect a leader")
			channelparticipation.JoinOrdererAppChannel(network, "testchannel", orderer, runner)

			By("Creating config update that adds another orderer admin")
			bootBlockPath := filepath.Join(network.RootDir, fmt.Sprintf("%s_block.pb", "testchannel"))
			bootBlock, err := ioutil.ReadFile(bootBlockPath)
			Expect(err).NotTo(HaveOccurred())

			current := configFromBootstrapBlock(bootBlock)
			updatedConfig := addAdminCertToConfig(current, originalAdminCert)

			tempDir, err := ioutil.TempDir("", "adminExpirationTest")
			Expect(err).NotTo(HaveOccurred())

			configBlockFile := filepath.Join(tempDir, "update.pb")
			defer os.RemoveAll(tempDir)
			nwo.ComputeUpdateOrdererConfig(configBlockFile, network, "testchannel", current, updatedConfig, peer)

			updateTransaction, err := ioutil.ReadFile(configBlockFile)
			Expect(err).NotTo(HaveOccurred())

			By("Creating config update")
			channelCreateTxn := createConfigTx(updateTransaction, "testchannel", network, orderer, peer)

			By("Updating channel config and failing")
			p, err := ordererclient.Broadcast(network, orderer, channelCreateTxn)
			Expect(err).NotTo(HaveOccurred())
			Expect(p.Status).To(Equal(common.Status_BAD_REQUEST))
			Expect(p.Info).To(ContainSubstring("broadcast client identity expired"))

			By("Attempting to fetch a block from orderer and failing")
			denv := CreateDeliverEnvelope(network, orderer, 0, "testchannel")
			Expect(denv).NotTo(BeNil())

			block, err := ordererclient.Deliver(network, orderer, denv)
			Expect(err).To(HaveOccurred())
			Expect(block).To(BeNil())
			Eventually(runner.Err(), time.Minute, time.Second).Should(gbytes.Say("deliver client identity expired"))

			By("Killing orderer")
			ordererProc.Signal(syscall.SIGTERM)
			Eventually(ordererProc.Wait(), network.EventuallyTimeout).Should(Receive())

			By("Launching orderers again")
			runner = network.OrdererRunner(orderer)
			runner.Command.Env = append(runner.Command.Env, "ORDERER_GENERAL_AUTHENTICATION_NOEXPIRATIONCHECKS=true")
			ordererProc = ifrit.Invoke(runner)

			By("Waiting for orderer to launch again")
			FindLeader([]*ginkgomon.Runner{runner})

			By("Updating channel config and succeeding")
			p, err = ordererclient.Broadcast(network, orderer, channelCreateTxn)
			Expect(err).NotTo(HaveOccurred())
			Expect(p.Status).To(Equal(common.Status_SUCCESS))

			By("Fetching a block from the orderer and succeeding")
			block = FetchBlock(network, orderer, 1, "testchannel")
			Expect(block).NotTo(BeNil())

			By("Restore the original admin cert")
			err = ioutil.WriteFile(adminCertPath, originalAdminCert, 0o600)
			Expect(err).NotTo(HaveOccurred())

			By("Ensure we can fetch the block using our original un-expired admin cert")
			ccb := func() uint64 {
				return nwo.GetConfigBlock(network, peer, orderer, "testchannel").Header.Number
			}
			Eventually(ccb, network.EventuallyTimeout).Should(Equal(uint64(1)))
		})
	})
})

func renewOrdererCertificates(network *nwo.Network, orderers ...*nwo.Orderer) {
	if len(orderers) == 0 {
		return
	}
	ordererDomain := network.Organization(orderers[0].Organization).Domain
	ordererTLSCAKeyPath := filepath.Join(network.RootDir, "crypto", "ordererOrganizations",
		ordererDomain, "tlsca", "priv_sk")

	ordererTLSCAKey, err := ioutil.ReadFile(ordererTLSCAKeyPath)
	Expect(err).NotTo(HaveOccurred())

	ordererTLSCACertPath := filepath.Join(network.RootDir, "crypto", "ordererOrganizations",
		ordererDomain, "tlsca", fmt.Sprintf("tlsca.%s-cert.pem", ordererDomain))
	ordererTLSCACert, err := ioutil.ReadFile(ordererTLSCACertPath)
	Expect(err).NotTo(HaveOccurred())

	serverTLSCerts := map[string][]byte{}
	for _, orderer := range orderers {
		tlsCertPath := filepath.Join(network.OrdererLocalTLSDir(orderer), "server.crt")
		serverTLSCerts[tlsCertPath], err = ioutil.ReadFile(tlsCertPath)
		Expect(err).NotTo(HaveOccurred())
	}

	for filePath, certPEM := range serverTLSCerts {
		renewedCert, _ := expireCertificate(certPEM, ordererTLSCACert, ordererTLSCAKey, time.Now().Add(time.Hour))
		err = ioutil.WriteFile(filePath, renewedCert, 0o600)
		Expect(err).NotTo(HaveOccurred())
	}
}

func expireCertificate(certPEM, caCertPEM, caKeyPEM []byte, expirationTime time.Time) (expiredcertPEM []byte, earlyMadeCACertPEM []byte) {
	keyAsDER, _ := pem.Decode(caKeyPEM)
	caKeyWithoutType, err := x509.ParsePKCS8PrivateKey(keyAsDER.Bytes)
	Expect(err).NotTo(HaveOccurred())
	caKey := caKeyWithoutType.(*ecdsa.PrivateKey)

	caCertAsDER, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caCertAsDER.Bytes)
	Expect(err).NotTo(HaveOccurred())

	certAsDER, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(certAsDER.Bytes)
	Expect(err).NotTo(HaveOccurred())

	cert.Raw = nil
	caCert.Raw = nil
	// The certificate was made 1 hour ago
	cert.NotBefore = time.Now().Add((-1) * time.Hour)
	// As well as the CA certificate
	caCert.NotBefore = time.Now().Add((-1) * time.Hour)
	// The certificate expires now
	cert.NotAfter = expirationTime

	// The CA signs the certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, cert.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())

	// The CA signs its own certificate
	caCertBytes, err := x509.CreateCertificate(rand.Reader, caCert, caCert, caCert.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())

	expiredcertPEM = pem.EncodeToMemory(&pem.Block{Bytes: certBytes, Type: "CERTIFICATE"})
	earlyMadeCACertPEM = pem.EncodeToMemory(&pem.Block{Bytes: caCertBytes, Type: "CERTIFICATE"})
	return
}

func createConfigTx(txData []byte, channelName string, network *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer) *common.Envelope {
	ctxEnv, err := protoutil.UnmarshalEnvelope(txData)
	Expect(err).NotTo(HaveOccurred())

	payload, err := protoutil.UnmarshalPayload(ctxEnv.Payload)
	Expect(err).NotTo(HaveOccurred())

	configUpdateEnv, err := configtx.UnmarshalConfigUpdateEnvelope(payload.Data)
	Expect(err).NotTo(HaveOccurred())

	signer := network.OrdererUserSigner(orderer, "Admin")
	signConfigUpdate(signer, configUpdateEnv)

	env, err := protoutil.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, channelName, signer, configUpdateEnv, 0, 0)
	Expect(err).NotTo(HaveOccurred())

	return env
}

func signConfigUpdate(signer *nwo.SigningIdentity, configUpdateEnv *common.ConfigUpdateEnvelope) *common.ConfigUpdateEnvelope {
	sigHeader, err := protoutil.NewSignatureHeader(signer)
	Expect(err).NotTo(HaveOccurred())

	configSig := &common.ConfigSignature{
		SignatureHeader: protoutil.MarshalOrPanic(sigHeader),
	}

	configSig.Signature, err = signer.Sign(util.ConcatenateBytes(configSig.SignatureHeader, configUpdateEnv.ConfigUpdate))
	Expect(err).NotTo(HaveOccurred())

	configUpdateEnv.Signatures = append(configUpdateEnv.Signatures, configSig)
	return configUpdateEnv
}

func addAdminCertToConfig(originalConfig *common.Config, additionalAdmin []byte) *common.Config {
	updatedConfig := proto.Clone(originalConfig).(*common.Config)

	rawMSPConfig := updatedConfig.ChannelGroup.Groups["Orderer"].Groups["OrdererOrg"].Values["MSP"]
	mspConfig := &msp.MSPConfig{}
	err := proto.Unmarshal(rawMSPConfig.Value, mspConfig)
	Expect(err).NotTo(HaveOccurred())

	fabricConfig := &msp.FabricMSPConfig{}
	err = proto.Unmarshal(mspConfig.Config, fabricConfig)
	Expect(err).NotTo(HaveOccurred())

	fabricConfig.Admins = append(fabricConfig.Admins, additionalAdmin)
	mspConfig.Config = protoutil.MarshalOrPanic(fabricConfig)

	rawMSPConfig.Value = protoutil.MarshalOrPanic(mspConfig)
	return updatedConfig
}

func configFromBootstrapBlock(bootstrapBlock []byte) *common.Config {
	block := &common.Block{}
	err := proto.Unmarshal(bootstrapBlock, block)
	Expect(err).NotTo(HaveOccurred())
	return configFromBlock(block)
}

func configFromBlock(block *common.Block) *common.Config {
	envelope, err := protoutil.GetEnvelopeFromBlock(block.Data.Data[0])
	Expect(err).NotTo(HaveOccurred())

	payload, err := protoutil.UnmarshalPayload(envelope.Payload)
	Expect(err).NotTo(HaveOccurred())

	configEnv := &common.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnv)
	Expect(err).NotTo(HaveOccurred())

	return configEnv.Config
}

func fetchConfig(n *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, port nwo.PortName, channel string, tlsHandshakeTimeShift time.Duration) *common.Config {
	tempDir, err := ioutil.TempDir(n.RootDir, "fetchConfig")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	output := filepath.Join(tempDir, "config_block.pb")
	fetchConfigBlock(n, peer, orderer, port, channel, output, tlsHandshakeTimeShift)
	configBlock := nwo.UnmarshalBlockFromFile(output)
	return configFromBlock(configBlock)
}

func fetchConfigBlock(n *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, port nwo.PortName, channel, output string, tlsHandshakeTimeShift time.Duration) {
	fetch := func() int {
		sess, err := n.OrdererAdminSession(orderer, peer, commands.ChannelFetch{
			ChannelID:             channel,
			Block:                 "config",
			Orderer:               n.OrdererAddress(orderer, port),
			OutputFile:            output,
			ClientAuth:            n.ClientAuthRequired,
			TLSHandshakeTimeShift: tlsHandshakeTimeShift,
		})
		Expect(err).NotTo(HaveOccurred())
		code := sess.Wait(n.EventuallyTimeout).ExitCode()
		if code == 0 {
			Expect(sess.Err).To(gbytes.Say("Received block: "))
		}
		return code
	}
	Eventually(fetch, n.EventuallyTimeout).Should(Equal(0))
}

func currentConfigBlockNumber(n *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, port nwo.PortName, channel string, tlsHandshakeTimeShift time.Duration) uint64 {
	tempDir, err := ioutil.TempDir(n.RootDir, "currentConfigBlock")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	output := filepath.Join(tempDir, "config_block.pb")
	fetchConfigBlock(n, peer, orderer, port, channel, output, tlsHandshakeTimeShift)
	configBlock := nwo.UnmarshalBlockFromFile(output)
	return configBlock.Header.Number
}

func updateOrdererConfig(n *nwo.Network, orderer *nwo.Orderer, port nwo.PortName, channel string, tlsHandshakeTimeShift time.Duration, current, updated *common.Config, submitter *nwo.Peer, additionalSigners ...*nwo.Orderer) {
	tempDir, err := ioutil.TempDir(n.RootDir, "updateConfig")
	Expect(err).NotTo(HaveOccurred())
	updateFile := filepath.Join(tempDir, "update.pb")
	defer os.RemoveAll(tempDir)

	currentBlockNumber := currentConfigBlockNumber(n, submitter, orderer, port, channel, tlsHandshakeTimeShift)
	nwo.ComputeUpdateOrdererConfig(updateFile, n, channel, current, updated, submitter, additionalSigners...)

	Eventually(func() bool {
		sess, err := n.OrdererAdminSession(orderer, submitter, commands.ChannelUpdate{
			ChannelID:             channel,
			Orderer:               n.OrdererAddress(orderer, port),
			File:                  updateFile,
			ClientAuth:            n.ClientAuthRequired,
			TLSHandshakeTimeShift: tlsHandshakeTimeShift,
		})
		Expect(err).NotTo(HaveOccurred())

		sess.Wait(n.EventuallyTimeout)
		if sess.ExitCode() != 0 {
			return false
		}

		return strings.Contains(string(sess.Err.Contents()), "Successfully submitted channel update")
	}, n.EventuallyTimeout).Should(BeTrue())

	ccb := func() uint64 {
		return currentConfigBlockNumber(n, submitter, orderer, port, channel, tlsHandshakeTimeShift)
	}
	Eventually(ccb, n.EventuallyTimeout).Should(BeNumerically(">", currentBlockNumber))
}
