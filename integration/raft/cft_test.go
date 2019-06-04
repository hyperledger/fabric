/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package raft

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"sync"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/protos/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
	"github.com/tedsuo/ifrit/grouper"
)

var _ = Describe("EndToEnd Crash Fault Tolerance", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network
		peer    *nwo.Peer

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
		for _, oProc := range []ifrit.Process{o1Proc, o2Proc, o3Proc} {
			if oProc != nil {
				oProc.Signal(syscall.SIGTERM)
				Eventually(oProc.Wait(), network.EventuallyTimeout).Should(Receive())
			}
		}

		if ordererProc != nil {
			ordererProc.Signal(syscall.SIGTERM)
			Eventually(ordererProc.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	When("orderer stops and restarts", func() {
		It("keeps network up and running", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, StartPort(), components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			peer = network.Peer("Org1", "peer1")

			network.GenerateConfigTree()
			network.Bootstrap()

			o1Runner := network.OrdererRunner(o1)
			orderers := grouper.Members{
				{Name: o2.ID(), Runner: network.OrdererRunner(o2)},
				{Name: o3.ID(), Runner: network.OrdererRunner(o3)},
			}
			ordererGroup := grouper.NewParallel(syscall.SIGTERM, orderers)

			o1Proc = ifrit.Invoke(o1Runner)
			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(o1Proc.Ready()).Should(BeClosed())
			Eventually(ordererProc.Ready()).Should(BeClosed())

			findLeader([]*ginkgomon.Runner{o1Runner})

			By("performing operation with orderer1")
			env := CreateBroadcastEnvelope(network, o1, network.SystemChannel.Name, []byte("foo"))
			resp, err := Broadcast(network, o1, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			block := FetchBlock(network, o1, 1, network.SystemChannel.Name)
			Expect(block).NotTo(BeNil())

			By("killing orderer1")
			o1Proc.Signal(syscall.SIGKILL)
			Eventually(o1Proc.Wait(), network.EventuallyTimeout).Should(Receive(MatchError("exit status 137")))

			By("broadcasting envelope to running orderer")
			resp, err = Broadcast(network, o2, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			block = FetchBlock(network, o2, 2, network.SystemChannel.Name)
			Expect(block).NotTo(BeNil())

			By("restarting orderer1")
			o1Runner = network.OrdererRunner(o1)
			o1Proc = ifrit.Invoke(o1Runner)
			Eventually(o1Proc.Ready()).Should(BeClosed())
			findLeader([]*ginkgomon.Runner{o1Runner})

			By("broadcasting envelope to restarted orderer")
			resp, err = Broadcast(network, o1, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			blko1 := FetchBlock(network, o1, 3, network.SystemChannel.Name)
			blko2 := FetchBlock(network, o2, 3, network.SystemChannel.Name)

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
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, StartPort(), components)
			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			peer = network.Peer("Org1", "peer1")

			network.GenerateConfigTree()
			network.Bootstrap()

			orderers := grouper.Members{
				{Name: o2.ID(), Runner: network.OrdererRunner(o2)},
				{Name: o3.ID(), Runner: network.OrdererRunner(o3)},
			}
			ordererGroup := grouper.NewParallel(syscall.SIGTERM, orderers)

			By("Starting 2/3 of cluster")
			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(ordererProc.Ready()).Should(BeClosed())

			By("Creating testchannel")
			channelID := "testchannel"
			network.CreateChannel(channelID, o2, peer)

			By("Submitting several transactions to trigger snapshot")
			o2SnapDir := path.Join(network.RootDir, "orderers", o2.ID(), "etcdraft", "snapshot")

			env := CreateBroadcastEnvelope(network, o2, channelID, make([]byte, 2000))
			for i := 1; i <= 4; i++ { // 4 < MaxSnapshotFiles(5), so that no snapshot is pruned
				// Note that MaxMessageCount is 1 be default, so every tx results in a new block
				resp, err := Broadcast(network, o2, env)
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
			ordererProc.Signal(syscall.SIGKILL)
			Eventually(ordererProc.Wait(), network.EventuallyTimeout).Should(Receive())

			By("Starting lagged orderer and one of up-to-date orderers")
			orderers = grouper.Members{
				{Name: o1.ID(), Runner: network.OrdererRunner(o1)},
				{Name: o2.ID(), Runner: network.OrdererRunner(o2)},
			}
			ordererGroup = grouper.NewParallel(syscall.SIGTERM, orderers)
			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(ordererProc.Ready()).Should(BeClosed())

			o1SnapDir := path.Join(network.RootDir, "orderers", o1.ID(), "etcdraft", "snapshot")

			By("Asserting that orderer1 has snapshot dir for both system and application channel")
			Eventually(func() int {
				files, err := ioutil.ReadDir(o1SnapDir)
				Expect(err).NotTo(HaveOccurred())
				return len(files)
			}, network.EventuallyTimeout).Should(Equal(2))

			By("Asserting that orderer1 receives and persists snapshot")
			Eventually(func() int {
				files, err := ioutil.ReadDir(path.Join(o1SnapDir, channelID))
				Expect(err).NotTo(HaveOccurred())
				return len(files)
			}, network.EventuallyTimeout).Should(Equal(1))

			By("Asserting cluster is still functional")
			env = CreateBroadcastEnvelope(network, o1, channelID, make([]byte, 1000))
			resp, err := Broadcast(network, o1, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			blko1 := FetchBlock(network, o1, 5, channelID)
			blko2 := FetchBlock(network, o2, 5, channelID)

			Expect(blko1.Header.DataHash).To(Equal(blko2.Header.DataHash))
		})
	})

	When("The leader dies", func() {
		It("Elects a new leader", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, StartPort(), components)

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

			Eventually(o1Proc.Ready()).Should(BeClosed())
			Eventually(o2Proc.Ready()).Should(BeClosed())
			Eventually(o3Proc.Ready()).Should(BeClosed())

			By("Waiting for them to elect a leader")
			ordererProcesses := []ifrit.Process{o1Proc, o2Proc, o3Proc}
			remainingAliveRunners := []*ginkgomon.Runner{o1Runner, o2Runner, o3Runner}
			leader := findLeader(remainingAliveRunners)

			leaderIndex := leader - 1
			By(fmt.Sprintf("Killing the leader (%d)", leader))
			ordererProcesses[leaderIndex].Signal(syscall.SIGTERM)
			By("Waiting for it to die")
			Eventually(ordererProcesses[leaderIndex].Wait(), network.EventuallyTimeout).Should(Receive())

			// Remove the leader from the orderer runners
			remainingAliveRunners = append(remainingAliveRunners[:leaderIndex], remainingAliveRunners[leaderIndex+1:]...)

			By("Waiting for a new leader to be elected")
			leader = findLeader(remainingAliveRunners)
			By(fmt.Sprintf("Orderer %d took over as a leader", leader))
		})
	})

	When("Leader cannot reach quorum", func() {
		It("Steps down", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, StartPort(), components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			orderers := []*nwo.Orderer{o1, o2, o3}
			peer = network.Peer("Org1", "peer1")
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

			Eventually(o1Proc.Ready()).Should(BeClosed())
			Eventually(o2Proc.Ready()).Should(BeClosed())
			Eventually(o3Proc.Ready()).Should(BeClosed())

			By("Waiting for them to elect a leader")
			ordererProcesses := []ifrit.Process{o1Proc, o2Proc, o3Proc}
			remainingAliveRunners := []*ginkgomon.Runner{o1Runner, o2Runner, o3Runner}
			leaderID := findLeader(remainingAliveRunners)
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
			env := CreateBroadcastEnvelope(network, leader, network.SystemChannel.Name, []byte("foo"))
			resp, err := Broadcast(network, leader, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SERVICE_UNAVAILABLE))
		})
	})
})

func findLeader(ordererRunners []*ginkgomon.Runner) int {
	var wg sync.WaitGroup
	wg.Add(len(ordererRunners))

	findLeader := func(runner *ginkgomon.Runner) int {
		Eventually(runner.Err(), time.Minute, time.Second).Should(gbytes.Say("Raft leader changed: [0-9] -> "))

		idBuff := make([]byte, 1)
		runner.Err().Read(idBuff)

		newLeader, err := strconv.ParseInt(string(idBuff), 10, 32)
		Expect(err).To(BeNil())
		return int(newLeader)
	}

	leaders := make(chan int, len(ordererRunners))

	for _, runner := range ordererRunners {
		go func(runner *ginkgomon.Runner) {
			defer GinkgoRecover()
			defer wg.Done()

			for {
				leader := findLeader(runner)
				if leader != 0 {
					leaders <- leader
					break
				}
			}
		}(runner)
	}

	wg.Wait()

	close(leaders)
	firstLeader := <-leaders
	for leader := range leaders {
		if firstLeader != leader {
			Fail(fmt.Sprintf("First leader is %d but saw %d also as a leader", firstLeader, leader))
		}
	}

	return firstLeader
}
