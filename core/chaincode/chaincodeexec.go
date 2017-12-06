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
	"bytes"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/resourcesconfig"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/peer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

//create a chaincode invocation spec
func createCIS(ccname string, args [][]byte) (*pb.ChaincodeInvocationSpec, error) {
	var err error
	spec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: ccname}, Input: &pb.ChaincodeInput{Args: args}}}
	if nil != err {
		return nil, err
	}
	return spec, nil
}

func getApplicationConfigForChain(chainID string) (channelconfig.Application, error) {
	peerSupport := peer.GetSupport()

	ac, exists := peerSupport.GetApplicationConfig(chainID)
	if !exists {
		return nil, errors.Errorf("no configuration available for channel %s", chainID)
	}

	return ac, nil
}

// GetCDS retrieves a chaincode deployment spec for the required chaincode
func GetCDS(ctxt context.Context, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal, chainID string, chaincodeID string) ([]byte, error) {
	ac, err := getApplicationConfigForChain(chainID)
	if err != nil {
		return nil, err
	}

	if ac.Capabilities().LifecycleViaConfig() {
		if err := aclmgmt.GetACLProvider().CheckACL(aclmgmt.LSCC_GETDEPSPEC, chainID, signedProp); err != nil {
			return nil, errors.Errorf("Authorization request failed %s: %s", chainID, err)
		}

		cd, exists := peer.GetSupport().ChaincodeByName(chainID, chaincodeID)
		if !exists {
			return nil, errors.Errorf("non existent chaincode %s on channel %s", chaincodeID, chainID)
		}

		ccpack, err := ccprovider.GetChaincodeFromFS(chaincodeID, cd.CCVersion())
		if err != nil {
			return nil, errors.WithMessage(err, fmt.Sprintf("failed retrieving information for chaincode %s/%s", chaincodeID, cd.CCVersion()))
		}

		if !bytes.Equal(cd.Hash(), ccpack.GetId()) {
			return nil, errors.Errorf("hash mismatch for chaincode %s/%s", chaincodeID, cd.CCVersion())
		}

		return ccpack.GetDepSpecBytes(), nil
	} else {
		version := util.GetSysCCVersion()
		cccid := ccprovider.NewCCContext(chainID, "lscc", version, txid, true, signedProp, prop)
		res, _, err := ExecuteChaincode(ctxt, cccid, [][]byte{[]byte("getdepspec"), []byte(chainID), []byte(chaincodeID)})
		if err != nil {
			return nil, errors.WithMessage(err, fmt.Sprintf("execute getdepspec(%s, %s) of LSCC error", chainID, chaincodeID))
		}
		if res.Status != shim.OK {
			return nil, errors.Errorf("get ChaincodeDeploymentSpec for %s/%s from LSCC error: %s", chaincodeID, chainID, res.Message)
		}

		return res.Payload, nil
	}
}

// GetChaincodeDefinition returns resourcesconfig.ChaincodeDefinition for the chaincode with the supplied name
func GetChaincodeDefinition(ctxt context.Context, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal, chainID string, chaincodeID string) (resourcesconfig.ChaincodeDefinition, error) {
	ac, err := getApplicationConfigForChain(chainID)
	if err != nil {
		return nil, err
	}

	if ac.Capabilities().LifecycleViaConfig() {
		if err := aclmgmt.GetACLProvider().CheckACL(aclmgmt.LSCC_GETCCDATA, chainID, signedProp); err != nil {
			return nil, errors.Errorf("Authorization request failed %s: %s", chainID, err)
		}

		cd, exists := peer.GetSupport().ChaincodeByName(chainID, chaincodeID)
		if !exists {
			return nil, errors.Errorf("non existent chaincode %s on channel %s", chaincodeID, chainID)
		}

		return cd, nil
	} else {
		version := util.GetSysCCVersion()
		cccid := ccprovider.NewCCContext(chainID, "lscc", version, txid, true, signedProp, prop)
		res, _, err := ExecuteChaincode(ctxt, cccid, [][]byte{[]byte("getccdata"), []byte(chainID), []byte(chaincodeID)})
		if err == nil {
			if res.Status != shim.OK {
				return nil, errors.New(res.Message)
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
		err = errors.WithMessage(err, "error executing chaincode")
		chaincodeLogger.Errorf("%+v", err)
		return nil, nil, err
	}

	return res, ccevent, err
}
