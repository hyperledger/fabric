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

package sbft

import (
	"flag"
	"fmt"
	"net"
	_ "net/http/pprof"
	"os"

	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
	"github.com/hyperledger/fabric/orderer/rawledger/fileledger"
	"github.com/hyperledger/fabric/orderer/sbft/backend"
	"github.com/hyperledger/fabric/orderer/sbft/connection"
	"github.com/hyperledger/fabric/orderer/sbft/persist"
	pb "github.com/hyperledger/fabric/orderer/sbft/simplebft"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"
	"google.golang.org/grpc"
)

type consensusStack struct {
	persist *persist.Persist
	backend *backend.Backend
}

type flags struct {
	listenAddr    string
	grpcAddr      string
	telemetryAddr string
	certFile      string
	keyFile       string
	dataDir       string
	verbose       string
	init          string
}

func main() {
	var c flags

	flag.StringVar(&c.init, "init", "", "initialized instance from pbft config `file`")
	flag.StringVar(&c.listenAddr, "addr", ":6100", "`addr`ess/port of service")
	flag.StringVar(&c.grpcAddr, "gaddr", ":7100", "`addr`ess/port of GRPC atomic broadcast server")
	flag.StringVar(&c.telemetryAddr, "telemetry", ":7100", "`addr`ess of telemetry/profiler")
	flag.StringVar(&c.certFile, "cert", "", "certificate `file`")
	flag.StringVar(&c.keyFile, "key", "", "key `file`")
	flag.StringVar(&c.dataDir, "data-dir", "", "data `dir`ectory")
	flag.StringVar(&c.verbose, "verbose", "info", "set verbosity `level` (critical, error, warning, notice, info, debug)")

	flag.Parse()

	level, err := logging.LogLevel(c.verbose)
	if err != nil {
		panic(err)
	}
	logging.SetLevel(level, "")

	if c.init != "" {
		err = initInstance(c)
		if err != nil {
			panic(err)
		}
		return
	}

	serve(c)
}

func initInstance(c flags) error {
	config, err := ReadJsonConfig(c.init)
	if err != nil {
		return err
	}

	err = os.Mkdir(c.dataDir, 0755)
	if err != nil {
		return err
	}

	p := persist.New(c.dataDir)
	err = SaveConfig(p, config)
	if err != nil {
		return err
	}

	fmt.Println("initialized new peer")
	return nil
}

func serve(c flags) {
	if c.dataDir == "" {
		fmt.Fprintln(os.Stderr, "need data directory")
		os.Exit(1)
	}

	persist := persist.New(c.dataDir)
	config, err := RestoreConfig(persist)
	if err != nil {
		panic(err)
	}

	conn, err := connection.New(c.listenAddr, c.certFile, c.keyFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Connection error.")
		panic(err)
	}
	s := &consensusStack{
		persist: nil,
	}
	genesisBlock, err := static.New().GenesisBlock()
	if err != nil {
		panic(err)
	}

	_, ledger := fileledger.New(c.dataDir, genesisBlock)
	s.backend, err = backend.NewBackend(config.Peers, conn, ledger, persist)
	if err != nil {
		panic(err)
	}

	sbft, _ := pb.New(s.backend.GetMyId(), config.Consensus, s.backend)
	s.backend.SetReceiver(sbft)

	grpcServer := grpc.NewServer()
	lis, err := net.Listen("tcp", c.grpcAddr)
	if err != nil {
		panic(fmt.Sprintf("Failed to listen: %s", err))
	}
	broadcastab := backend.NewBackendAB(s.backend)
	ab.RegisterAtomicBroadcastServer(grpcServer, broadcastab)
	grpcServer.Serve(lis)

	// block forever
	select {}
}
