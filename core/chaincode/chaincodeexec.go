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
	"golang.org/x/net/context"

	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"
)

//create a chaincode invocation spec
func createCIS(ccname string, args [][]byte) (*pb.ChaincodeInvocationSpec, error) {
	var err error
	spec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: ccname}, Input: &pb.ChaincodeInput{Args: args}}}
	if nil != err {
		return nil, err
	}
	return spec, nil
}

// GetCDSFromLCCC gets chaincode deployment spec from LCCC
func GetCDSFromLCCC(ctxt context.Context, txid string, prop *pb.Proposal, chainID string, chaincodeID string) ([]byte, error) {
	version := util.GetSysCCVersion()
	cccid := ccprovider.NewCCContext(chainID, "lccc", version, txid, true, prop)
	res, _, err := ExecuteChaincode(ctxt, cccid, [][]byte{[]byte("getdepspec"), []byte(chainID), []byte(chaincodeID)})
	if err != nil {
		return nil, fmt.Errorf("Execute getdepspec(%s, %s) of LCCC error: %s", chainID, chaincodeID, err)
	}
	if res.Status != shim.OK {
		return nil, fmt.Errorf("Get ChaincodeDeploymentSpec for %s/%s from LCCC error: %s", chaincodeID, chainID, res.Message)
	}

	return res.Payload, nil
}

// GetChaincodeDataFromLCCC gets chaincode data from LCCC given name
func GetChaincodeDataFromLCCC(ctxt context.Context, txid string, prop *pb.Proposal, chainID string, chaincodeID string) (*ccprovider.ChaincodeData, error) {
	version := util.GetSysCCVersion()
	cccid := ccprovider.NewCCContext(chainID, "lccc", version, txid, true, prop)
	res, _, err := ExecuteChaincode(ctxt, cccid, [][]byte{[]byte("getccdata"), []byte(chainID), []byte(chaincodeID)})
	if err == nil {
		if res.Status != shim.OK {
			return nil, fmt.Errorf("%s", res.Message)
		}
		cd := &ccprovider.ChaincodeData{}
		err = proto.Unmarshal(res.Payload, cd)
		if err != nil {
			return nil, err
		}
		return cd, nil
	}

	return nil, err
}

// ExecuteChaincode executes a given chaincode given chaincode name and arguments
func ExecuteChaincode(ctxt context.Context, cccid *ccprovider.CCContext, args [][]byte) (*pb.Response, *pb.ChaincodeEvent, error) {
	var spec *pb.ChaincodeInvocationSpec
	var err error
	var res *pb.Response
	var ccevent *pb.ChaincodeEvent

	spec, err = createCIS(cccid.Name, args)
	res, ccevent, err = Execute(ctxt, cccid, spec)
	if err != nil {
		chaincodeLogger.Errorf("Error executing chaincode: %s", err)
		return nil, nil, fmt.Errorf("Error executing chaincode: %s", err)
	}

	return res, ccevent, err
}
