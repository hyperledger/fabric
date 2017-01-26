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
	"errors"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/events/producer"
	pb "github.com/hyperledger/fabric/protos/peer"
)

//Execute - execute proposal, return original response of chaincode
func Execute(ctxt context.Context, cccid *ccprovider.CCContext, spec interface{}) (*pb.Response, *pb.ChaincodeEvent, error) {
	var err error
	var cds *pb.ChaincodeDeploymentSpec
	var ci *pb.ChaincodeInvocationSpec
	if cds, _ = spec.(*pb.ChaincodeDeploymentSpec); cds == nil {
		if ci, _ = spec.(*pb.ChaincodeInvocationSpec); ci == nil {
			panic("Execute should be called with deployment or invocation spec")
		}
	}

	if cds != nil {
		_, err := theChaincodeSupport.Deploy(ctxt, cccid, cds)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to deploy chaincode spec(%s)", err)
		}

		_, _, err = theChaincodeSupport.Launch(ctxt, cccid, cds)
		if err != nil {
			return nil, nil, fmt.Errorf("%s", err)
		}
	} else {
		//will launch if necessary (and wait for ready)
		cID, cMsg, err := theChaincodeSupport.Launch(ctxt, cccid, ci)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to launch chaincode spec(%s)", err)
		}

		//this should work because it worked above...
		chaincode := cID.Name

		if err != nil {
			return nil, nil, fmt.Errorf("Failed to stablish stream to container %s", chaincode)
		}

		// TODO: Need to comment next line and uncomment call to getTimeout, when transaction blocks are being created
		timeout := time.Duration(30000) * time.Millisecond

		if err != nil {
			return nil, nil, fmt.Errorf("Failed to retrieve chaincode spec(%s)", err)
		}

		var ccMsg *pb.ChaincodeMessage
		ccMsg, err = createTransactionMessage(cccid.TxID, cMsg)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to transaction message(%s)", err)
		}

		resp, err := theChaincodeSupport.Execute(ctxt, cccid, ccMsg, timeout)
		if err != nil {
			// Rollback transaction
			return nil, nil, fmt.Errorf("Failed to execute transaction (%s)", err)
		} else if resp == nil {
			// Rollback transaction
			return nil, nil, fmt.Errorf("Failed to receive a response for (%s)", cccid.TxID)
		}
		res := &pb.Response{}
		unmarshalErr := proto.Unmarshal(resp.Payload, res)
		if unmarshalErr != nil {
			return nil, nil, fmt.Errorf("Failed to unmarshal response for (%s): %s", cccid.TxID, unmarshalErr)
		} else {
			if resp.ChaincodeEvent != nil {
				resp.ChaincodeEvent.ChaincodeID = cccid.Name
				resp.ChaincodeEvent.TxID = cccid.TxID
			}

			if resp.Type == pb.ChaincodeMessage_COMPLETED {
				// Success
				return res, resp.ChaincodeEvent, nil
			} else if resp.Type == pb.ChaincodeMessage_ERROR {
				// Rollback transaction
				return nil, resp.ChaincodeEvent, fmt.Errorf("Transaction returned with failure: %s", string(resp.Payload))
			}
			return res, nil, fmt.Errorf("receive a response for (%s) but in invalid state(%d)", cccid.TxID, resp.Type)
		}

	}
	return &pb.Response{Status: shim.OK, Payload: nil}, nil, err
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

// GetSecureContext returns the security context from the context object or error
// Security context is nil if security is off from core.yaml file
// func GetSecureContext(ctxt context.Context) (crypto.Peer, error) {
// 	var err error
// 	temp := ctxt.Value("security")
// 	if nil != temp {
// 		if secCxt, ok := temp.(crypto.Peer); ok {
// 			return secCxt, nil
// 		}
// 		err = errors.New("Failed to convert security context type")
// 	}
// 	return nil, err
// }

var errFailedToGetChainCodeSpecForTransaction = errors.New("Failed to get ChainCodeSpec from Transaction")

func sendTxRejectedEvent(tx *pb.Transaction, errorMsg string) {
	producer.Send(producer.CreateRejectionEvent(tx, errorMsg))
}
