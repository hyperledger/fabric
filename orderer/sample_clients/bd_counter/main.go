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
	"os/signal"
	"strings"

	ab "github.com/hyperledger/fabric/protos/orderer"
	logging "github.com/op/go-logging"
	"google.golang.org/grpc"
)

var logger *logging.Logger

type configImpl struct {
	logLevel                 logging.Level
	rpc, server              string
	count, seek, window, ack int
}

type clientImpl struct {
	config     configImpl
	rpc        ab.AtomicBroadcastClient
	signalChan chan os.Signal
}

func main() {
	var loglevel string
	client := &clientImpl{}

	backend := logging.NewLogBackend(os.Stderr, "", 0)
	logging.SetBackend(backend)
	formatter := logging.MustStringFormatter("[%{time:15:04:05}] %{shortfile:18s}: %{color}[%{level:-5s}]%{color:reset} %{message}")
	logging.SetFormatter(formatter)
	logger = logging.MustGetLogger("orderer/bd_counter")

	flag.StringVar(&client.config.rpc, "rpc", "broadcast",
		"The RPC that this client is requesting.")
	flag.StringVar(&client.config.server, "server",
		"127.0.0.1:7050", "The RPC server to connect to.")
	flag.IntVar(&client.config.count, "count", 100,
		"When in broadcast mode, how many messages to send.")
	flag.StringVar(&loglevel, "loglevel", "info",
		"The logging level. (Suggested values: info, debug)")
	flag.IntVar(&client.config.seek, "seek", -2,
		"When in deliver mode, the number of the first block that should be delivered (-2 for oldest available, -1 for newest).")
	flag.IntVar(&client.config.window, "window", 10,
		"When in deliver mode, how many blocks can the server send without acknowledgement.")
	flag.IntVar(&client.config.ack, "ack", 7,
		"When in deliver mode, send acknowledgment per this many blocks received.")
	flag.Parse() // TODO Validate user input (e.g. ack should be =< window)

	client.config.logLevel, _ = logging.LogLevel(strings.ToUpper(loglevel))
	logging.SetLevel(client.config.logLevel, logger.Module)

	// Trap SIGINT to trigger a shutdown
	// We must use a buffered channel or risk missing the signal
	// if we're not ready to receive when the signal is sent.
	client.signalChan = make(chan os.Signal, 1)
	signal.Notify(client.signalChan, os.Interrupt)

	conn, err := grpc.Dial(client.config.server, grpc.WithInsecure())
	if err != nil {
		logger.Fatalf("Client did not connect to %s: %v\n", client.config.server, err)
	}
	defer conn.Close()
	client.rpc = ab.NewAtomicBroadcastClient(conn)

	switch client.config.rpc {
	case "broadcast":
		client.broadcast()
	case "deliver":
		client.deliver()
	}
}
