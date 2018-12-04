/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/common"
	protosorderer "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("EndToEnd", func() {
	var (
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		chaincode nwo.Chaincode
		process   ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		chaincode = nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `AND ('Org1MSP.member','Org2MSP.member')`,
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

	Describe("basic solo network with 2 orgs", func() {
		var datagramReader *DatagramReader

		BeforeEach(func() {
			datagramReader = NewDatagramReader()
			go datagramReader.Start()

			network = nwo.New(nwo.BasicSolo(), testDir, client, BasePort(), components)
			network.MetricsProvider = "statsd"
			network.StatsdEndpoint = datagramReader.Address()

			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		AfterEach(func() {
			if datagramReader != nil {
				datagramReader.Close()
			}
		})

		It("executes a basic solo network with 2 orgs", func() {
			By("getting the orderer by name")
			orderer := network.Orderer("orderer")

			By("setting up the channel")
			network.CreateAndJoinChannel(orderer, "testchannel")

			By("deploying the chaincode")
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			By("getting the client peer by name")
			peer := network.Peer("Org1", "peer1")

			RunQueryInvokeQuery(network, orderer, peer, "testchannel")
			RunRespondWith(network, orderer, peer, "testchannel")

			By("waiting for DeliverFiltered stats to be emitted")
			metricsWriteInterval := 5 * time.Second
			Eventually(datagramReader, 2*metricsWriteInterval).Should(gbytes.Say("stream_request_duration.protos_Deliver.DeliverFiltered."))

			CheckPeerStatsdStreamMetrics(datagramReader.String())
			CheckPeerStatsdMetrics(datagramReader.String(), "org1_peer0")
			CheckPeerStatsdMetrics(datagramReader.String(), "org2_peer1")
			CheckOrdererStatsdMetrics(datagramReader.String(), "ordererorg_orderer")
		})
	})

	Describe("basic kafka network with 2 orgs", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.BasicKafka(), testDir, client, BasePort(), components)
			network.MetricsProvider = "prometheus"
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		It("executes a basic kafka network with 2 orgs", func() {
			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			network.CreateAndJoinChannel(orderer, "testchannel")
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
			RunQueryInvokeQuery(network, orderer, peer, "testchannel")

			CheckPeerOperationEndpoints(network, network.Peer("Org2", "peer1"))
			CheckOrdererOperationEndpoints(network, orderer)
		})
	})

	PDescribe("basic single node etcdraft network with 2 orgs", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.BasicEtcdRaft(), testDir, client, BasePort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		It("executes a basic etcdraft network with 2 orgs and a single node", func() {
			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			network.CreateAndJoinChannel(orderer, "testchannel")
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
			RunQueryInvokeQuery(network, orderer, peer, "testchannel")
		})
	})

	PDescribe("three node etcdraft network with 2 orgs", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, BasePort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		// This tests:
		//
		// 1. channel creation with raft orderer,
		// 2. all the nodes on three-node raft cluster are in sync wrt blocks,
		// 3. raft orderer processes type A config updates and delivers the
		//    config blocks to the peers.
		It("executes an etcdraft network with 2 orgs and three orderer nodes", func() {
			orderer1 := network.Orderer("orderer1")
			orderer2 := network.Orderer("orderer2")
			orderer3 := network.Orderer("orderer3")
			peer := network.Peer("Org1", "peer1")
			org1Peer0 := network.Peer("Org1", "peer0")
			blockFile1 := filepath.Join(testDir, "newest_orderer1_block.pb")
			blockFile2 := filepath.Join(testDir, "newest_orderer2_block.pb")
			blockFile3 := filepath.Join(testDir, "newest_orderer3_block.pb")

			fetchLatestBlock := func(targetOrderer *nwo.Orderer, blockFile string) {
				c := commands.ChannelFetch{
					ChannelID:  "testchannel",
					Block:      "newest",
					OutputFile: blockFile,
				}
				if targetOrderer != nil {
					c.Orderer = network.OrdererAddress(targetOrderer, nwo.ListenPort)
				}
				sess, err := network.PeerAdminSession(org1Peer0, c)
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			}

			By("creating a new chain and having the peers join it to test for channel creation")
			network.CreateAndJoinChannel(orderer1, "testchannel")
			nwo.DeployChaincode(network, "testchannel", orderer1, chaincode)
			RunQueryInvokeQuery(network, orderer1, peer, "testchannel")

			// the above can work even if the orderer nodes are not in the same Raft
			// cluster; we need to verify all the three orderer nodes are in sync wrt
			// blocks.
			By("fetching the latest blocks from all the orderer nodes and testing them for equality")
			fetchLatestBlock(orderer1, blockFile1)
			fetchLatestBlock(orderer2, blockFile2)
			fetchLatestBlock(orderer3, blockFile3)
			b1 := nwo.UnmarshalBlockFromFile(blockFile1)
			b2 := nwo.UnmarshalBlockFromFile(blockFile2)
			b3 := nwo.UnmarshalBlockFromFile(blockFile3)
			Expect(bytes.Equal(b1.Header.Bytes(), b2.Header.Bytes())).To(BeTrue())
			Expect(bytes.Equal(b2.Header.Bytes(), b3.Header.Bytes())).To(BeTrue())

			By("updating ACL policies to test for type A configuration updates")
			invokeChaincode := commands.ChaincodeInvoke{
				ChannelID:    "testchannel",
				Orderer:      network.OrdererAddress(orderer1, nwo.ListenPort),
				Name:         chaincode.Name,
				Ctor:         `{"Args":["invoke","a","b","10"]}`,
				WaitForEvent: true,
			}
			// setting the filtered block event ACL policy to org2/Admins
			policyName := resources.Event_FilteredBlock
			policy := "/Channel/Application/org2/Admins"
			SetACLPolicy(network, "testchannel", policyName, policy, "orderer1")
			// invoking chaincode as a forbidden Org1 Admin identity
			sess, err := network.PeerAdminSession(org1Peer0, invokeChaincode)
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess.Err, network.EventuallyTimeout).Should(gbytes.Say(`\Qdeliver completed with status (FORBIDDEN)\E`))
		})
	})

	PDescribe("etcd raft, checking valid configuration update of type B", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.BasicEtcdRaft(), testDir, client, BasePort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		It("executes a basic etcdraft network with a single Raft node", func() {
			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			channel := "testchannel"
			network.CreateAndJoinChannel(orderer, channel)
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
			RunQueryInvokeQuery(network, orderer, peer, "testchannel")

			config := nwo.GetConfigBlock(network, peer, orderer, channel)
			updatedConfig := proto.Clone(config).(*common.Config)

			consensusTypeConfigValue := updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
			consensusTypeValue := &protosorderer.ConsensusType{}
			err := proto.Unmarshal(consensusTypeConfigValue.Value, consensusTypeValue)
			Expect(err).NotTo(HaveOccurred())

			metadata := &etcdraft.Metadata{}
			err = proto.Unmarshal(consensusTypeValue.Metadata, metadata)
			Expect(err).NotTo(HaveOccurred())

			// update max in flight messages
			metadata.Options.MaxInflightMsgs = 1000
			metadata.Options.MaxSizePerMsg = 512

			// write metadata back
			consensusTypeValue.Metadata, err = proto.Marshal(metadata)
			Expect(err).NotTo(HaveOccurred())

			updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"] = &common.ConfigValue{
				ModPolicy: "Admins",
				Value:     utils.MarshalOrPanic(consensusTypeValue),
			}

			nwo.UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
		})
	})

	PDescribe("basic single node etcdraft network with 2 orgs and 2 channels", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.MultiChannelEtcdRaft(), testDir, client, BasePort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		It("executes a basic etcdraft network with 2 orgs and 2 channels", func() {
			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			network.CreateAndJoinChannel(orderer, "testchannel1")
			nwo.DeployChaincode(network, "testchannel1", orderer, chaincode)

			network.CreateAndJoinChannel(orderer, "testchannel2")
			nwo.InstantiateChaincode(network, "testchannel2", orderer, chaincode, peer)

			RunQueryInvokeQuery(network, orderer, peer, "testchannel1")
			RunQueryInvokeQuery(network, orderer, peer, "testchannel2")
		})
	})
})

