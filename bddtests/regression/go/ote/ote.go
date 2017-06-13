/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

// Orderer Traffic Engine
// ======================
//
// This file ote.go contains main(), for executing from command line
// using environment variables to override those in sampleconfig/orderer.yaml
// or to set OTE test configuration parameters.
//
// Function ote() is called by main after reading environment variables,
// and is also called via "go test" from tests in ote_test.go. Those
// tests can be executed from automated Continuous Integration processes,
// which can use https://github.com/jstemmer/go-junit-report to convert the
// logs to produce junit output for CI reports.
//   go get github.com/jstemmer/go-junit-report
//   go test -v | go-junit-report > report.xml
//
// ote() invokes tool driver.sh (including network.json and json2yml.js) -
//   which is only slightly modified from the original version at
//   https://github.com/dongmingh/v1FabricGenOption -
//   to launch an orderer service network per the specified parameters
//   (including kafka brokers or other necessary support processes).
//   Function ote() performs several actions:
// + create Producer clients to connect via grpc to all the channels on
//   all the orderers to send/broadcast transaction messages
// + create Consumer clients to connect via grpc to ListenAddress:ListenPort
//   on all channels on all orderers and call deliver() to receive messages
//   containing batches of transactions
// + use parameters for specifying test configuration such as:
//   number of transactions, number of channels, number of orderers ...
// + load orderer/orderer.yml to retrieve environment variables used for
//   overriding orderer configuration such as batchsize, batchtimeout ...
// + generate unique transactions, dividing up the requested OTE_TXS count
//   among all the Producers
// + Consumers confirm the same number of blocks and TXs are delivered
//   by all the orderers on all the channels
// + print logs for any errors, and print final tallied results
// + return a pass/fail result and a result summary string

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig" // config for genesis.yaml
	genesisconfigProvisional "github.com/hyperledger/fabric/common/configtx/tool/provisional"
	"github.com/hyperledger/fabric/orderer/localconfig" // config, for the orderer.yaml
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var ordConf *config.TopLevel
var genConf *genesisconfig.TopLevel
var genesisConfigLocation = "CONFIGTX_ORDERER_"
var ordererConfigLocation = "ORDERER_GENERAL_"
var batchSizeParamStr = genesisConfigLocation + "BATCHSIZE_MAXMESSAGECOUNT"
var batchTimeoutParamStr = genesisConfigLocation + "BATCHTIMEOUT"
var ordererTypeParamStr = genesisConfigLocation + "ORDERERTYPE"

var debugflagLaunch = false
var debugflagAPI = true
var debugflag1 = false
var debugflag2 = false
var debugflag3 = false // most detailed and voluminous

var producersWG sync.WaitGroup
var logFile *os.File
var logEnabled = false
var envvar string

var numChannels = 1
var numOrdsInNtwk = 1
var numOrdsToWatch = 1
var ordererType = "solo"
var numKBrokers int
var producersPerCh = 1
var numConsumers = 1
var numProducers = 1

// numTxToSend is the total number of Transactions to send; A fraction is
// sent by each producer for each channel for each orderer.

var numTxToSend int64 = 1

// One GO thread is created for each producer and each consumer client.
// To optimize go threads usage, to prevent running out of swap space
// in the (laptop) test environment for tests using either numerous
// channels or numerous producers per channel, set bool optimizeClientsMode
// true to only create one go thread MasterProducer per orderer, which will
// broadcast messages to all channels on one orderer. Note this option
// works a little less efficiently on the consumer side, where we
// share a single grpc connection but still need to use separate
// GO threads per channel per orderer (instead of one per orderer).

var optimizeClientsMode = false

// ordStartPort (default port is 7050, but driver.sh uses 5005).

var ordStartPort uint16 = 5005

func initialize() {
	// When running multiple tests, e.g. from go test, reset to defaults
	// for the parameters that could change per test.
	// We do NOT reset things that would apply to every test, such as
	// settings for environment variables
	logEnabled = false
	envvar = ""
	numChannels = 1
	numOrdsInNtwk = 1
	numOrdsToWatch = 1
	ordererType = "solo"
	numKBrokers = 0
	numConsumers = 1
	numProducers = 1
	numTxToSend = 1
	producersPerCh = 1
	initLogger("ote")
}

func initLogger(fileName string) {
	if !logEnabled {
		layout := "Jan_02_2006"
		// Format Now with the layout const.
		t := time.Now()
		res := t.Format(layout)
		var err error
		logFile, err = os.OpenFile(fileName+"-"+res+".log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			panic(fmt.Sprintf("error opening file: %s", err))
		}
		logEnabled = true
		log.SetOutput(logFile)
		//log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.SetFlags(log.LstdFlags)
	}
}

func logger(printStmt string) {
	fmt.Println(printStmt)
	if !logEnabled {
		return
	}
	log.Println(printStmt)
}

func closeLogger() {
	if logFile != nil {
		logFile.Close()
	}
	logEnabled = false
}

type ordererdriveClient struct {
	client  ab.AtomicBroadcast_DeliverClient
	chainID string
}
type broadcastClient struct {
	client  ab.AtomicBroadcast_BroadcastClient
	chainID string
}

func newOrdererdriveClient(client ab.AtomicBroadcast_DeliverClient, chainID string) *ordererdriveClient {
	return &ordererdriveClient{client: client, chainID: chainID}
}
func newBroadcastClient(client ab.AtomicBroadcast_BroadcastClient, chainID string) *broadcastClient {
	return &broadcastClient{client: client, chainID: chainID}
}

func seekHelper(chainID string, start *ab.SeekPosition) *cb.Envelope {
	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				//ChainHeader: &cb.ChainHeader{
				//        ChainID: b.chainID,
				//},
				ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
					ChannelId: chainID,
				}),
				SignatureHeader: utils.MarshalOrPanic(&cb.SignatureHeader{}),
			},

			Data: utils.MarshalOrPanic(&ab.SeekInfo{
				Start:    &ab.SeekPosition{Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}}},
				Stop:     &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: math.MaxUint64}}},
				Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
			}),
		}),
	}
}

func (r *ordererdriveClient) seekOldest() error {
	return r.client.Send(seekHelper(r.chainID, &ab.SeekPosition{Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}}}))
}

func (r *ordererdriveClient) seekNewest() error {
	return r.client.Send(seekHelper(r.chainID, &ab.SeekPosition{Type: &ab.SeekPosition_Newest{Newest: &ab.SeekNewest{}}}))
}

func (r *ordererdriveClient) seek(blockNumber uint64) error {
	return r.client.Send(seekHelper(r.chainID, &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: blockNumber}}}))
}

