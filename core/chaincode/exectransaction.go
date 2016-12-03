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

	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/events/producer"
	pb "github.com/hyperledger/fabric/protos/peer"
)

//Execute - execute proposal
func Execute(ctxt context.Context, chainID string, txid string, prop *pb.Proposal, spec interface{}) ([]byte, *pb.ChaincodeEvent, error) {
	var err error
	var cds *pb.ChaincodeDeploymentSpec
	var ci *pb.ChaincodeInvocationSpec
	if cds, _ = spec.(*pb.ChaincodeDeploymentSpec); cds == nil {
		if ci, _ = spec.(*pb.ChaincodeInvocationSpec); ci == nil {
			panic("Execute should be called with deployment or invocation spec")
		}
	}

	if cds != nil {
		_, err := theChaincodeSupport.Deploy(ctxt, cds)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to deploy chaincode spec(%s)", err)
		}

		_, _, err = theChaincodeSupport.Launch(ctxt, chainID, txid, prop, cds)
		if err != nil {
			return nil, nil, fmt.Errorf("%s", err)
		}
	} else {
		//will launch if necessary (and wait for ready)
		cID, cMsg, err := theChaincodeSupport.Launch(ctxt, chainID, txid, prop, ci)
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
		ccMsg, err = createTransactionMessage(txid, cMsg)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to transaction message(%s)", err)
		}

		resp, err := theChaincodeSupport.Execute(ctxt, chainID, chaincode, ccMsg, timeout, prop)
		if err != nil {
			// Rollback transaction
			return nil, nil, fmt.Errorf("Failed to execute transaction (%s)", err)
		} else if resp == nil {
			// Rollback transaction
			return nil, nil, fmt.Errorf("Failed to receive a response for (%s)", txid)
		} else {
			if resp.ChaincodeEvent != nil {
				resp.ChaincodeEvent.ChaincodeID = chaincode
				resp.ChaincodeEvent.TxID = txid
			}

			if resp.Type == pb.ChaincodeMessage_COMPLETED {
				// Success
				return resp.Payload, resp.ChaincodeEvent, nil
			} else if resp.Type == pb.ChaincodeMessage_ERROR {
				// Rollback transaction
				return nil, resp.ChaincodeEvent, fmt.Errorf("Transaction returned with failure: %s", string(resp.Payload))
			}
			return resp.Payload, nil, fmt.Errorf("receive a response for (%s) but in invalid state(%d)", txid, resp.Type)
		}

	}
	return nil, nil, err
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
