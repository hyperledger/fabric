// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	perf "github.com/hyperledger/fabric/orderer/common/performance"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

// USAGE
//
//  BENCHMARK=true go test -run=TestOrdererBenchmark[Solo|Kafka][Broadcast|Deliver]
//
// You can specify a specific test permutation by specifying the complete subtest
// name corresponding to the permutation you would like to run. e.g:
//
//  TestOrdererBenchmark[Solo|Kafka][Broadcast|Deliver]/10ch/10000tx/10kb/10bc/0dc/10ord
//
// (The permutation has to be valid as defined in the source code)
//
// RUNNING KAFKA ORDERER BENCHMARKS
//
// A Kafka cluster is required to run the Kafka-based benchmark. The benchmark
// expects to find a seed broker is listening on localhost:9092.
//
// A suitable Kafka cluster is provided as a docker compose application defined
// in the docker-compose.yml file provided with this package. To run the Kafka
// benchmarks with the provided Kafaka cluster:
//
//    From this package's directory, first run:
//
//       docker-compose up -d
//
//    Then execute:
//
//       BENCHMARK=true go test -run TestOrdererBenchmarkKafkaBroadcast
//
// If you are not using the Kafka cluster provided in the docker-compose.yml,
// the list of seed brokers can be adjusted by setting the value of
// x_ORDERERS_KAFKA_BROKERS in the `envvars` map below.
//
// DESCRIPTION
//
// Benchmark test makes [ch] channels, creates [bc] clients per channel per orderer. There
// are [ord] orderer instances in total. A client ONLY interacts with ONE channel and ONE
// orderer, so the number of client in total is [ch * bc * ord]. Note that all clients
// execute concurrently.
//
// The test sends [tx] transactions of size [kb] in total. These tx are evenly distributed
// among all clients, which gives us [tx / (ch * bc * ord)] tx per client.
//
// For example, given following configuration:
//
// channels          [ch]: 4 (CH0, CH1, CH2, CH3)
// broadcast clients [bc]: 5
// orderers         [ord]: 2 (ORD0, ORD1)
// transactions      [tx]: 200
//
// We will spawn 4 * 5 * 2 = 40 simultaneous broadcast clients in total, 20 clients per
// orderer. For each orderer, there will be 5 clients per channel. Each client will send
// 200 / 40 = 5 transactions. Pseudo-code would be:
//
// for each one of [ord] orderers:
//     for each one of [ch] channels:
//         for each one of [bc] clients:
//             go send [tx / (ch * bc * ord)] tx
//
// Additionally, [dc] deliver clients per channel per orderer seeks all the blocks in
// that channel. It would 'predict' the last block number, and seek from the oldest to
// that. In this manner, we could reliably assert that all transactions we send are
// ordered. This is important for evaluating elapsed time of async broadcast operations.
//
// Again, each deliver client only interacts with one channel and one orderer, which
// results in [ch * dc * ord] deliver clients in total.
//
// ch  -> channelCounts
// bc  -> broadcastClientPerChannel
// tx  -> totalTx
// kb  -> messagesSizes
// ord -> numOfOrderer
// dc  -> deliverClientPerChannel
//
// If `multiplex` is true, broadcast and deliver are running simultaneously. Otherwise,
// broadcast is run before deliver. This is useful when testing deliver performance only,
// as deliver is effectively retrieving pre-generated blocks, so it shouldn't be choked
// by slower broadcast.
//

const (
	MaxMessageCount = 10

	// This is the hard limit for all types of tx, including config tx, which is normally
	// larger than 13 KB. Therefore, for config tx not to be rejected, this value cannot
	// be less than 13 KB.
	AbsoluteMaxBytes  = 16 // KB
	PreferredMaxBytes = 10 // KB
	ChannelProfile    = genesisconfig.SampleSingleMSPChannelProfile
)