func (r *ordererdriveClient) readUntilClose(ordererIndex int, channelIndex int, txRecvCntrP *int64, blockRecvCntrP *int64) {
	for {
		msg, err := r.client.Recv()
		if err != nil {
			if !strings.Contains(err.Error(), "transport is closing") {
				// print if we do not see the msg indicating graceful closing of the connection
				logger(fmt.Sprintf("Consumer for orderer %d channel %d readUntilClose() Recv error: %v", ordererIndex, channelIndex, err))
			}
			return
		}
		switch t := msg.Type.(type) {
		case *ab.DeliverResponse_Status:
			logger(fmt.Sprintf("Got DeliverResponse_Status: %v", t))
			return
		case *ab.DeliverResponse_Block:
			if t.Block.Header.Number > 0 {
				if debugflag2 {
					logger(fmt.Sprintf("Consumer recvd a block, o %d c %d blkNum %d numtrans %d", ordererIndex, channelIndex, t.Block.Header.Number, len(t.Block.Data.Data)))
				}
				if debugflag3 {
					logger(fmt.Sprintf("blk: %v", t.Block.Data.Data))
				}
			}
			*txRecvCntrP += int64(len(t.Block.Data.Data))
			//*blockRecvCntrP = int64(t.Block.Header.Number) // this assumes header number is the block number; instead let's just add one
			(*blockRecvCntrP)++
		}
	}
}

func (b *broadcastClient) broadcast(transaction []byte) error {
	payload, err := proto.Marshal(&cb.Payload{
		Header: &cb.Header{
			//ChainHeader: &cb.ChainHeader{
			//        ChainID: b.chainID,
			//},
			ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
				ChannelId: b.chainID,
			}),
			SignatureHeader: utils.MarshalOrPanic(&cb.SignatureHeader{}),
		},
		Data: transaction,
	})
	if err != nil {
		panic(err)
	}
	return b.client.Send(&cb.Envelope{Payload: payload})
}

func (b *broadcastClient) getAck() error {
	msg, err := b.client.Recv()
	if err != nil {
		return err
	}
	if msg.Status != cb.Status_SUCCESS {
		return fmt.Errorf("Got unexpected status: %v", msg.Status)
	}
	return nil
}

func startConsumer(serverAddr string, chainID string, ordererIndex int, channelIndex int, txRecvCntrP *int64, blockRecvCntrP *int64, consumerConnP **grpc.ClientConn) {
	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
	if err != nil {
		logger(fmt.Sprintf("Error on Consumer ord[%d] ch[%d] connecting (grpc) to %s, err: %v", ordererIndex, channelIndex, serverAddr, err))
		return
	}
	(*consumerConnP) = conn
	client, err := ab.NewAtomicBroadcastClient(*consumerConnP).Deliver(context.TODO())
	if err != nil {
		logger(fmt.Sprintf("Error on Consumer ord[%d] ch[%d] invoking Deliver() on grpc connection to %s, err: %v", ordererIndex, channelIndex, serverAddr, err))
		return
	}
	s := newOrdererdriveClient(client, chainID)
	err = s.seekOldest()
	if err == nil {
		if debugflag1 {
			logger(fmt.Sprintf("Started Consumer to recv delivered batches from ord[%d] ch[%d] srvr=%s chID=%s", ordererIndex, channelIndex, serverAddr, chainID))
		}
	} else {
		logger(fmt.Sprintf("ERROR starting Consumer client for ord[%d] ch[%d] for srvr=%s chID=%s; err: %v", ordererIndex, channelIndex, serverAddr, chainID, err))
	}
	s.readUntilClose(ordererIndex, channelIndex, txRecvCntrP, blockRecvCntrP)
}

func startConsumerMaster(serverAddr string, chainIDsP *[]string, ordererIndex int, txRecvCntrsP *[]int64, blockRecvCntrsP *[]int64, consumerConnP **grpc.ClientConn) {
	// create one conn to the orderer and share it for communications to all channels
	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
	if err != nil {
		logger(fmt.Sprintf("Error on MasterConsumer ord[%d] connecting (grpc) to %s, err: %v", ordererIndex, serverAddr, err))
		return
	}
	(*consumerConnP) = conn

	// create an orderer driver client for every channel on this orderer
	//[][]*ordererdriveClient  //  numChannels
	dc := make([]*ordererdriveClient, numChannels)
	for c := 0; c < numChannels; c++ {
		client, err := ab.NewAtomicBroadcastClient(*consumerConnP).Deliver(context.TODO())
		if err != nil {
			logger(fmt.Sprintf("Error on MasterConsumer ord[%d] invoking Deliver() on grpc connection to %s, err: %v", ordererIndex, serverAddr, err))
			return
		}
		dc[c] = newOrdererdriveClient(client, (*chainIDsP)[c])
		err = dc[c].seekOldest()
		if err == nil {
			if debugflag1 {
				logger(fmt.Sprintf("Started MasterConsumer to recv delivered batches from ord[%d] ch[%d] srvr=%s chID=%s", ordererIndex, c, serverAddr, (*chainIDsP)[c]))
			}
		} else {
			logger(fmt.Sprintf("ERROR starting MasterConsumer client for ord[%d] ch[%d] for srvr=%s chID=%s; err: %v", ordererIndex, c, serverAddr, (*chainIDsP)[c], err))
		}
		// we would prefer to skip these go threads, and just have on "readUntilClose" that looks for deliveries on all channels!!! (see below.)
		// otherwise, what have we really saved?
		go dc[c].readUntilClose(ordererIndex, c, &((*txRecvCntrsP)[c]), &((*blockRecvCntrsP)[c]))
	}
}

func executeCmd(cmd string) []byte {
	out, err := exec.Command("/bin/sh", "-c", cmd).Output()
	if err != nil {
		logger(fmt.Sprintf("Unsuccessful exec command: "+cmd+"\nstdout="+string(out)+"\nstderr=%v", err))
		log.Fatal(err)
	}
	return out
}

func executeCmdAndDisplay(cmd string) {
	out := executeCmd(cmd)
	logger("Results of exec command: " + cmd + "\nstdout=" + string(out))
}

func connClose(consumerConnsPP **([][]*grpc.ClientConn)) {
	for i := 0; i < numOrdsToWatch; i++ {
		for j := 0; j < numChannels; j++ {
			if (**consumerConnsPP)[i][j] != nil {
				_ = (**consumerConnsPP)[i][j].Close()
			}
		}
	}
}

func cleanNetwork(consumerConnsP *([][]*grpc.ClientConn)) {
	if debugflag1 {
		logger("Removing the Network Consumers")
	}
	connClose(&consumerConnsP)

	// Docker is not perfect; we need to unpause any paused containers, before we can kill them.
	//_ = executeCmd("docker ps -aq -f status=paused | xargs docker unpause")
	if out := executeCmd("docker ps -aq -f status=paused"); out != nil && string(out) != "" {
		logger("Removing paused docker containers: " + string(out))
		_ = executeCmd("docker ps -aq -f status=paused | xargs docker unpause")
	}

	// kill any containers that are still running
	//_ = executeCmd("docker kill $(docker ps -q)")

	if debugflag1 {
		logger("Removing the Network orderers and associated docker containers")
	}
	_ = executeCmd("docker rm -f $(docker ps -aq)")
}

