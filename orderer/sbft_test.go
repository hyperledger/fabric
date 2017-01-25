/*
Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"net"

	"bytes"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/localmsp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	"github.com/hyperledger/fabric/orderer/ledger"
	"github.com/hyperledger/fabric/orderer/ledger/ram"
	"github.com/hyperledger/fabric/orderer/localconfig"
	"github.com/hyperledger/fabric/orderer/multichain"
	"github.com/hyperledger/fabric/orderer/sbft"
	"github.com/hyperledger/fabric/orderer/sbft/backend"
	"github.com/hyperledger/fabric/orderer/sbft/crypto"
	"github.com/hyperledger/fabric/orderer/sbft/simplebft"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const update byte = 0
const sent byte = 1

const neededUpdates = 2
const neededSent = 1

var testData = []byte{0, 1, 2, 3}

const sbftName = "sbft"

type item struct {
	itemtype byte
	payload  []byte
}

func TestSbftPeer(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	t.Parallel()
	skipInShortMode(t)
	logging.SetLevel(logging.DEBUG, "")

	// Start SBFT
	dataTmpDir, err := ioutil.TempDir("", "sbft_test")
	if err != nil {
		panic("Failed to create a temporary directory")
	}
	// We only need the path as the directory will be created
	// by the peer
	os.RemoveAll(dataTmpDir)
	defer func() {
		os.RemoveAll(dataTmpDir)
	}()
	peers := make(map[string][]byte)
	peers["6101"], err = crypto.ParseCertPEM("sbft/testdata/cert1.pem")
	panicOnError(err)
	listenAddr := ":6101"
	certFile := "sbft/testdata/cert1.pem"
	keyFile := "sbft/testdata/key.pem"
	cons := &simplebft.Config{N: 1, F: 0, BatchDurationNsec: 1000, BatchSizeBytes: 1000000000, RequestTimeoutNsec: 1000000000}
	c := &sbft.ConsensusConfig{Consensus: cons, Peers: peers}
	sc := &backend.StackConfig{listenAddr, certFile, keyFile, dataTmpDir}
	sbftConsenter := sbft.New(c, sc)
	<-time.After(5 * time.Second)
	// End SBFT

	// Start GRPC
	logger.Info("Creating a GRPC server.")
	conf := config.Load()
	conf.Genesis.OrdererType = sbftName
	conf.General.LocalMSPDir = pwd + "/../msp/sampleconfig"
	lf := newRAMLedgerFactory(conf)
	consenters := make(map[string]multichain.Consenter)
	consenters[sbftName] = sbftConsenter

	err = mspmgmt.LoadLocalMsp(conf.General.LocalMSPDir)
	if err != nil { // Handle errors reading the config file
		panic(fmt.Errorf("Failed initializing crypto [%s]", err))
	}
	signer := localmsp.NewSigner()
	manager := multichain.NewManagerImpl(lf, consenters, signer)

	server := NewServer(manager, int(conf.General.QueueSize), int(conf.General.MaxWindowSize))
	grpcServer := grpc.NewServer()
	grpcAddr := fmt.Sprintf("%s:%d", conf.General.ListenAddress, conf.General.ListenPort)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		panic("Listening on the given port failed.")
	}
	ab.RegisterAtomicBroadcastServer(grpcServer, server)
	go grpcServer.Serve(lis)
	// End GRPC

	// Start Test Setup
	logger.Info("Creating an Atomic Broadcast GRPC connection.")
	timeout := 4 * time.Second
	clientconn, err := grpc.Dial(grpcAddr, grpc.WithBlock(), grpc.WithTimeout(timeout), grpc.WithInsecure())
	if err != nil {
		t.Errorf("Failed to connect to GRPC: %s", err)
		return
	}
	client := ab.NewAtomicBroadcastClient(clientconn)

	<-time.After(4 * time.Second)

	resultch := make(chan item)
	errorch := make(chan error)

	logger.Info("Starting a goroutine waiting for ledger updates.")
	go updateReceiver(t, resultch, errorch, client)

	logger.Info("Starting a single broadcast sender goroutine.")
	go broadcastSender(t, resultch, errorch, client)
	// End Test Setup

	checkResults(t, resultch, errorch)
}

func checkResults(t *testing.T, resultch chan item, errorch chan error) {
	l := len(errorch)
	for i := 0; i < l; i++ {
		errres := <-errorch
		t.Error(errres)
	}

	updates := 0
	sentBroadcast := 0
	testDataReceived := false
	for i := 0; i < neededUpdates+neededSent; i++ {
		select {
		case result := <-resultch:
			switch result.itemtype {
			case update:
				updates++
				if bytes.Equal(result.payload, testData) {
					testDataReceived = true
				}
			case sent:
				sentBroadcast++
			}
		case <-time.After(30 * time.Second):
			continue
		}
	}
	if updates != neededUpdates {
		t.Errorf("We did not get all the ledger updates.")
	} else if sentBroadcast != neededSent {
		t.Errorf("We were unable to send all the broadcasts.")
	} else if !testDataReceived {
		t.Errorf("We did not receive an update containing the test data sent in a broadcast.")
	} else {
		logger.Info("Successfully sent and received everything.")
	}
}

func updateReceiver(t *testing.T, resultch chan item, errorch chan error, client ab.AtomicBroadcastClient) {
	logger.Info("{Update Receiver} Creating a ledger update delivery stream.")
	dstream, err := client.Deliver(context.Background())
	if err != nil {
		errorch <- fmt.Errorf("Failed to get Deliver stream: %s", err)
		return
	}
	err = dstream.Send(&cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChainHeader: &cb.ChainHeader{
					ChainID: provisional.TestChainID,
				},
				SignatureHeader: &cb.SignatureHeader{},
			},
			Data: utils.MarshalOrPanic(&ab.SeekInfo{
				Start:    &ab.SeekPosition{Type: &ab.SeekPosition_Newest{Newest: &ab.SeekNewest{}}},
				Stop:     &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: ^uint64(0)}}},
				Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
			}),
		}),
	})
	if err != nil {
		errorch <- fmt.Errorf("Failed to send to Deliver stream: %s", err)
		return
	}
	logger.Info("{Update Receiver} Listening to ledger updates.")
	for i := 0; i < neededUpdates; {
		m, inerr := dstream.Recv()
		logger.Info("{Update Receiver} Got message: ", m, "err:", inerr)
		if inerr != nil {
			errorch <- fmt.Errorf("Failed to receive consensus: %s", inerr)
			return
		}
		b, ok := m.Type.(*ab.DeliverResponse_Block)
		if !ok {
			logger.Info("{Update Receiver} Received s status message.")
			continue
		}
		logger.Info("{Update Receiver} Received a ledger update.")
		for i, tx := range b.Block.Data.Data {
			pl := &cb.Payload{}
			e := &cb.Envelope{}
			merr1 := proto.Unmarshal(tx, e)
			merr2 := proto.Unmarshal(e.Payload, pl)
			if merr1 == nil && merr2 == nil {
				logger.Infof("{Update Receiver} %d - %v", i+1, pl.Data)
				resultch <- item{itemtype: update, payload: pl.Data}
			}
		}
		i++
	}
	logger.Info("{Update Receiver} Exiting...")
}

func broadcastSender(t *testing.T, resultch chan item, errorch chan error, client ab.AtomicBroadcastClient) {
	logger.Info("{Broadcast Sender} Waiting before sending.")
	<-time.After(5 * time.Second)
	bstream, err := client.Broadcast(context.Background())
	if err != nil {
		errorch <- fmt.Errorf("Failed to get broadcast stream: %s", err)
		return
	}
	h := &cb.Header{ChainHeader: &cb.ChainHeader{ChainID: provisional.TestChainID}, SignatureHeader: &cb.SignatureHeader{}}
	bs := testData
	pl := &cb.Payload{Data: bs, Header: h}
	mpl, err := proto.Marshal(pl)
	if err != nil {
		panic("Failed to marshal payload.")
	}
	bstream.Send(&cb.Envelope{Payload: mpl})
	logger.Infof("{Broadcast Sender} Broadcast sent: %v", bs)
	logger.Info("{Broadcast Sender} Exiting...")
	resultch <- item{itemtype: sent, payload: mpl}
}

func newRAMLedgerFactory(conf *config.TopLevel) ordererledger.Factory {
	rlf := ramledger.New(10)
	genesisBlock := provisional.New(conf).GenesisBlock()
	rl, err := rlf.GetOrCreate(provisional.TestChainID)
	if err != nil {
		panic(err)
	}
	err = rl.Append(genesisBlock)
	if err != nil {
		panic(err)
	}
	return rlf
}