func RunQueryInvokeQuery(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string) {
	By("querying the chaincode")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("100"))

	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
			n.PeerAddress(n.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("90"))
}

func RunRespondWith(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string) {
	By("responding with a 300")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["respond","300","response-message","response-payload"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:300"))

	By("responding with a 400")
	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["respond","400","response-message","response-payload"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(1))
	Expect(sess.Err).To(gbytes.Say(`Error: endorsement failure during invoke.`))
}

func CheckPeerStatsdMetrics(contents, prefix string) {
	By("checking for peer statsd metrics")
	Expect(contents).To(ContainSubstring(prefix + ".go.mem.gc_completed_count:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.unary_requests_received.protos_Endorser.ProcessProposal:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.unary_requests_completed.protos_Endorser.ProcessProposal.OK:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.unary_request_duration.protos_Endorser.ProcessProposal.OK:"))
}

func CheckPeerStatsdStreamMetrics(contents string) {
	By("checking for stream metrics")
	Expect(contents).To(ContainSubstring(".grpc.server.stream_requests_received.protos_Deliver.DeliverFiltered:"))
	Expect(contents).To(ContainSubstring(".grpc.server.stream_requests_completed.protos_Deliver.DeliverFiltered.Unknown:"))
	Expect(contents).To(ContainSubstring(".grpc.server.stream_request_duration.protos_Deliver.DeliverFiltered.Unknown:"))
	Expect(contents).To(ContainSubstring(".grpc.server.stream_messages_received.protos_Deliver.DeliverFiltered"))
	Expect(contents).To(ContainSubstring(".grpc.server.stream_messages_sent.protos_Deliver.DeliverFiltered"))
}

func CheckOrdererStatsdMetrics(contents, prefix string) {
	By("checking for AtomicBroadcast")
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_request_duration.orderer_AtomicBroadcast.Broadcast.OK"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_request_duration.orderer_AtomicBroadcast.Deliver."))

	By("checking for orderer metrics")
	Expect(contents).To(ContainSubstring(prefix + ".go.mem.gc_completed_count:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_requests_received.orderer_AtomicBroadcast.Deliver:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_requests_completed.orderer_AtomicBroadcast.Deliver."))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_messages_received.orderer_AtomicBroadcast.Deliver"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_messages_sent.orderer_AtomicBroadcast.Deliver"))
}