func launchNetwork(appendFlags string) {
	// Alternative way: hardcoded docker compose (not driver.sh tool)
	//  _ = executeCmd("docker-compose -f docker-compose-3orderers.yml up -d")

	cmd := fmt.Sprintf("./driver.sh -a create -p 1 %s", appendFlags)
	logger(fmt.Sprintf("Launching network:  %s", cmd))
	if debugflagLaunch {
		executeCmdAndDisplay(cmd) // show stdout logs; debugging help
	} else {
		executeCmd(cmd)
	}

	// display the network of docker containers with the orderers and such
	executeCmdAndDisplay("docker ps -a")
}

func countGenesis() int64 {
	return int64(numChannels)
}
func sendEqualRecv(numTxToSend int64, totalTxRecvP *[]int64, totalTxRecvMismatch bool, totalBlockRecvMismatch bool) bool {
	var matching = false
	if (*totalTxRecvP)[0] == numTxToSend {
		// recv count on orderer 0 matches the send count
		if !totalTxRecvMismatch && !totalBlockRecvMismatch {
			// all orderers have same recv counters
			matching = true
		}
	}
	return matching
}

func moreDeliveries(txSentP *[][]int64, totalNumTxSentP *int64, txSentFailuresP *[][]int64, totalNumTxSentFailuresP *int64, txRecvP *[][]int64, totalTxRecvP *[]int64, totalTxRecvMismatchP *bool, blockRecvP *[][]int64, totalBlockRecvP *[]int64, totalBlockRecvMismatchP *bool) (moreReceived bool) {
	moreReceived = false
	prevTotalTxRecv := *totalTxRecvP
	computeTotals(txSentP, totalNumTxSentP, txSentFailuresP, totalNumTxSentFailuresP, txRecvP, totalTxRecvP, totalTxRecvMismatchP, blockRecvP, totalBlockRecvP, totalBlockRecvMismatchP)
	for ordNum := 0; ordNum < numOrdsToWatch; ordNum++ {
		if prevTotalTxRecv[ordNum] != (*totalTxRecvP)[ordNum] {
			moreReceived = true
		}
	}
	return moreReceived
}

func startProducer(serverAddr string, chainID string, ordererIndex int, channelIndex int, txReq int64, txSentCntrP *int64, txSentFailureCntrP *int64) {
	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
	defer func() {
		_ = conn.Close()
	}()
	if err != nil {
		logger(fmt.Sprintf("Error creating connection for Producer for ord[%d] ch[%d], err: %v", ordererIndex, channelIndex, err))
		return
	}
	client, err := ab.NewAtomicBroadcastClient(conn).Broadcast(context.TODO())
	if err != nil {
		logger(fmt.Sprintf("Error creating Producer for ord[%d] ch[%d], err: %v", ordererIndex, channelIndex, err))
		return
	}
	if debugflag1 {
		logger(fmt.Sprintf("Started Producer to send %d TXs to ord[%d] ch[%d] srvr=%s chID=%s, %v", txReq, ordererIndex, channelIndex, serverAddr, chainID, time.Now()))
	}
	b := newBroadcastClient(client, chainID)

	// print a log after sending multiples of this percentage of requested TX: 25,50,75%...
	// only on one producer, and assume all producers are generating at same rate.
	// e.g. when txReq = 50, to print log every 10. set progressPercentage = 20
	printProgressLogs := false
	var progressPercentage int64 = 25 // set this between 1 and 99
	printLogCnt := txReq * progressPercentage / 100
	if debugflag1 {
		printProgressLogs = true // to test logs for all producers
	} else {
		if txReq > 10000 && printLogCnt > 0 && ordererIndex == 0 && channelIndex == 0 {
			printProgressLogs = true
		}
	}
	var mult int64 = 0

	firstErr := false
	for i := int64(0); i < txReq; i++ {
		b.broadcast([]byte(fmt.Sprintf("Testing %v", time.Now())))
		err = b.getAck()
		if err == nil {
			(*txSentCntrP)++
			if printProgressLogs && (*txSentCntrP)%printLogCnt == 0 {
				mult++
				if debugflag1 {
					logger(fmt.Sprintf("Producer ord[%d] ch[%d] sent %4d /%4d = %3d%%, %v", ordererIndex, channelIndex, (*txSentCntrP), txReq, progressPercentage*mult, time.Now()))
				} else {
					logger(fmt.Sprintf("Sent %3d%%, %v", progressPercentage*mult, time.Now()))
				}
			}
		} else {
			(*txSentFailureCntrP)++
			if !firstErr {
				firstErr = true
				logger(fmt.Sprintf("Broadcast error on TX %d (the first error for Producer ord[%d] ch[%d]); err: %v", i+1, ordererIndex, channelIndex, err))
			}
		}
	}
	if err != nil {
		logger(fmt.Sprintf("Broadcast error on last TX %d of Producer ord[%d] ch[%d]: %v", txReq, ordererIndex, channelIndex, err))
	}
	if txReq == *txSentCntrP {
		if debugflag1 {
			logger(fmt.Sprintf("Producer finished sending broadcast msgs to ord[%d] ch[%d]: ACKs  %9d  (100%%) , %v", ordererIndex, channelIndex, *txSentCntrP, time.Now()))
		}
	} else {
		logger(fmt.Sprintf("Producer finished sending broadcast msgs to ord[%d] ch[%d]: ACKs  %9d  NACK %d  Other %d , %v", ordererIndex, channelIndex, *txSentCntrP, *txSentFailureCntrP, txReq-*txSentFailureCntrP-*txSentCntrP, time.Now()))
	}
	producersWG.Done()
}

