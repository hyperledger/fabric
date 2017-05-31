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
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"
	"golang.org/x/net/context"
)

//Execute - execute proposal, return original response of chaincode
func Execute(ctxt context.Context, cccid *ccprovider.CCContext, spec interface{}) (*pb.Response, *pb.ChaincodeEvent, error) {
	var err error
	var cds *pb.ChaincodeDeploymentSpec
	var ci *pb.ChaincodeInvocationSpec

	//init will call the Init method of a on a chain
	cctyp := pb.ChaincodeMessage_INIT
	if cds, _ = spec.(*pb.ChaincodeDeploymentSpec); cds == nil {
		if ci, _ = spec.(*pb.ChaincodeInvocationSpec); ci == nil {
			panic("Execute should be called with deployment or invocation spec")
		}
		cctyp = pb.ChaincodeMessage_TRANSACTION
	}

	_, cMsg, err := theChaincodeSupport.Launch(ctxt, cccid, spec)
	if err != nil {
		return nil, nil, fmt.Errorf("%s", err)
	}

	var ccMsg *pb.ChaincodeMessage
	ccMsg, err = createCCMessage(cctyp, cccid.TxID, cMsg)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to transaction message(%s)", err)
	}

	resp, err := theChaincodeSupport.Execute(ctxt, cccid, ccMsg, theChaincodeSupport.executetimeout)
	if err != nil {
		// Rollback transaction
		return nil, nil, fmt.Errorf("Failed to execute transaction (%s)", err)
	} else if resp == nil {
		// Rollback transaction
		return nil, nil, fmt.Errorf("Failed to receive a response for (%s)", cccid.TxID)
	}

	if resp.ChaincodeEvent != nil {
		resp.ChaincodeEvent.ChaincodeId = cccid.Name
		resp.ChaincodeEvent.TxId = cccid.TxID
	}

	if resp.Type == pb.ChaincodeMessage_COMPLETED {
		res := &pb.Response{}
		unmarshalErr := proto.Unmarshal(resp.Payload, res)
		if unmarshalErr != nil {
			return nil, nil, fmt.Errorf("Failed to unmarshal response for (%s): %s", cccid.TxID, unmarshalErr)
		}

		// Success
		return res, resp.ChaincodeEvent, nil
	} else if resp.Type == pb.ChaincodeMessage_ERROR {
		// Rollback transaction
		return nil, resp.ChaincodeEvent, fmt.Errorf("Transaction returned with failure: %s", string(resp.Payload))
	}

	//TODO - this should never happen ... a panic is more appropriate but will save that for future
	return nil, nil, fmt.Errorf("receive a response for (%s) but in invalid state(%d)", cccid.TxID, resp.Type)
}

// ExecuteWithErrorFilter is similar to Execute, but filters error contained in chaincode response and returns Payload of response only.
// Mostly used by unit-test.
func ExecuteWithErrorFilter(ctxt context.Context, cccid *ccprovider.CCContext, spec interface{}) ([]byte, *pb.ChaincodeEvent, error) {
	res, event, err := Execute(ctxt, cccid, spec)
	if err != nil {
		chaincodeLogger.Errorf("ExecuteWithErrorFilter %s error: %s", cccid.Name, err)
		return nil, nil, err
	}

	if res == nil {
		chaincodeLogger.Errorf("ExecuteWithErrorFilter %s get nil response without error", cccid.Name)
		return nil, nil, err
	}

	if res.Status != shim.OK {
		return nil, nil, fmt.Errorf("%s", res.Message)
	}

	return res.Payload, event, nil
}