var envvars = map[string]string{
	"ORDERER_GENERAL_GENESISPROFILE":                              genesisconfig.SampleDevModeSoloProfile,
	"ORDERER_GENERAL_LEDGERTYPE":                                  "file",
	"ORDERER_GENERAL_LOGLEVEL":                                    "error",
	"ORDERER_KAFKA_VERBOSE":                                       "false",
	genesisconfig.Prefix + "_ORDERER_BATCHSIZE_MAXMESSAGECOUNT":   strconv.Itoa(MaxMessageCount),
	genesisconfig.Prefix + "_ORDERER_BATCHSIZE_ABSOLUTEMAXBYTES":  strconv.Itoa(AbsoluteMaxBytes) + " KB",
	genesisconfig.Prefix + "_ORDERER_BATCHSIZE_PREFERREDMAXBYTES": strconv.Itoa(PreferredMaxBytes) + " KB",
	genesisconfig.Prefix + "_ORDERER_KAFKA_BROKERS":               "[localhost:9092]",
}

type factors struct {
	numOfChannels             int // number of channels
	totalTx                   int // total number of messages
	messageSize               int // message size in KB
	broadcastClientPerChannel int // concurrent broadcast clients
	deliverClientPerChannel   int // concurrent deliver clients
	numOfOrderer              int // number of orderer instances (Kafka ONLY)
}

// This is to give each test run a better name. The output
// would be something like '4ch/100tx/5kb/100bc/100dc/5ord'
func (f factors) String() string {
	return fmt.Sprintf(
		"%dch/%dtx/%dkb/%dbc/%ddc/%dord",
		f.numOfChannels,
		f.totalTx,
		f.messageSize,
		f.broadcastClientPerChannel,
		f.deliverClientPerChannel,
		f.numOfOrderer,
	)
}

// As benchmark tests are skipped by default, we put this test here to catch
// potential code changes that might break benchmark tests. If this test fails,
// it is likely that benchmark tests need to be updated.
func TestOrdererBenchmarkSolo(t *testing.T) {
	for key, value := range envvars {
		os.Setenv(key, value)
		defer os.Unsetenv(key)
	}

	t.Run("Benchmark Sample Test (Solo)", func(t *testing.T) {
		benchmarkOrderer(t, 1, 5, PreferredMaxBytes, 1, 0, 1, true)
	})
}

// Benchmark broadcast API in Solo mode
func TestOrdererBenchmarkSoloBroadcast(t *testing.T) {
	if os.Getenv("BENCHMARK") == "" {
		t.Skip("Skipping benchmark test")
	}

	for key, value := range envvars {
		os.Setenv(key, value)
		defer os.Unsetenv(key)
	}

	var (
		channelCounts             = []int{1, 10, 50}
		totalTx                   = []int{10000}
		messagesSizes             = []int{1, 2, 10}
		broadcastClientPerChannel = []int{1, 10, 50}
		deliverClientPerChannel   = []int{0} // We are not interested in deliver performance here
		numOfOrderer              = []int{1}

		args = [][]int{
			channelCounts,
			totalTx,
			messagesSizes,
			broadcastClientPerChannel,
			deliverClientPerChannel,
			numOfOrderer,
		}
	)

	for factors := range combinations(args) {
		t.Run(factors.String(), func(t *testing.T) {
			benchmarkOrderer(
				t,
				factors.numOfChannels,
				factors.totalTx,
				factors.messageSize,
				factors.broadcastClientPerChannel,
				factors.deliverClientPerChannel,
				1, // For solo orderer, we should always have exactly one instance
				true,
			)
		})
	}
}

// Benchmark deliver API in Solo mode
func TestOrdererBenchmarkSoloDeliver(t *testing.T) {
	if os.Getenv("BENCHMARK") == "" {
		t.Skip("Skipping benchmark test")
	}

	for key, value := range envvars {
		os.Setenv(key, value)
		defer os.Unsetenv(key)
	}

	var (
		channelCounts             = []int{1, 10, 50}
		totalTx                   = []int{10000}
		messagesSizes             = []int{1, 2, 10}
		broadcastClientPerChannel = []int{100}
		deliverClientPerChannel   = []int{1, 10, 50}
		numOfOrderer              = []int{1}

		args = [][]int{
			channelCounts,
			totalTx,
			messagesSizes,
			broadcastClientPerChannel,
			deliverClientPerChannel,
			numOfOrderer,
		}
	)

	for factors := range combinations(args) {
		t.Run(factors.String(), func(t *testing.T) {
			benchmarkOrderer(
				t,
				factors.numOfChannels,
				factors.totalTx,
				factors.messageSize,
				factors.broadcastClientPerChannel,
				factors.deliverClientPerChannel,
				1, // For solo orderer, we should always have exactly one instance
				false,
			)
		})
	}
}