func OrdererOperationalClients(network *nwo.Network, orderer *nwo.Orderer) (authClient, unauthClient *http.Client) {
	return operationalClients(network.OrdererLocalTLSDir(orderer))
}

func PeerOperationalClients(network *nwo.Network, peer *nwo.Peer) (authClient, unauthClient *http.Client) {
	return operationalClients(network.PeerLocalTLSDir(peer))
}

func operationalClients(tlsDir string) (authClient, unauthClient *http.Client) {
	clientCert, err := tls.LoadX509KeyPair(
		filepath.Join(tlsDir, "server.crt"),
		filepath.Join(tlsDir, "server.key"),
	)
	Expect(err).NotTo(HaveOccurred())

	clientCertPool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile(filepath.Join(tlsDir, "ca.crt"))
	Expect(err).NotTo(HaveOccurred())
	clientCertPool.AppendCertsFromPEM(caCert)

	authenticatedClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{clientCert},
				RootCAs:      clientCertPool,
			},
		},
	}
	unauthenticatedClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: clientCertPool},
		},
	}

	return authenticatedClient, unauthenticatedClient
}

func CheckPeerOperationEndpoints(network *nwo.Network, peer *nwo.Peer) {
	metricsURL := fmt.Sprintf("https://127.0.0.1:%d/metrics", network.PeerPort(peer, nwo.OperationsPort))
	logspecURL := fmt.Sprintf("https://127.0.0.1:%d/logspec", network.PeerPort(peer, nwo.OperationsPort))
	healthURL := fmt.Sprintf("https://127.0.0.1:%d/healthz", network.PeerPort(peer, nwo.OperationsPort))

	authClient, unauthClient := PeerOperationalClients(network, peer)

	CheckPeerPrometheusMetrics(authClient, metricsURL)
	CheckLogspecOperations(authClient, logspecURL)
	CheckHealthEndpoint(authClient, healthURL)

	By("getting the logspec without a client cert")
	resp, err := unauthClient.Get(logspecURL)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

	By("ensuring health checks do not require a client cert")
	CheckHealthEndpoint(unauthClient, healthURL)
}

