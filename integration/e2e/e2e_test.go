/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
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

			network = nwo.New(nwo.BasicSolo(), testDir, client, StartPort(), components)
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
			nwo.DeployChaincodeLegacy(network, "testchannel", orderer, chaincode)

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

			By("setting up a channel from a base profile")
			additionalPeer := network.Peer("Org2", "peer1")
			network.CreateChannel("baseprofilechannel", orderer, peer, additionalPeer)
		})
	})

	Describe("basic kafka network with 2 orgs", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.BasicKafka(), testDir, client, StartPort(), components)
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
			nwo.DeployChaincodeLegacy(network, "testchannel", orderer, chaincode)
			RunQueryInvokeQuery(network, orderer, peer, "testchannel")

			CheckPeerOperationEndpoints(network, network.Peer("Org2", "peer1"))
			CheckOrdererOperationEndpoints(network, orderer)
		})
	})

	Describe("basic single node etcdraft network", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.MultiChannelEtcdRaft(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		It("creates two channels with two orgs trying to reconfigure and update metadata", func() {
			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			By("Create first channel and deploy the chaincode")
			network.CreateAndJoinChannel(orderer, "testchannel1")
			nwo.DeployChaincodeLegacy(network, "testchannel1", orderer, chaincode)
			RunQueryInvokeQuery(network, orderer, peer, "testchannel1")

			By("Create second channel and deploy chaincode")
			network.CreateAndJoinChannel(orderer, "testchannel2")
			nwo.InstantiateChaincodeLegacy(network, "testchannel2", orderer, chaincode, peer, network.PeersWithChannel("testchannel2")...)
			RunQueryInvokeQuery(network, orderer, peer, "testchannel2")

			By("Update consensus metadata to increase snapshot interval")
			snapDir := path.Join(network.RootDir, "orderers", orderer.ID(), "etcdraft", "snapshot", "testchannel1")
			files, err := ioutil.ReadDir(snapDir)
			Expect(err).NotTo(HaveOccurred())
			numOfSnaps := len(files)

			nwo.UpdateConsensusMetadata(network, peer, orderer, "testchannel1", func(originalMetadata []byte) []byte {
				metadata := &etcdraft.ConfigMetadata{}
				err := proto.Unmarshal(originalMetadata, metadata)
				Expect(err).NotTo(HaveOccurred())

				// update max in flight messages
				metadata.Options.MaxInflightBlocks = 1000
				metadata.Options.SnapshotIntervalSize = 10 * 1024 * 1024 // 10 MB

				// write metadata back
				newMetadata, err := proto.Marshal(metadata)
				Expect(err).NotTo(HaveOccurred())
				return newMetadata
			})

			// assert that no new snapshot is taken because SnapshotIntervalSize has just enlarged
			files, err = ioutil.ReadDir(snapDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(files)).To(Equal(numOfSnaps))

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
			n.PeerAddress(n.Peer("Org1", "peer1"), nwo.ListenPort),
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
			n.PeerAddress(n.Peer("Org1", "peer1"), nwo.ListenPort),
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
	Expect(contents).To(ContainSubstring(prefix + ".logging.entries_checked.info:"))
	Expect(contents).To(ContainSubstring(prefix + ".logging.entries_written.info:"))
	Expect(contents).To(ContainSubstring(prefix + ".go.mem.gc_completed_count:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.unary_requests_received.protos_Endorser.ProcessProposal:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.unary_requests_completed.protos_Endorser.ProcessProposal.OK:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.unary_request_duration.protos_Endorser.ProcessProposal.OK:"))
	Expect(contents).To(ContainSubstring(prefix + ".ledger.blockchain_height"))
	Expect(contents).To(ContainSubstring(prefix + ".ledger.blockstorage_commit_time"))
	Expect(contents).To(ContainSubstring(prefix + ".ledger.blockstorage_and_pvtdata_commit_time"))
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
	Expect(contents).To(ContainSubstring(prefix + ".logging.entries_checked.info:"))
	Expect(contents).To(ContainSubstring(prefix + ".logging.entries_written.info:"))
	Expect(contents).To(ContainSubstring(prefix + ".go.mem.gc_completed_count:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_requests_received.orderer_AtomicBroadcast.Deliver:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_requests_completed.orderer_AtomicBroadcast.Deliver."))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_messages_received.orderer_AtomicBroadcast.Deliver"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_messages_sent.orderer_AtomicBroadcast.Deliver"))
	Expect(contents).To(ContainSubstring(prefix + ".ledger.blockchain_height"))
	Expect(contents).To(ContainSubstring(prefix + ".ledger.blockstorage_commit_time"))
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
	Expect(body).To(ContainSubstring(`ledger_blockchain_height`))
	Expect(body).To(ContainSubstring(`ledger_blockstorage_commit_time_bucket`))
	Expect(body).To(ContainSubstring(`ledger_blockstorage_and_pvtdata_commit_time_bucket`))
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
	Expect(body).To(ContainSubstring(`ledger_blockchain_height`))
	Expect(body).To(ContainSubstring(`ledger_blockstorage_commit_time_bucket`))
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