// Benchmark broadcast API in Kafka mode
func TestOrdererBenchmarkKafkaBroadcast(t *testing.T) {
	if os.Getenv("BENCHMARK") == "" {
		t.Skip("Skipping benchmark test")
	}

	for key, value := range envvars {
		os.Setenv(key, value)
		defer os.Unsetenv(key)
	}

	os.Setenv("ORDERER_GENERAL_GENESISPROFILE", genesisconfig.SampleDevModeKafkaProfile)
	defer os.Unsetenv("ORDERER_GENERAL_GENESISPROFILE")

	var (
		channelCounts             = []int{1, 10}
		totalTx                   = []int{10000}
		messagesSizes             = []int{1, 2, 10}
		broadcastClientPerChannel = []int{1, 10, 50}
		deliverClientPerChannel   = []int{0} // We are not interested in deliver performance here
		numOfOrderer              = []int{1, 5, 10}

		args = [][]int{
			channelCounts,
			totalTx,
			messagesSizes,
			broadcastClientPerChannel,
			deliverClientPerChannel,
			numOfOrderer,
		}
	)

	for factors := range combinations(args) {
		t.Run(factors.String(), func(t *testing.T) {
			benchmarkOrderer(
				t,
				factors.numOfChannels,
				factors.totalTx,
				factors.messageSize,
				factors.broadcastClientPerChannel,
				factors.deliverClientPerChannel,
				factors.numOfOrderer,
				true,
			)
		})
	}
}

// Benchmark deliver API in Kafka mode
func TestOrdererBenchmarkKafkaDeliver(t *testing.T) {
	if os.Getenv("BENCHMARK") == "" {
		t.Skip("Skipping benchmark test")
	}

	for key, value := range envvars {
		os.Setenv(key, value)
		defer os.Unsetenv(key)
	}

	os.Setenv("ORDERER_GENERAL_GENESISPROFILE", genesisconfig.SampleDevModeKafkaProfile)
	defer os.Unsetenv("ORDERER_GENERAL_GENESISPROFILE")

	var (
		channelCounts             = []int{1, 10}
		totalTx                   = []int{10000}
		messagesSizes             = []int{1, 2, 10}
		broadcastClientPerChannel = []int{50}
		deliverClientPerChannel   = []int{1, 10, 50}
		numOfOrderer              = []int{1, 5, 10}

		args = [][]int{
			channelCounts,
			totalTx,
			messagesSizes,
			broadcastClientPerChannel,
			deliverClientPerChannel,
			numOfOrderer,
		}
	)

	for factors := range combinations(args) {
		t.Run(factors.String(), func(t *testing.T) {
			benchmarkOrderer(
				t,
				factors.numOfChannels,
				factors.totalTx,
				factors.messageSize,
				factors.broadcastClientPerChannel,
				factors.deliverClientPerChannel,
				factors.numOfOrderer,
				false,
			)
		})
	}
}