func CheckOrdererOperationEndpoints(network *nwo.Network, orderer *nwo.Orderer) {
	metricsURL := fmt.Sprintf("https://127.0.0.1:%d/metrics", network.OrdererPort(orderer, nwo.OperationsPort))
	logspecURL := fmt.Sprintf("https://127.0.0.1:%d/logspec", network.OrdererPort(orderer, nwo.OperationsPort))
	healthURL := fmt.Sprintf("https://127.0.0.1:%d/healthz", network.OrdererPort(orderer, nwo.OperationsPort))

	authClient, unauthClient := OrdererOperationalClients(network, orderer)

	CheckOrdererPrometheusMetrics(authClient, metricsURL)
	CheckLogspecOperations(authClient, logspecURL)
	CheckHealthEndpoint(authClient, healthURL)

	By("getting the logspec without a client cert")
	resp, err := unauthClient.Get(logspecURL)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

	By("ensuring health checks do not require a client cert")
	CheckHealthEndpoint(unauthClient, healthURL)
}

func CheckPeerPrometheusMetrics(client *http.Client, url string) {
	By("hitting the prometheus metrics endpoint")
	resp, err := client.Get(url)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	resp.Body.Close()

	Eventually(getBody(client, url)).Should(ContainSubstring(`# TYPE grpc_server_stream_request_duration histogram`))

	By("checking for some expected metrics")
	body := getBody(client, url)()
	Expect(body).To(ContainSubstring(`# TYPE go_gc_duration_seconds summary`))
	Expect(body).To(ContainSubstring(`# TYPE grpc_server_stream_request_duration histogram`))
	Expect(body).To(ContainSubstring(`grpc_server_stream_request_duration_count{code="Unknown",method="DeliverFiltered",service="protos_Deliver"}`))
	Expect(body).To(ContainSubstring(`grpc_server_stream_messages_received{method="DeliverFiltered",service="protos_Deliver"}`))
	Expect(body).To(ContainSubstring(`grpc_server_stream_messages_sent{method="DeliverFiltered",service="protos_Deliver"}`))
	Expect(body).To(ContainSubstring(`# TYPE grpc_comm_conn_closed counter`))
	Expect(body).To(ContainSubstring(`# TYPE grpc_comm_conn_opened counter`))
}

func CheckOrdererPrometheusMetrics(client *http.Client, url string) {
	By("hitting the prometheus metrics endpoint")
	resp, err := client.Get(url)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	resp.Body.Close()

	Eventually(getBody(client, url)).Should(ContainSubstring(`# TYPE grpc_server_stream_request_duration histogram`))

	By("checking for some expected metrics")
	body := getBody(client, url)()
	Expect(body).To(ContainSubstring(`# TYPE go_gc_duration_seconds summary`))
	Expect(body).To(ContainSubstring(`# TYPE grpc_server_stream_request_duration histogram`))
	Expect(body).To(ContainSubstring(`grpc_server_stream_request_duration_sum{code="OK",method="Deliver",service="orderer_AtomicBroadcast"`))
	Expect(body).To(ContainSubstring(`grpc_server_stream_request_duration_sum{code="OK",method="Broadcast",service="orderer_AtomicBroadcast"`))
	Expect(body).To(ContainSubstring(`# TYPE grpc_comm_conn_closed counter`))
	Expect(body).To(ContainSubstring(`# TYPE grpc_comm_conn_opened counter`))
}

func CheckLogspecOperations(client *http.Client, logspecURL string) {
	By("getting the logspec")
	resp, err := client.Get(logspecURL)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	Expect(err).NotTo(HaveOccurred())
	Expect(string(bodyBytes)).To(MatchJSON(`{"spec":"info"}`))

	updateReq, err := http.NewRequest(http.MethodPut, logspecURL, strings.NewReader(`{"spec":"debug"}`))
	Expect(err).NotTo(HaveOccurred())

	By("setting the logspec")
	resp, err = client.Do(updateReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	resp, err = client.Get(logspecURL)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	bodyBytes, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	Expect(err).NotTo(HaveOccurred())
	Expect(string(bodyBytes)).To(MatchJSON(`{"spec":"debug"}`))
}

func CheckHealthEndpoint(client *http.Client, url string) {
	body := getBody(client, url)()

	var healthStatus healthz.HealthStatus
	err := json.Unmarshal([]byte(body), &healthStatus)
	Expect(err).NotTo(HaveOccurred())
	Expect(healthStatus.Status).To(Equal(healthz.StatusOK))
}

func getBody(client *http.Client, url string) func() string {
	return func() string {
		resp, err := client.Get(url)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		resp.Body.Close()
		return string(bodyBytes)
	}
}
