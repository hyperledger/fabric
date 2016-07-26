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

package genesis

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos"
	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	setupTestConfig()
	os.Exit(m.Run())
}

func TestGenesis(t *testing.T) {

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:50303"

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)

	getPeerEndpoint := func() (*protos.PeerEndpoint, error) {
		return &protos.PeerEndpoint{ID: &protos.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(30000) * time.Millisecond
	protos.RegisterChaincodeSupportServer(grpcServer, chaincode.NewChaincodeSupport(chaincode.DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	ledger := ledger.InitTestLedger(t)

	if ledger.GetBlockchainSize() != 0 {
		t.Fatalf("Expected blockchain size of 0, but got %d", ledger.GetBlockchainSize())
	}

	makeGenesisErr := MakeGenesis()
	if makeGenesisErr != nil {
		t.Fatalf("Error creating genesis block, %s", makeGenesisErr)
	}
	if ledger.GetBlockchainSize() != 1 {
		t.Fatalf("Expected blockchain size of 1, but got %d", ledger.GetBlockchainSize())
	}
}

func setupTestConfig() {
	viper.AddConfigPath(".")
	viper.SetConfigName("genesis_test")
	viper.Set("ledger.blockchain.deploy-system-chaincode", "false")
	err := viper.ReadInConfig()
	if err != nil { // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
}