func startProducerMaster(serverAddr string, chainIDs *[]string, ordererIndex int, txReqP *[]int64, txSentCntrP *[]int64, txSentFailureCntrP *[]int64) {
	// This function creates a grpc connection to one orderer,
	// creates multiple clients (one per numChannels) for that one orderer,
	// and sends a TX to all channels repeatedly until no more to send.
	var txReqTotal int64
	var txMax int64
	for c := 0; c < numChannels; c++ {
		txReqTotal += (*txReqP)[c]
		if txMax < (*txReqP)[c] {
			txMax = (*txReqP)[c]
		}
	}
	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
	defer func() {
		_ = conn.Close()
	}()
	if err != nil {
		logger(fmt.Sprintf("Error creating connection for MasterProducer for ord[%d], err: %v", ordererIndex, err))
		return
	}
	client, err := ab.NewAtomicBroadcastClient(conn).Broadcast(context.TODO())
	if err != nil {
		logger(fmt.Sprintf("Error creating MasterProducer for ord[%d], err: %v", ordererIndex, err))
		return
	}
	logger(fmt.Sprintf("Started MasterProducer to send %d TXs to ord[%d] srvr=%s distributed across all channels", txReqTotal, ordererIndex, serverAddr))

	// create the broadcast clients for every channel on this orderer
	bc := make([]*broadcastClient, numChannels)
	for c := 0; c < numChannels; c++ {
		bc[c] = newBroadcastClient(client, (*chainIDs)[c])
	}

	firstErr := false
	for i := int64(0); i < txMax; i++ {
		// send one TX to every broadcast client (one TX on each chnl)
		for c := 0; c < numChannels; c++ {
			if i < (*txReqP)[c] {
				// more TXs to send on this channel
				bc[c].broadcast([]byte(fmt.Sprintf("Testing %v", time.Now())))
				err = bc[c].getAck()
				if err == nil {
					(*txSentCntrP)[c]++
				} else {
					(*txSentFailureCntrP)[c]++
					if !firstErr {
						firstErr = true
						logger(fmt.Sprintf("Broadcast error on TX %d (the first error for MasterProducer on ord[%d] ch[%d] channelID=%s); err: %v", i+1, ordererIndex, c, (*chainIDs)[c], err))
					}
				}
			}
		}
	}
	if err != nil {
		logger(fmt.Sprintf("Broadcast error on last TX %d of MasterProducer on ord[%d] ch[%d]: %v", txReqTotal, ordererIndex, numChannels-1, err))
	}
	var txSentTotal int64
	var txSentFailTotal int64
	for c := 0; c < numChannels; c++ {
		txSentTotal += (*txSentCntrP)[c]
		txSentFailTotal += (*txSentFailureCntrP)[c]
	}
	if txReqTotal == txSentTotal {
		logger(fmt.Sprintf("MasterProducer finished sending broadcast msgs to all channels on ord[%d]: ACKs  %9d  (100%%)", ordererIndex, txSentTotal))
	} else {
		logger(fmt.Sprintf("MasterProducer finished sending broadcast msgs to all channels on ord[%d]: ACKs  %9d  NACK %d  Other %d", ordererIndex, txSentTotal, txSentFailTotal, txReqTotal-txSentTotal-txSentFailTotal))
	}
	producersWG.Done()
}

func computeTotals(txSent *[][]int64, totalNumTxSent *int64, txSentFailures *[][]int64, totalNumTxSentFailures *int64, txRecv *[][]int64, totalTxRecv *[]int64, totalTxRecvMismatch *bool, blockRecv *[][]int64, totalBlockRecv *[]int64, totalBlockRecvMismatch *bool) {
	// The counters for Producers are indexed by orderer (numOrdsInNtwk)
	// and channel (numChannels).
	// Total count includes all counters for all channels on ALL orderers.
	// e.g.    totalNumTxSent         = sum of txSent[*][*]
	// e.g.    totalNumTxSentFailures = sum of txSentFailures[*][*]

	*totalNumTxSent = 0
	*totalNumTxSentFailures = 0
	for i := 0; i < numOrdsInNtwk; i++ {
		for j := 0; j < numChannels; j++ {
			*totalNumTxSent += (*txSent)[i][j]
			*totalNumTxSentFailures += (*txSentFailures)[i][j]
		}
	}

	// Counters for consumers are indexed by orderer (numOrdsToWatch)
	// and channel (numChannels).
	// The total count includes all counters for all channels on
	// ONLY ONE orderer.
	// Tally up the totals for all the channels on each orderer, and
	// store them for comparison; they should all be the same.
	// e.g.    totalTxRecv[k]    = sum of txRecv[k][*]
	// e.g.    totalBlockRecv[k] = sum of blockRecv[k][*]

	*totalTxRecvMismatch = false
	*totalBlockRecvMismatch = false
	for k := 0; k < numOrdsToWatch; k++ {
		// count only the requested TXs - not the genesis block TXs
		(*totalTxRecv)[k] = -countGenesis()
		(*totalBlockRecv)[k] = -countGenesis()
		for l := 0; l < numChannels; l++ {
			(*totalTxRecv)[k] += (*txRecv)[k][l]
			(*totalBlockRecv)[k] += (*blockRecv)[k][l]
			if debugflag3 {
				logger(fmt.Sprintf("in compute(): k %d l %d txRecv[k][l] %d blockRecv[k][l] %d", k, l, (*txRecv)[k][l], (*blockRecv)[k][l]))
			}
		}
		if (k > 0) && (*totalTxRecv)[k] != (*totalTxRecv)[k-1] {
			*totalTxRecvMismatch = true
		}
		if (k > 0) && (*totalBlockRecv)[k] != (*totalBlockRecv)[k-1] {
			*totalBlockRecvMismatch = true
		}
	}
	if debugflag2 {
		logger(fmt.Sprintf("in compute(): totalTxRecv[]= %v, totalBlockRecv[]= %v", *totalTxRecv, *totalBlockRecv))
	}
}