func benchmarkOrderer(
	t *testing.T,
	numOfChannels int,
	totalTx int,
	msgSize int,
	broadcastClientPerChannel int,
	deliverClientPerChannel int,
	numOfOrderer int,
	multiplex bool,
) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	// Initialization shared by all orderers
	conf, err := localconfig.Load()
	if err != nil {
		t.Fatal("failed to load config")
	}

	initializeLoggingLevel(conf)
	initializeLocalMsp(conf)
	perf.InitializeServerPool(numOfOrderer)

	// Load sample channel profile
	channelProfile := genesisconfig.Load(ChannelProfile)

	// Calculate intermediate variables used internally. See the comment at the beginning
	// of this file for the purpose of these vars.
	txPerClient := totalTx / (broadcastClientPerChannel * numOfChannels * numOfOrderer)
	// In case total tx cannot be fully divided, txPerClient is actually round-down value.
	// So we need to calculate the actual number of transactions to be sent. However, this
	// number should be very close to demanded number of tx, as it's fairly big.
	totalTx = txPerClient * broadcastClientPerChannel * numOfChannels * numOfOrderer

	// Estimate number of tx per block, which is used by deliver client to seek for.
	txPerChannel := totalTx / numOfChannels
	// Message size consists of payload and signature
	msg := perf.MakeNormalTx("abcdefghij", msgSize)
	actualMsgSize := len(msg.Payload) + len(msg.Signature)
	// max(1, x) in case a block can only contain exactly one tx
	txPerBlk := min(MaxMessageCount, max(1, PreferredMaxBytes*perf.Kilo/actualMsgSize))
	// Round-down here is ok because we don't really care about trailing tx
	blkPerChannel := txPerChannel / txPerBlk

	var txCount uint64 // Atomic counter to keep track of actual tx sent

	// Generate a random system channel id for each test run,
	// so it does not recover ledgers from previous run.
	systemchannel := "system-channel-" + perf.RandomID(5)
	conf.General.SystemChannel = systemchannel

	// Spawn orderers
	for i := 0; i < numOfOrderer; i++ {
		// If we are using json or file ledger, we should use temp dir for ledger location
		// because default location "/var/hyperledger/production/orderer" in sample config
		// isn't always writable. Also separate dirs are created for each orderer instance
		// as leveldb cannot share the same dir. These temp dirs are cleaned up after each
		// test run.
		//
		// We need to make a copy of config here so that multiple orderers won't refer to
		// the same config object by address.
		localConf := localconfig.TopLevel(*conf)
		if localConf.General.LedgerType != "ram" {
			tempDir, err := ioutil.TempDir("", "fabric-benchmark-test-")
			assert.NoError(t, err, "Should be able to create temp dir")
			localConf.FileLedger.Location = tempDir
			defer os.RemoveAll(tempDir)
		}

		go Start("benchmark", &localConf)
	}

	defer perf.OrdererExec(perf.Halt)

	// Wait for server to boot and systemchannel to be ready
	perf.OrdererExec(perf.WaitForService)
	perf.OrdererExecWithArgs(perf.WaitForChannels, systemchannel)

	// Create channels
	benchmarkServers := perf.GetBenchmarkServerPool()
	channelIDs := make([]string, numOfChannels)
	txs := make(map[string]*cb.Envelope)
	for i := 0; i < numOfChannels; i++ {
		id := perf.CreateChannel(benchmarkServers[0], channelProfile) // We only need to create channel on one orderer
		channelIDs[i] = id
		txs[id] = perf.MakeNormalTx(id, msgSize)
	}

	// Wait for all the created channels to be ready
	perf.OrdererExecWithArgs(perf.WaitForChannels, stoi(channelIDs)...)

	// Broadcast loop
	broadcast := func(wg *sync.WaitGroup) {
		perf.OrdererExec(func(server *perf.BenchmarkServer) {
			var broadcastWG sync.WaitGroup
			// Since the submission and ordering of transactions are async, we need to
			// spawn a deliver client to track the progress of ordering. So there are
			// x broadcast clients and 1 deliver client per channel, so we should be
			// waiting for (numOfChannels * (x + 1)) goroutines here
			broadcastWG.Add(numOfChannels * (broadcastClientPerChannel + 1))

			for _, channelID := range channelIDs {
				go func(channelID string) {
					// Spawn a deliver instance per channel to track the progress of broadcast
					go func() {
						deliverClient := server.CreateDeliverClient()
						status, err := perf.SeekAllBlocks(deliverClient, channelID, uint64(blkPerChannel))
						assert.Equal(t, cb.Status_SUCCESS, status, "Expect deliver reply to be SUCCESS")
						assert.NoError(t, err, "Expect deliver handler to exist normally")

						broadcastWG.Done()
					}()

					for c := 0; c < broadcastClientPerChannel; c++ {
						go func() {
							broadcastClient := server.CreateBroadcastClient()
							defer func() {
								broadcastClient.Close()
								err := <-broadcastClient.Errors()
								assert.NoError(t, err, "Expect broadcast handler to shutdown gracefully")
							}()

							for i := 0; i < txPerClient; i++ {
								atomic.AddUint64(&txCount, 1)
								broadcastClient.SendRequest(txs[channelID])
								assert.Equal(t, cb.Status_SUCCESS, broadcastClient.GetResponse().Status, "Expect enqueue to succeed")
							}
							broadcastWG.Done()
						}()
					}
				}(channelID)
			}

			broadcastWG.Wait()
		})

		if wg != nil {
			wg.Done()
		}
	}

	// Deliver loop
	deliver := func(wg *sync.WaitGroup) {
		perf.OrdererExec(func(server *perf.BenchmarkServer) {
			var deliverWG sync.WaitGroup
			deliverWG.Add(deliverClientPerChannel * numOfChannels)
			for g := 0; g < deliverClientPerChannel; g++ {
				go func() {
					for _, channelID := range channelIDs {
						go func(channelID string) {
							deliverClient := server.CreateDeliverClient()
							status, err := perf.SeekAllBlocks(deliverClient, channelID, uint64(blkPerChannel))
							assert.Equal(t, cb.Status_SUCCESS, status, "Expect deliver reply to be SUCCESS")
							assert.NoError(t, err, "Expect deliver handler to exist normally")

							deliverWG.Done()
						}(channelID)
					}
				}()
			}
			deliverWG.Wait()
		})

		if wg != nil {
			wg.Done()
		}
	}

	var wg sync.WaitGroup
	var btime, dtime time.Duration

	if multiplex {
		// Parallel
		start := time.Now()

		wg.Add(2)
		go broadcast(&wg)
		go deliver(&wg)
		wg.Wait()

		btime = time.Since(start)
		dtime = time.Since(start)
	} else {
		// Serial
		start := time.Now()
		broadcast(nil)
		btime = time.Since(start)

		start = time.Now()
		deliver(nil)
		dtime = time.Since(start)
	}

	// Assert here to guard against programming error caused by miscalculation of message count.
	// Experiment shows that atomic counter is not bottleneck.
	assert.Equal(t, uint64(totalTx), txCount, "Expected to send %d msg, but actually sent %d", uint64(totalTx), txCount)

	ordererProfile := os.Getenv("ORDERER_GENERAL_GENESISPROFILE")

	fmt.Printf(
		"Messages: %6d  Message Size: %3dKB  Channels: %3d Orderer (%s): %2d | "+
			"Broadcast Clients: %3d  Write tps: %5.1f tx/s Elapsed Time: %0.2fs | "+
			"Deliver clients: %3d  Read tps: %8.1f blk/s Elapsed Time: %0.2fs\n",
		totalTx,
		msgSize,
		numOfChannels,
		ordererProfile,
		numOfOrderer,
		broadcastClientPerChannel*numOfChannels*numOfOrderer,
		float64(totalTx)/btime.Seconds(),
		btime.Seconds(),
		deliverClientPerChannel*numOfChannels*numOfOrderer,
		float64(blkPerChannel*deliverClientPerChannel*numOfChannels)/dtime.Seconds(),
		dtime.Seconds())
}

func combinations(args [][]int) <-chan factors {
	ch := make(chan factors)
	go func() {
		defer close(ch)
		for c := range combine(args) {
			ch <- factors{
				numOfChannels:             c[0],
				totalTx:                   c[1],
				messageSize:               c[2],
				broadcastClientPerChannel: c[3],
				deliverClientPerChannel:   c[4],
				numOfOrderer:              c[5],
			}
		}
	}()
	return ch
}

// this generates all combinations of elements in arrays.
// for example,
// given
// [[A, B],
//  [C, D]],
// the result should be
// [A, C]
// [A, D]
// [B, C]
// [B, D]
func combine(args [][]int) <-chan []int {
	ch := make(chan []int)
	go func() {
		defer close(ch)
		if len(args) == 1 {
			for _, i := range args[0] {
				ch <- []int{i}
			}
		} else {
			for _, i := range args[0] {
				for j := range combine(args[1:]) {
					ch <- append([]int{i}, j...)
				}
			}
		}
	}()
	return ch
}

func min(x, y int) int {
	if x > y {
		return y
	}
	return x
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func stoi(s []string) (ret []interface{}) {
	ret = make([]interface{}, len(s))
	for i, d := range s {
		ret[i] = d
	}
	return
}
