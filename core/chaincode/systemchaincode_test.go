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

package chaincode

import (
	"net"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/system_chaincode/samplesyscc"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// Test deploy of a transaction.
func TestExecuteDeploySysChaincode(t *testing.T) {
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/tmpdb")
	kvledger.Initialize("/var/hyperledger/test/tmpdb")

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:21726"
	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(5000) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	var ctxt = context.Background()

	//set systemChaincodes to sample
	systemChaincodes = []*SystemChaincode{
		{
			Enabled:   true,
			Name:      "sample_syscc",
			Path:      "github.com/hyperledger/fabric/core/system_chaincode/samplesyscc",
			InitArgs:  [][]byte{},
			Chaincode: &samplesyscc.SampleSysCC{},
		},
	}

	// System chaincode has to be enabled
	viper.Set("chaincode.system", map[string]string{"sample_syscc": "true"})
	RegisterSysCCs()

	url := "github.com/hyperledger/fabric/core/system_chaincode/sample_syscc"
	f := "putval"
	args := util.ToChaincodeArgs(f, "greeting", "hey there")

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: "sample_syscc", Path: url}, CtorMsg: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(ctxt, spec)
	if err != nil {
		closeListenerAndSleep(lis)
		t.Fail()
		t.Logf("Error invoking sample_syscc: %s", err)
		return
	}

	f = "getval"
	args = util.ToChaincodeArgs(f, "greeting")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: "sample_syscc", Path: url}, CtorMsg: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(ctxt, spec)
	if err != nil {
		closeListenerAndSleep(lis)
		t.Fail()
		t.Logf("Error invoking sample_syscc: %s", err)
		return
	}

	cds := &pb.ChaincodeDeploymentSpec{ExecEnv: 1, ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: "sample_syscc", Path: url}, CtorMsg: &pb.ChaincodeInput{Args: args}}}

	GetChain(DefaultChain).Stop(ctxt, cds)

	closeListenerAndSleep(lis)
}