func reportTotals(testname string, numTxToSendTotal int64, countToSend [][]int64, txSent [][]int64, totalNumTxSent int64, txSentFailures [][]int64, totalNumTxSentFailures int64, batchSize int64, txRecv [][]int64, totalTxRecv []int64, totalTxRecvMismatch bool, blockRecv [][]int64, totalBlockRecv []int64, totalBlockRecvMismatch bool, masterSpy bool, channelIDs *[]string) (successResult bool, resultStr string) {

	// default to failed
	var passFailStr = "FAILED"
	successResult = false
	resultStr = "TEST " + testname + " "

	// For each Producer, print the ordererIndex and channelIndex, the
	// number of TX requested to be sent, the actual number of TX sent,
	// and the number we failed to send.

	if numOrdsInNtwk > 3 || numChannels > 3 {
		logger(fmt.Sprintf("Print only the first 3 chans of only the first 3 ordererIdx; and any others ONLY IF they contain failures.\nTotals numOrdInNtwk=%d numChan=%d numPRODUCERs=%d", numOrdsInNtwk, numChannels, numOrdsInNtwk*numChannels))
	}
	logger("PRODUCERS   OrdererIdx  ChannelIdx ChannelID              TX Target         ACK        NACK")
	for i := 0; i < numOrdsInNtwk; i++ {
		for j := 0; j < numChannels; j++ {
			if (i < 3 && j < 3) || txSentFailures[i][j] > 0 || countToSend[i][j] != txSent[i][j]+txSentFailures[i][j] {
				logger(fmt.Sprintf("%22d%12d %-20s%12d%12d%12d", i, j, (*channelIDs)[j], countToSend[i][j], txSent[i][j], txSentFailures[i][j]))
			} else if i < 3 && j == 3 {
				logger(fmt.Sprintf("%34s", "..."))
			} else if i == 3 && j == 0 {
				logger(fmt.Sprintf("%22s", "..."))
			}
		}
	}

	// for each consumer print the ordererIndex & channel, the num blocks and the num transactions received/delivered
	if numOrdsToWatch > 3 || numChannels > 3 {
		logger(fmt.Sprintf("Print only the first 3 chans of only the first 3 ordererIdx (and the last ordererIdx if masterSpy is present), plus any others that contain failures.\nTotals numOrdIdx=%d numChanIdx=%d numCONSUMERS=%d", numOrdsToWatch, numChannels, numOrdsToWatch*numChannels))
	}
	logger("CONSUMERS   OrdererIdx  ChannelIdx ChannelID                    TXs     Batches")
	for i := 0; i < numOrdsToWatch; i++ {
		for j := 0; j < numChannels; j++ {
			if (j < 3 && (i < 3 || (masterSpy && i == numOrdsInNtwk-1))) || (i > 1 && (blockRecv[i][j] != blockRecv[1][j] || txRecv[1][j] != txRecv[1][j])) {
				// Subtract one from the received Block count and TX count, to ignore the genesis block
				// (we already ignore genesis blocks when we compute the totals in totalTxRecv[n] , totalBlockRecv[n])
				logger(fmt.Sprintf("%22d%12d %-20s%12d%12d", i, j, (*channelIDs)[j], txRecv[i][j]-1, blockRecv[i][j]-1))
			} else if i < 3 && j == 3 {
				logger(fmt.Sprintf("%34s", "..."))
			} else if i == 3 && j == 0 {
				logger(fmt.Sprintf("%22s", "..."))
			}
		}
	}

	// Check for differences on the deliveries from the orderers. These are
	// probably errors - unless the test stopped an orderer on purpose and
	// never restarted it, while the others continued to deliver TXs.
	// (If an orderer is restarted, then it would reprocess all the
	// back-ordered transactions to catch up with the others.)

	if totalTxRecvMismatch {
		logger("!!!!! Num TXs Delivered is not same on all orderers!!!!!")
	}
	if totalBlockRecvMismatch {
		logger("!!!!! Num Blocks Delivered is not same on all orderers!!!!!")
	}

	if totalTxRecvMismatch || totalBlockRecvMismatch {
		resultStr += "Orderers were INCONSISTENT! "
	}
	if totalTxRecv[0] == numTxToSendTotal {
		// recv count on orderer 0 matches the send count
		if !totalTxRecvMismatch && !totalBlockRecvMismatch {
			logger("Hooray! Every TX was successfully sent AND delivered by orderer service.")
			successResult = true
			passFailStr = "PASSED"
		} else {
			resultStr += "Every TX was successfully sent AND delivered by orderer0 but not all orderers"
		}
	} else if totalTxRecv[0] == totalNumTxSent {
		resultStr += "Every ACked TX was delivered, but failures occurred:"
	} else if totalTxRecv[0] < totalNumTxSent {
		resultStr += "BAD! Some ACKed TX were LOST by orderer service!"
	} else {
		resultStr += "BAD! Some EXTRA TX were delivered by orderer service!"
	}

	////////////////////////////////////////////////////////////////////////
	//
	// Before we declare success, let's check some more things...
	//
	// At this point, we have decided if most of the numbers make sense by
	// setting succssResult to true if the tests passed. Thus we assume
	// successReult=true and just set it to false if we find a problem.

	// Check the totals to verify if the number of blocks on each channel
	// is appropriate for the given batchSize and number of TXs sent.

	expectedBlocksOnChan := make([]int64, numChannels) // create a counter for all the channels on one orderer
	for c := 0; c < numChannels; c++ {
		var chanSentTotal int64
		for ord := 0; ord < numOrdsInNtwk; ord++ {
			chanSentTotal += txSent[ord][c]
		}
		expectedBlocksOnChan[c] = chanSentTotal / batchSize
		if chanSentTotal%batchSize > 0 {
			expectedBlocksOnChan[c]++
		}
		for ord := 0; ord < numOrdsToWatch; ord++ {
			if expectedBlocksOnChan[c] != blockRecv[ord][c]-1 { // ignore genesis block
				successResult = false
				passFailStr = "FAILED"
				logger(fmt.Sprintf("Error: Unexpected Block count %d (expected %d) on ordIndx=%d channelIDs[%d]=%s, chanSentTxTotal=%d BatchSize=%d", blockRecv[ord][c]-1, expectedBlocksOnChan[c], ord, c, (*channelIDs)[c], chanSentTotal, batchSize))
			} else {
				if debugflag1 {
					logger(fmt.Sprintf("GOOD block count %d on ordIndx=%d channelIDs[%d]=%s chanSentTxTotal=%d BatchSize=%d", expectedBlocksOnChan[c], ord, c, (*channelIDs)[c], chanSentTotal, batchSize))
				}
			}
		}
	}

	// TODO - Verify the contents of the last block of transactions.
	//        Since we do not know exactly what should be in the block,
	//        then at least we can do:
	//            for each channel, verify if the block delivered from
	//            each orderer is the same (i.e. contains the same
	//            Data bytes (transactions) in the last block)

	// print some counters totals
	logger(fmt.Sprintf("Not counting genesis blks (1 per chan)%9d", countGenesis()))
	logger(fmt.Sprintf("Total TX broadcasts Requested to Send %9d", numTxToSendTotal))
	logger(fmt.Sprintf("Total TX broadcasts send success ACK  %9d", totalNumTxSent))
	logger(fmt.Sprintf("Total TX broadcasts sendFailed - NACK %9d", totalNumTxSentFailures))
	logger(fmt.Sprintf("Total Send-LOST TX (Not Ack or Nack)) %9d", numTxToSendTotal-totalNumTxSent-totalNumTxSentFailures))
	logger(fmt.Sprintf("Total Recv-LOST TX (Ack but not Recvd)%9d", totalNumTxSent-totalTxRecv[0]))
	if successResult {
		logger(fmt.Sprintf("Total deliveries received TX          %9d", totalTxRecv[0]))
		logger(fmt.Sprintf("Total deliveries received Blocks      %9d", totalBlockRecv[0]))
	} else {
		logger(fmt.Sprintf("Total deliveries received TX on each ordrr     %7d", totalTxRecv))
		logger(fmt.Sprintf("Total deliveries received Blocks on each ordrr %7d", totalBlockRecv))
	}

	// print output result and counts : overall summary
	resultStr += fmt.Sprintf(" RESULT=%s: TX Req=%d BrdcstACK=%d NACK=%d DelivBlk=%d DelivTX=%d numChannels=%d batchSize=%d", passFailStr, numTxToSendTotal, totalNumTxSent, totalNumTxSentFailures, totalBlockRecv, totalTxRecv, numChannels, batchSize)
	logger(fmt.Sprintf(resultStr))

	return successResult, resultStr
}

