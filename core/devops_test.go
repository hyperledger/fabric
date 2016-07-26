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

package core

import (
	"testing"

	"golang.org/x/net/context"

	pb "github.com/hyperledger/fabric/protos"
)

func TestDevops_Build_NilSpec(t *testing.T) {
	t.Skip("Skipping until we have the Validator system setup properly for testing.")
	// TODO Cannot pass in nil to NewDevopsServer
	devopsServer := NewDevopsServer(nil)

	_, err := devopsServer.Build(context.Background(), nil)
	if err == nil {
		t.Fail()
		t.Log("Expected error in Devops.Build call with 'nil' spec:")
	}
	t.Logf("Got expected err: %s", err)
	//performHandshake(t, peerClientConn)
}

func TestDevops_Build(t *testing.T) {
	t.Skip("Skipping until we have the Validator system setup properly for testing.")
	// TODO Cannot pass in nil to NewDevopsServer
	devopsServer := NewDevopsServer(nil)

	// Build the spec
	chaincodePath := "github.com/hyperledger/fabric/core/example/chaincode/chaincode_example01"
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeID: &pb.ChaincodeID{Path: chaincodePath}}

	buildResult, err := devopsServer.Build(context.Background(), spec)
	if err != nil {
		t.Fail()
		t.Logf("Error in Devops.Build call: %s", err)
	}
	t.Logf("Build result = %s", buildResult.ChaincodeSpec.ChaincodeID)
	//performHandshake(t, peerClientConn)
}

func TestDevops_Deploy(t *testing.T) {
	t.Skip("Skipping until we have the Validator system setup properly for testing.")
	// TODO Cannot pass in nil to NewDevopsServer
	devopsServer := NewDevopsServer(nil)

	// Build the spec
	chaincodePath := "github.com/hyperledger/fabric/core/example/chaincode/chaincode_example01"
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeID: &pb.ChaincodeID{Path: chaincodePath}}

	buildResult, err := devopsServer.Deploy(context.Background(), spec)
	if err != nil {
		t.Fail()
		t.Logf("Error in Devops.Build call: %s", err)
	}
	t.Logf("Deploy result = %s", buildResult.ChaincodeSpec)
	//performHandshake(t, peerClientConn)
}

func TestDevops_Spec_NoVersion(t *testing.T) {
	t.Skip("Skipping until we have the Validator system setup properly for testing.")
	// TODO Cannot pass in nil to NewDevopsServer
	devopsServer := NewDevopsServer(nil)

	// Build the spec
	chaincodePath := "github.com/hyperledger/fabric/core/example/chaincode/chaincode_example01"
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeID: &pb.ChaincodeID{Path: chaincodePath}}

	buildResult, err := devopsServer.Deploy(context.Background(), spec)
	if err == nil {
		t.Fail()
		t.Log("Expected error with no version specified")
		return
	}
	t.Logf("Deploy result = %s, err = %s", buildResult, err)
	//performHandshake(t, peerClientConn)
}
