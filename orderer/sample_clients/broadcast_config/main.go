/*
Copyright IBM Corp. 2016 All Rights Reserved.

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
	"flag"
	"os"
	"strings"

	ab "github.com/hyperledger/fabric/protos/orderer"
	logging "github.com/op/go-logging"
	"google.golang.org/grpc"
)

const pkgName = "orderer/broadcast_config"

var logger *logging.Logger

// Include here all the possible arguments for a command
type argsImpl struct {
	creationPolicy string
	chainID        string
}

// This holds the command and its arguments
type cmdImpl struct {
	cmd  string
	args argsImpl
}

type configImpl struct {
	logLevel logging.Level
	server   string
	cmd      cmdImpl
}

type clientImpl struct {
	config   configImpl
	rpc      ab.AtomicBroadcastClient
	doneChan chan struct{}
}

func main() {
	var loglevel string

	client := &clientImpl{doneChan: make(chan struct{})}

	backend := logging.NewLogBackend(os.Stderr, "", 0)
	logging.SetBackend(backend)
	formatter := logging.MustStringFormatter("[%{time:15:04:05}] %{shortfile:18s}: %{color}[%{level:-5s}]%{color:reset} %{message}")
	logging.SetFormatter(formatter)
	logger = logging.MustGetLogger(pkgName)

	flag.StringVar(&loglevel, "loglevel", "info",
		"The logging level. (Suggested values: info, debug)")
	flag.StringVar(&client.config.server, "server",
		"127.0.0.1:7050", "The RPC server to connect to.")
	flag.StringVar(&client.config.cmd.cmd, "cmd", "new-chain",
		"The action that this client is requesting via the config transaction.")
	flag.StringVar(&client.config.cmd.args.creationPolicy, "creationPolicy", "AcceptAllPolicy",
		"In case of a new-chain command, the chain createion policy this request should be validated against.")
	flag.StringVar(&client.config.cmd.args.chainID, "chainID", "NewChainID",
		"In case of a new-chain command, the chain ID to create.")
	flag.Parse()

	client.config.logLevel, _ = logging.LogLevel(strings.ToUpper(loglevel))
	logging.SetLevel(client.config.logLevel, logger.Module)

	conn, err := grpc.Dial(client.config.server, grpc.WithInsecure())
	if err != nil {
		logger.Fatalf("Client did not connect to %s: %v", client.config.server, err)
	}
	defer conn.Close()
	client.rpc = ab.NewAtomicBroadcastClient(conn)

	switch client.config.cmd.cmd {
	case "new-chain":
		envelope := newChainRequest(client.config.cmd.args.creationPolicy, client.config.cmd.args.chainID)
		logger.Infof("Requesting the creation of chain \"%s\"", client.config.cmd.args.chainID)
		client.broadcast(envelope)
	default:
		panic("Invalid cmd given")
	}
}