// Function:    ote - the Orderer Test Engine
// Outputs:     print report to stdout with lots of counters
// Returns:     passed bool, resultSummary string
func ote(testname string, txs int64, chans int, orderers int, ordType string, kbs int, masterSpy bool, pPerCh int) (passed bool, resultSummary string) {

	initialize() // multiple go tests could be run; we must call initialize() each time

	passed = false
	resultSummary = testname + " test not completed: INPUT ERROR: "
	defer closeLogger()

	logger(fmt.Sprintf("========== OTE testname=%s TX=%d Channels=%d Orderers=%d ordererType=%s kafka-brokers=%d addMasterSpy=%t producersPerCh=%d", testname, txs, chans, orderers, ordType, kbs, masterSpy, pPerCh))

	// Establish the default configuration from yaml files - and this also
	// picks up any variables overridden on command line or in environment
	ordConf := config.Load()
	genConf := genesisconfig.Load(genesisconfig.SampleInsecureProfile)
	var launchAppendFlags string

	////////////////////////////////////////////////////////////////////////
	// Check parameters and/or env vars to see if user wishes to override
	// default config parms.
	////////////////////////////////////////////////////////////////////////

	//////////////////////////////////////////////////////////////////////
	// Arguments for OTE settings for test variations:
	//////////////////////////////////////////////////////////////////////

	if txs > 0 {
		numTxToSend = txs
	} else {
		return passed, resultSummary + "number of transactions must be > 0"
	}
	if chans > 0 {
		numChannels = chans
	} else {
		return passed, resultSummary + "number of channels must be > 0"
	}
	if orderers > 0 {
		numOrdsInNtwk = orderers
		launchAppendFlags += fmt.Sprintf(" -o %d", orderers)
	} else {
		return passed, resultSummary + "number of orderers in network must be > 0"
	}

	if pPerCh > 1 {
		producersPerCh = pPerCh
		return passed, resultSummary + "Multiple producersPerChannel NOT SUPPORTED yet."
	}

	numOrdsToWatch = numOrdsInNtwk // Watch every orderer to verify they are all delivering the same.
	if masterSpy {
		numOrdsToWatch++
	} // We are not creating another orderer here, but we do need
	// another set of counters; the masterSpy will be created for
	// this test to watch every channel on an orderer - so that means
	// one orderer is being watched twice

	// this is not an argument, but user may set this tuning parameter before running test
	envvar = os.Getenv("OTE_CLIENTS_SHARE_CONNS")
	if envvar != "" {
		if strings.ToLower(envvar) == "true" || strings.ToLower(envvar) == "t" {
			optimizeClientsMode = true
		}
		if debugflagAPI {
			logger(fmt.Sprintf("%-50s %s=%t", "OTE_CLIENTS_SHARE_CONNS="+envvar, "optimizeClientsMode", optimizeClientsMode))
			logger("Setting OTE_CLIENTS_SHARE_CONNS option to true does the following:\n1. All Consumers on an orderer (one GO thread per each channel) will share grpc connection.\n2. All Producers on an orderer will share a grpc conn AND share one GO-thread.\nAlthough this reduces concurrency and lengthens the test duration, it satisfies\nthe objective of reducing swap space requirements and should be selected when\nrunning tests with numerous channels or producers per channel.")
		}
	}
	if optimizeClientsMode {
		// use only one MasterProducer and one MasterConsumer on each orderer
		numProducers = numOrdsInNtwk
		numConsumers = numOrdsInNtwk
	} else {
		// one Producer and one Consumer for EVERY channel on each orderer
		numProducers = numOrdsInNtwk * numChannels
		numConsumers = numOrdsInNtwk * numChannels
	}

	//////////////////////////////////////////////////////////////////////
	// Arguments to override configuration parameter values in yaml file:
	//////////////////////////////////////////////////////////////////////

	// ordererType is an argument of ote(), and is also in the genesisconfig
	ordererType = genConf.Orderer.OrdererType
	if ordType != "" {
		ordererType = ordType
	} else {
		logger(fmt.Sprintf("Null value provided for ordererType; using value from config file: %s", ordererType))
	}
	launchAppendFlags += fmt.Sprintf(" -t %s", ordererType)
	if "kafka" == strings.ToLower(ordererType) {
		if kbs > 0 {
			numKBrokers = kbs
			launchAppendFlags += fmt.Sprintf(" -k %d", numKBrokers)
		} else {
			return passed, resultSummary + "When using kafka ordererType, number of kafka-brokers must be > 0"
		}
	} else {
		numKBrokers = 0
	}

	// batchSize is not an argument of ote(), but is in the genesisconfig
	// variable may be overridden on command line or by exporting it.
	batchSize := int64(genConf.Orderer.BatchSize.MaxMessageCount) // retype the uint32
	envvar = os.Getenv(batchSizeParamStr)
	if envvar != "" {
		launchAppendFlags += fmt.Sprintf(" -b %d", batchSize)
	}
	if debugflagAPI {
		logger(fmt.Sprintf("%-50s %s=%d", batchSizeParamStr+"="+envvar, "batchSize", batchSize))
	}

	// batchTimeout is not an argument of ote(), but is in the genesisconfig
	//logger(fmt.Sprintf("DEBUG=====BatchTimeout conf:%v Seconds-float():%v Seconds-int:%v", genConf.Orderer.BatchTimeout, (genConf.Orderer.BatchTimeout).Seconds(), int((genConf.Orderer.BatchTimeout).Seconds())))
	batchTimeout := int((genConf.Orderer.BatchTimeout).Seconds()) // Seconds() converts time.Duration to float64, and then retypecast to int
	envvar = os.Getenv(batchTimeoutParamStr)
	if envvar != "" {
		launchAppendFlags += fmt.Sprintf(" -c %d", batchTimeout)
	}
	if debugflagAPI {
		logger(fmt.Sprintf("%-50s %s=%d", batchTimeoutParamStr+"="+envvar, "batchTimeout", batchTimeout))
	}

	// CoreLoggingLevel
	envvar = strings.ToUpper(os.Getenv("CORE_LOGGING_LEVEL")) // (default = not set)|CRITICAL|ERROR|WARNING|NOTICE|INFO|DEBUG
	if envvar != "" {
		launchAppendFlags += fmt.Sprintf(" -l %s", envvar)
	}
	if debugflagAPI {
		logger(fmt.Sprintf("CORE_LOGGING_LEVEL=%s", envvar))
	}

	// CoreLedgerStateDB
	envvar = os.Getenv("CORE_LEDGER_STATE_STATEDATABASE") // goleveldb | CouchDB
	if envvar != "" {
		launchAppendFlags += fmt.Sprintf(" -d %s", envvar)
	}
	if debugflagAPI {
		logger(fmt.Sprintf("CORE_LEDGER_STATE_STATEDATABASE=%s", envvar))
	}

	// CoreSecurityLevel
	envvar = os.Getenv("CORE_SECURITY_LEVEL") // 256 | 384
	if envvar != "" {
		launchAppendFlags += fmt.Sprintf(" -w %s", envvar)
	}
	if debugflagAPI {
		logger(fmt.Sprintf("CORE_SECURITY_LEVEL=%s", envvar))
	}

	// CoreSecurityHashAlgorithm
	envvar = os.Getenv("CORE_SECURITY_HASHALGORITHM") // SHA2 | SHA3
	if envvar != "" {
		launchAppendFlags += fmt.Sprintf(" -x %s", envvar)
	}
	if debugflagAPI {
		logger(fmt.Sprintf("CORE_SECURITY_HASHALGORITHM=%s", envvar))
	}

	//////////////////////////////////////////////////////////////////////////
	// Each producer sends TXs to one channel on one orderer, and increments
	// its own counters for the successfully sent Tx, and the send-failures
	// (rejected/timeout). These arrays are indexed by dimensions:
	// numOrdsInNtwk and numChannels

	var countToSend [][]int64
	var txSent [][]int64
	var txSentFailures [][]int64
	var totalNumTxSent int64
	var totalNumTxSentFailures int64

	// Each consumer receives blocks delivered on one channel from one
	// orderer, and must track its own counters for the received number of
	// blocks and received number of Tx.
	// We will create consumers for every channel on an orderer, and total
	// up the TXs received. And do that for all the orderers (indexed by
	// numOrdsToWatch). We will check to ensure all the orderers receive
	// all the same deliveries. These arrays are indexed by dimensions:
	// numOrdsToWatch and numChannels

	var txRecv [][]int64
	var blockRecv [][]int64
	var totalTxRecv []int64    // total TXs rcvd by all consumers on an orderer, indexed by numOrdsToWatch
	var totalBlockRecv []int64 // total Blks recvd by all consumers on an orderer, indexed by numOrdsToWatch
	var totalTxRecvMismatch = false
	var totalBlockRecvMismatch = false
	var consumerConns [][]*grpc.ClientConn

	////////////////////////////////////////////////////////////////////////
	// Create the 1D and 2D slices of counters for the producers and
	// consumers. All are initialized to zero.

	for i := 0; i < numOrdsInNtwk; i++ { // for all orderers

		countToSendForOrd := make([]int64, numChannels)      // create a counter for all the channels on one orderer
		countToSend = append(countToSend, countToSendForOrd) // orderer-i gets a set

		sendPassCntrs := make([]int64, numChannels) // create a counter for all the channels on one orderer
		txSent = append(txSent, sendPassCntrs)      // orderer-i gets a set

		sendFailCntrs := make([]int64, numChannels)            // create a counter for all the channels on one orderer
		txSentFailures = append(txSentFailures, sendFailCntrs) // orderer-i gets a set
	}

	for i := 0; i < numOrdsToWatch; i++ { // for all orderers which we will watch/monitor for deliveries

		blockRecvCntrs := make([]int64, numChannels)  // create a set of block counters for each channel
		blockRecv = append(blockRecv, blockRecvCntrs) // orderer-i gets a set

		txRecvCntrs := make([]int64, numChannels) // create a set of tx counters for each channel
		txRecv = append(txRecv, txRecvCntrs)      // orderer-i gets a set

		consumerRow := make([]*grpc.ClientConn, numChannels)
		consumerConns = append(consumerConns, consumerRow)
	}

	totalTxRecv = make([]int64, numOrdsToWatch)    // create counter for each orderer, for total tx received (for all channels)
	totalBlockRecv = make([]int64, numOrdsToWatch) // create counter for each orderer, for total blk received (for all channels)

	////////////////////////////////////////////////////////////////////////

	launchNetwork(launchAppendFlags)
	time.Sleep(10 * time.Second)

	////////////////////////////////////////////////////////////////////////
	// Create the 1D slice of channel IDs, and create names for them
	// which we will use when producing/broadcasting/sending msgs and
	// consuming/delivering/receiving msgs.

	var channelIDs []string
	channelIDs = make([]string, numChannels)

	// TODO (after FAB-2001 and FAB-2083 are fixed) - Remove the if-then clause.
	// Due to those bugs, we cannot pass many tests using multiple orderers and multiple channels.
	// TEMPORARY PARTIAL SOLUTION: To test multiple orderers with a single channel,
	// use hardcoded TestChainID and skip creating any channels.
	if numChannels == 1 {
		channelIDs[0] = genesisconfigProvisional.TestChainID
		logger(fmt.Sprintf("Using DEFAULT channelID = %s", channelIDs[0]))
	} else {
		logger(fmt.Sprintf("Using %d new channelIDs, e.g. test-chan.00023", numChannels))
		for c := 0; c < numChannels; c++ {
			channelIDs[c] = fmt.Sprintf("test-chan.%05d", c)
			cmd := fmt.Sprintf("cd $GOPATH/src/github.com/hyperledger/fabric && peer channel create -c %s", channelIDs[c])
			_ = executeCmd(cmd)
			//executeCmdAndDisplay(cmd)
		}
	}

	////////////////////////////////////////////////////////////////////////
	// Start threads for each consumer to watch each channel on all (the
	// specified number of) orderers. This code assumes orderers in the
	// network will use increasing port numbers, which is the same logic
	// used by the driver.sh tool that starts the network for us: the first
	// orderer uses ordStartPort, the second uses ordStartPort+1, etc.

	for ord := 0; ord < numOrdsToWatch; ord++ {
		serverAddr := fmt.Sprintf("%s:%d", ordConf.General.ListenAddress, ordStartPort+uint16(ord))
		if masterSpy && ord == numOrdsToWatch-1 {
			// Special case: this is the last row of counters,
			// added (and incremented numOrdsToWatch) for the
			// masterSpy to use to watch the first orderer for
			// deliveries, on all channels. This will be a duplicate
			// Consumer (it is the second one monitoring the first
			// orderer), so we need to reuse the first port.
			serverAddr = fmt.Sprintf("%s:%d", ordConf.General.ListenAddress, ordStartPort)
			go startConsumerMaster(serverAddr, &channelIDs, ord, &(txRecv[ord]), &(blockRecv[ord]), &(consumerConns[ord][0]))
		} else if optimizeClientsMode {
			// Create just one Consumer to receive all deliveries
			// (on all channels) on an orderer.
			go startConsumerMaster(serverAddr, &channelIDs, ord, &(txRecv[ord]), &(blockRecv[ord]), &(consumerConns[ord][0]))
		} else {
			// Normal mode: create a unique consumer client
			// go-thread for each channel on each orderer.
			for c := 0; c < numChannels; c++ {
				go startConsumer(serverAddr, channelIDs[c], ord, c, &(txRecv[ord][c]), &(blockRecv[ord][c]), &(consumerConns[ord][c]))
			}
		}

	}

	logger("Finished creating all CONSUMERS clients")
	time.Sleep(5 * time.Second)
	defer cleanNetwork(&consumerConns)

	////////////////////////////////////////////////////////////////////////
	// Now that the orderer service network is running, and the consumers
	// are watching for deliveries, we can start clients which will
	// broadcast the specified number of TXs to their associated orderers.

	if optimizeClientsMode {
		producersWG.Add(numOrdsInNtwk)
	} else {
		producersWG.Add(numProducers)
	}
	sendStart := time.Now().Unix()
	for ord := 0; ord < numOrdsInNtwk; ord++ {
		serverAddr := fmt.Sprintf("%s:%d", ordConf.General.ListenAddress, ordStartPort+uint16(ord))
		for c := 0; c < numChannels; c++ {
			countToSend[ord][c] = numTxToSend / int64(numOrdsInNtwk*numChannels)
			if c == 0 && ord == 0 {
				countToSend[ord][c] += numTxToSend % int64(numOrdsInNtwk*numChannels)
			}
		}
		if optimizeClientsMode {
			// create one Producer for all channels on this orderer
			go startProducerMaster(serverAddr, &channelIDs, ord, &(countToSend[ord]), &(txSent[ord]), &(txSentFailures[ord]))
		} else {
			// Normal mode: create a unique consumer client
			// go thread for each channel
			for c := 0; c < numChannels; c++ {
				go startProducer(serverAddr, channelIDs[c], ord, c, countToSend[ord][c], &(txSent[ord][c]), &(txSentFailures[ord][c]))
			}
		}
	}

	if optimizeClientsMode {
		logger(fmt.Sprintf("Finished creating all %d MASTER-PRODUCERs", numOrdsInNtwk))
	} else {
		logger(fmt.Sprintf("Finished creating all %d PRODUCERs", numOrdsInNtwk*numChannels))
	}
	producersWG.Wait()
	logger(fmt.Sprintf("Send Duration (seconds): %4d", time.Now().Unix()-sendStart))
	recoverStart := time.Now().Unix()

	////////////////////////////////////////////////////////////////////////
	// All producer threads are finished sending broadcast transactions.
	// Let's determine if the deliveries have all been received by the
	// consumer threads. We will check if the receive counts match the send
	// counts on all consumers, or if all consumers are no longer receiving
	// blocks. Wait and continue rechecking as necessary, as long as the
	// delivery (recv) counters are climbing closer to the broadcast (send)
	// counter. If the counts do not match, wait for up to batchTimeout
	// seconds, to ensure that we received the last (non-full) batch.

	computeTotals(&txSent, &totalNumTxSent, &txSentFailures, &totalNumTxSentFailures, &txRecv, &totalTxRecv, &totalTxRecvMismatch, &blockRecv, &totalBlockRecv, &totalBlockRecvMismatch)

	waitSecs := 0
	for !sendEqualRecv(numTxToSend, &totalTxRecv, totalTxRecvMismatch, totalBlockRecvMismatch) && (moreDeliveries(&txSent, &totalNumTxSent, &txSentFailures, &totalNumTxSentFailures, &txRecv, &totalTxRecv, &totalTxRecvMismatch, &blockRecv, &totalBlockRecv, &totalBlockRecvMismatch) || waitSecs < batchTimeout) {
		time.Sleep(1 * time.Second)
		waitSecs++
	}

	// Recovery Duration = time spent waiting for orderer service to finish delivering transactions,
	// after all producers finished sending them.
	// waitSecs = some possibly idle time spent waiting for the last batch to be generated (waiting for batchTimeout)
	logger(fmt.Sprintf("Recovery Duration (secs):%4d", time.Now().Unix()-recoverStart))
	logger(fmt.Sprintf("waitSecs for last batch: %4d", waitSecs))
	passed, resultSummary = reportTotals(testname, numTxToSend, countToSend, txSent, totalNumTxSent, txSentFailures, totalNumTxSentFailures, batchSize, txRecv, totalTxRecv, totalTxRecvMismatch, blockRecv, totalBlockRecv, totalBlockRecvMismatch, masterSpy, &channelIDs)

	return passed, resultSummary
}

