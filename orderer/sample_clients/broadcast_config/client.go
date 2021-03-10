// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"google.golang.org/grpc"
)

type broadcastClient struct {
	ab.AtomicBroadcast_BroadcastClient
}

func (bc *broadcastClient) broadcast(env *cb.Envelope) error {
	var err error
	var resp *ab.BroadcastResponse

	err = bc.Send(env)
	if err != nil {
		return err
	}

	resp, err = bc.Recv()
	if err != nil {
		return err
	}

	fmt.Println("Status:", resp)
	return nil
}

// cmdImpl holds the command and its arguments.
type cmdImpl struct {
	name string
	args argsImpl
}

// argsImpl holds all the possible arguments for all possible commands.
type argsImpl struct {
	consensusType  string
	creationPolicy string
	chainID        string
}

var conf *localconfig.TopLevel

func init() {
	var err error
	conf, err = localconfig.Load()
	if err != nil {
		fmt.Println("failed to load config:", err)
		os.Exit(1)
	}

	// Load local MSP
	mspConfig, err := msp.GetLocalMspConfig(conf.General.LocalMSPDir, conf.General.BCCSP, conf.General.LocalMSPID)
	if err != nil {
		panic(fmt.Errorf("Failed to load MSP config: %s", err))
	}
	err = mspmgmt.GetLocalMSP(factory.GetDefault()).Setup(mspConfig)
	if err != nil {
		panic(fmt.Errorf("failed to initialize local MSP: %s", err))
	}
}

func main() {
	cmd := new(cmdImpl)
	var srv string

	flag.StringVar(&srv, "server", fmt.Sprintf("%s:%d", conf.General.ListenAddress, conf.General.ListenPort), "The RPC server to connect to.")
	flag.StringVar(&cmd.name, "cmd", "newChain", "The action that this client is requesting via the config transaction.")
	flag.StringVar(&cmd.args.consensusType, "consensusType", "solo", "In case of a newChain command, the type of consensus the ordering service is running on.")
	flag.StringVar(&cmd.args.creationPolicy, "creationPolicy", "AcceptAllPolicy", "In case of a newChain command, the chain creation policy this request should be validated against.")
	flag.StringVar(&cmd.args.chainID, "chainID", "mychannel", "In case of a newChain command, the chain ID to create.")
	flag.Parse()

	signer, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	if err != nil {
		fmt.Println("Failed to load local signing identity:", err)
		os.Exit(0)
	}

	conn, err := grpc.Dial(srv, grpc.WithInsecure())
	defer func() {
		_ = conn.Close()
	}()
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}

	client, err := ab.NewAtomicBroadcastClient(conn).Broadcast(context.TODO())
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}

	bc := &broadcastClient{client}

	switch cmd.name {
	case "newChain":
		env := newChainRequest(
			cmd.args.consensusType,
			cmd.args.creationPolicy,
			cmd.args.chainID,
			signer,
		)
		fmt.Println("Requesting the creation of chain", cmd.args.chainID)
		fmt.Println(bc.broadcast(env))
	default:
		panic("Invalid command given")
	}
}
