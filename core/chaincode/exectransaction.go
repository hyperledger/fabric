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
	pb "github.com/hyperledger/fabric/protos"
)

//Execute - execute transaction or a query
func Execute(ctxt context.Context, chain *ChaincodeSupport, t *pb.Transaction) ([]byte, *pb.ChaincodeEvent, error) {
	var err error

	if secHelper := chain.getSecHelper(); nil != secHelper {
		var err error
		t, err = secHelper.TransactionPreExecution(t)
		// Note that t is now decrypted and is a deep clone of the original input t
		if nil != err {
			return nil, nil, err
		}
	}

	if t.Type == pb.Transaction_CHAINCODE_DEPLOY {
		_, err := chain.Deploy(ctxt, t)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to deploy chaincode spec(%s)", err)
		}

		_, _, err = chain.Launch(ctxt, t)
		if err != nil {
			return nil, nil, fmt.Errorf("%s", err)
		}
	} else if t.Type == pb.Transaction_CHAINCODE_INVOKE || t.Type == pb.Transaction_CHAINCODE_QUERY {
		//will launch if necessary (and wait for ready)
		cID, cMsg, err := chain.Launch(ctxt, t)
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
		if t.Type == pb.Transaction_CHAINCODE_INVOKE {
			ccMsg, err = createTransactionMessage(t.Txid, cMsg)
			if err != nil {
				return nil, nil, fmt.Errorf("Failed to transaction message(%s)", err)
			}
		} else {
			ccMsg, err = createQueryMessage(t.Txid, cMsg)
			if err != nil {
				return nil, nil, fmt.Errorf("Failed to query message(%s)", err)
			}
		}

		resp, err := chain.Execute(ctxt, chaincode, ccMsg, timeout, t)
		if err != nil {
			// Rollback transaction
			return nil, nil, fmt.Errorf("Failed to execute transaction or query(%s)", err)
		} else if resp == nil {
			// Rollback transaction
			return nil, nil, fmt.Errorf("Failed to receive a response for (%s)", t.Txid)
		} else {
			if resp.ChaincodeEvent != nil {
				resp.ChaincodeEvent.ChaincodeID = chaincode
				resp.ChaincodeEvent.TxID = t.Txid
			}

			if resp.Type == pb.ChaincodeMessage_COMPLETED || resp.Type == pb.ChaincodeMessage_QUERY_COMPLETED {
				// Success
				return resp.Payload, resp.ChaincodeEvent, nil
			} else if resp.Type == pb.ChaincodeMessage_ERROR || resp.Type == pb.ChaincodeMessage_QUERY_ERROR {
				// Rollback transaction
				return nil, resp.ChaincodeEvent, fmt.Errorf("Transaction or query returned with failure: %s", string(resp.Payload))
			}
			return resp.Payload, nil, fmt.Errorf("receive a response for (%s) but in invalid state(%d)", t.Txid, resp.Type)
		}

	} else {
		err = fmt.Errorf("Invalid transaction type %s", t.Type.String())
	}
	return nil, nil, err
}

/**************
//ExecuteTransactions - will execute transactions on the array one by one
//will return an array of errors one for each transaction. If the execution
//succeeded, array element will be nil. returns []byte of state hash or
//error
func ExecuteTransactions(ctxt context.Context, cname ChainName, xacts []*pb.Transaction) (succeededTXs []*pb.Transaction, stateHash []byte, ccevents []*pb.ChaincodeEvent, txerrs []error, err error) {
	var chain = GetChain(cname)
	if chain == nil {
		// TODO: We should never get here, but otherwise a good reminder to better handle
		panic(fmt.Sprintf("[ExecuteTransactions]Chain %s not found\n", cname))
	}

	txerrs = make([]error, len(xacts))
	ccevents = make([]*pb.ChaincodeEvent, len(xacts))
	var succeededTxs = make([]*pb.Transaction, 0)
	for i, t := range xacts {
		_, ccevents[i], txerrs[i] = Execute(ctxt, chain, t)
		if txerrs[i] == nil {
			succeededTxs = append(succeededTxs, t)
		} else {
			sendTxRejectedEvent(xacts[i], txerrs[i].Error())
		}
	}

	var lgr *ledger.Ledger
	lgr, err = ledger.GetLedger()
	if err == nil {
		stateHash, err = lgr.GetTempStateHash()
	}

	return succeededTxs, stateHash, ccevents, txerrs, err
}
**************/

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
