/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/golang/protobuf/proto"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	pb "github.com/hyperledger/fabric/protos/peer"
)

type PendingQueryResult struct {
	batch []*pb.QueryResultBytes
}

func (p *PendingQueryResult) Cut() []*pb.QueryResultBytes {
	batch := p.batch
	p.batch = nil
	return batch
}

func (p *PendingQueryResult) Add(queryResult commonledger.QueryResult) error {
	queryResultBytes, err := proto.Marshal(queryResult.(proto.Message))
	if err != nil {
		chaincodeLogger.Errorf("failed to marshal query result: %s", err)
		return err
	}
	p.batch = append(p.batch, &pb.QueryResultBytes{ResultBytes: queryResultBytes})
	return nil
}

func (p *PendingQueryResult) Size() int {
	return len(p.batch)
}