func main() {

	initialize()

	// Set reasonable defaults in case any env vars are unset.
	var txs int64 = 55
	chans := numChannels
	orderers := numOrdsInNtwk
	ordType := ordererType
	kbs := numKBrokers

	// Set addMasterSpy to true to create one additional consumer client
	// that monitors all channels on one orderer with one grpc connection.
	addMasterSpy := false

	pPerCh := producersPerCh
	// TODO lPerCh := listenersPerCh

	// Read env vars
	if debugflagAPI {
		logger("==========Environment variables provided for this test, and corresponding values actually used for the test:")
	}
	testcmd := ""
	envvar := os.Getenv("OTE_TXS")
	if envvar != "" {
		txs, _ = strconv.ParseInt(envvar, 10, 64)
		testcmd += " OTE_TXS=" + envvar
	}
	if debugflagAPI {
		logger(fmt.Sprintf("%-50s %s=%d", "OTE_TXS="+envvar, "txs", txs))
	}

	envvar = os.Getenv("OTE_CHANNELS")
	if envvar != "" {
		chans, _ = strconv.Atoi(envvar)
		testcmd += " OTE_CHANNELS=" + envvar
	}
	if debugflagAPI {
		logger(fmt.Sprintf("%-50s %s=%d", "OTE_CHANNELS="+envvar, "chans", chans))
	}

	envvar = os.Getenv("OTE_ORDERERS")
	if envvar != "" {
		orderers, _ = strconv.Atoi(envvar)
		testcmd += " OTE_ORDERERS=" + envvar
	}
	if debugflagAPI {
		logger(fmt.Sprintf("%-50s %s=%d", "OTE_ORDERERS="+envvar, "orderers", orderers))
	}

	envvar = os.Getenv(ordererTypeParamStr)
	if envvar != "" {
		ordType = envvar
		testcmd += " " + ordererTypeParamStr + "=" + envvar
	}
	if debugflagAPI {
		logger(fmt.Sprintf("%-50s %s=%s", ordererTypeParamStr+"="+envvar, "ordType", ordType))
	}

	envvar = os.Getenv("OTE_KAFKABROKERS")
	if envvar != "" {
		kbs, _ = strconv.Atoi(envvar)
		testcmd += " OTE_KAFKABROKERS=" + envvar
	}
	if debugflagAPI {
		logger(fmt.Sprintf("%-50s %s=%d", "OTE_KAFKABROKERS="+envvar, "kbs", kbs))
	}

	envvar = os.Getenv("OTE_MASTERSPY")
	if "true" == strings.ToLower(envvar) || "t" == strings.ToLower(envvar) {
		addMasterSpy = true
		testcmd += " OTE_MASTERSPY=" + envvar
	}
	if debugflagAPI {
		logger(fmt.Sprintf("%-50s %s=%t", "OTE_MASTERSPY="+envvar, "masterSpy", addMasterSpy))
	}

	envvar = os.Getenv("OTE_PRODUCERS_PER_CHANNEL")
	if envvar != "" {
		pPerCh, _ = strconv.Atoi(envvar)
		testcmd += " OTE_PRODUCERS_PER_CHANNEL=" + envvar
	}
	if debugflagAPI {
		logger(fmt.Sprintf("%-50s %s=%d", "OTE_PRODUCERS_PER_CHANNEL="+envvar, "producersPerCh", pPerCh))
	}

	_, _ = ote("<commandline>"+testcmd+" ote", txs, chans, orderers, ordType, kbs, addMasterSpy, pPerCh)
}
