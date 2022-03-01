// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"

	pb "github.com/cheggaaa/pb"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/grpc"
)

type broadcastClient struct {
	client    ab.AtomicBroadcast_BroadcastClient
	signer    identity.SignerSerializer
	channelID string
}

// newBroadcastClient creates a simple instance of the broadcastClient interface
func newBroadcastClient(client ab.AtomicBroadcast_BroadcastClient, channelID string, signer identity.SignerSerializer) *broadcastClient {
	return &broadcastClient{client: client, channelID: channelID, signer: signer}
}

func (s *broadcastClient) broadcast(transaction []byte) error {
	env, err := protoutil.CreateSignedEnvelope(cb.HeaderType_MESSAGE, s.channelID, s.signer, &cb.ConfigValue{Value: transaction}, 0, 0)
	if err != nil {
		panic(err)
	}
	return s.client.Send(env)
}

func (s *broadcastClient) getAck() error {
	msg, err := s.client.Recv()
	if err != nil {
		return err
	}
	if msg.Status != cb.Status_SUCCESS {
		return fmt.Errorf("got unexpected status: %v - %s", msg.Status, msg.Info)
	}
	return nil
}

func main() {
	conf, err := localconfig.Load()
	if err != nil {
		fmt.Println("failed to load config:", err)
		os.Exit(1)
	}

	// Load local MSP
	mspConfig, err := msp.GetLocalMspConfig(conf.General.LocalMSPDir, conf.General.BCCSP, conf.General.LocalMSPID)
	if err != nil {
		fmt.Println("Failed to load MSP config:", err)
		os.Exit(0)
	}
	err = mspmgmt.GetLocalMSP(factory.GetDefault()).Setup(mspConfig)
	if err != nil { // Handle errors reading the config file
		fmt.Println("Failed to initialize local MSP:", err)
		os.Exit(0)
	}

	signer, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	if err != nil {
		fmt.Println("Failed to load local signing identity:", err)
		os.Exit(0)
	}

	var channelID string
	var serverAddr string
	var messages uint64
	var goroutines uint64
	var msgSize uint64
	var bar *pb.ProgressBar

	flag.StringVar(&serverAddr, "server", fmt.Sprintf("%s:%d", conf.General.ListenAddress, conf.General.ListenPort), "The RPC server to connect to.")
	flag.StringVar(&channelID, "channelID", "mychannel", "The channel ID to broadcast to.")
	flag.Uint64Var(&messages, "messages", 1, "The number of messages to broadcast.")
	flag.Uint64Var(&goroutines, "goroutines", 1, "The number of concurrent go routines to broadcast the messages on")
	flag.Uint64Var(&msgSize, "size", 1024, "The size in bytes of the data section for the payload")
	flag.Parse()

	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
	defer func() {
		_ = conn.Close()
	}()
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}

	msgsPerGo := messages / goroutines
	roundMsgs := msgsPerGo * goroutines
	if roundMsgs != messages {
		fmt.Println("Rounding messages to", roundMsgs)
	}
	bar = pb.New64(int64(roundMsgs))
	bar.ShowPercent = true
	bar.ShowSpeed = true
	bar = bar.Start()

	msgData := make([]byte, msgSize)

	var wg sync.WaitGroup
	wg.Add(int(goroutines))
	for i := uint64(0); i < goroutines; i++ {
		go func(i uint64, pb *pb.ProgressBar) {
			client, err := ab.NewAtomicBroadcastClient(conn).Broadcast(context.TODO())
			if err != nil {
				fmt.Println("Error connecting:", err)
				return
			}

			s := newBroadcastClient(client, channelID, signer)
			done := make(chan (struct{}))
			go func() {
				for i := uint64(0); i < msgsPerGo; i++ {
					err = s.getAck()
					if err == nil && bar != nil {
						bar.Increment()
					}
				}
				if err != nil {
					fmt.Printf("\nError: %v\n", err)
				}
				close(done)
			}()
			for i := uint64(0); i < msgsPerGo; i++ {
				if err := s.broadcast(msgData); err != nil {
					panic(err)
				}
			}
			<-done
			wg.Done()
			client.CloseSend()
		}(i, bar)
	}

	wg.Wait()
	bar.FinishPrint("----------------------broadcast message finish-------------------------------")
}
