// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"google.golang.org/grpc"

	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
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
var genConf *genesisconfig.Profile

func init() {
	var err error
	conf, err = localconfig.Load()
	if err != nil {
		fmt.Println("failed to load config:", err)
		os.Exit(1)
	}

	// Load local MSP
	err = mspmgmt.LoadLocalMsp(conf.General.LocalMSPDir, conf.General.BCCSP, conf.General.LocalMSPID)
	if err != nil {
		panic(fmt.Errorf("Failed to initialize local MSP: %s", err))
	}

	genConf = genesisconfig.Load(conf.General.GenesisProfile)
}

func main() {
	cmd := new(cmdImpl)
	var srv string

	flag.StringVar(&srv, "server", fmt.Sprintf("%s:%d", conf.General.ListenAddress, conf.General.ListenPort), "The RPC server to connect to.")
	flag.StringVar(&cmd.name, "cmd", "newChain", "The action that this client is requesting via the config transaction.")
	flag.StringVar(&cmd.args.consensusType, "consensusType", genConf.Orderer.OrdererType, "In case of a newChain command, the type of consensus the ordering service is running on.")
	flag.StringVar(&cmd.args.creationPolicy, "creationPolicy", "AcceptAllPolicy", "In case of a newChain command, the chain creation policy this request should be validated against.")
	flag.StringVar(&cmd.args.chainID, "chainID", "mychannel", "In case of a newChain command, the chain ID to create.")
	flag.Parse()

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
		env := newChainRequest(cmd.args.consensusType, cmd.args.creationPolicy, cmd.args.chainID)
		fmt.Println("Requesting the creation of chain", cmd.args.chainID)
		fmt.Println(bc.broadcast(env))
	default:
		panic("Invalid command given")
	}
}
