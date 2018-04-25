/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccprovider

import (
	"bytes"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
)

// IsChaincodeDeployed returns true if the chaincode with given name and version is deployed
func IsChaincodeDeployed(chainid, ccName, ccVersion string, ccHash []byte, sccp sysccprovider.SystemChaincodeProvider) (bool, error) {
	qe, err := sccp.GetQueryExecutorForLedger(chainid)
	if err != nil {
		return false, fmt.Errorf("Could not retrieve QueryExecutor for channel %s, error %s", chainid, err)
	}
	defer qe.Done()

	// XXX We are leaking details of the LSCC table structure to other parts of the code, and this is terrible
	chaincodeDataBytes, err := qe.GetState("lscc", ccName)
	if err != nil {
		return false, fmt.Errorf("Could not retrieve state for chaincode %s on channel %s, error %s", ccName, chainid, err)
	}

	if chaincodeDataBytes == nil {
		return false, nil
	}

	chaincodeData := &ChaincodeData{}
	err = proto.Unmarshal(chaincodeDataBytes, chaincodeData)
	if err != nil {
		return false, fmt.Errorf("Unmarshalling ChaincodeQueryResponse failed, error %s", err)
	}
	return chaincodeData.CCVersion() == ccVersion && bytes.Equal(chaincodeData.Hash(), ccHash), nil
}
