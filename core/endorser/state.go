/*
Copyright IBM Corp. 2018 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"github.com/hyperledger/fabric/core/handlers/endorsement/api/state"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
)

// go:generate mockery -dir core/endorser/ -name QueryCreator -case underscore -output core/endorser/mocks/
// QueryCreator creates new QueryExecutors
type QueryCreator interface {
	NewQueryExecutor() (ledger.QueryExecutor, error)
}

// ChannelState defines state operations
type ChannelState struct {
	QueryCreator
}

// FetchState fetches state
func (cs *ChannelState) FetchState() (endorsement.State, error) {
	qe, err := cs.NewQueryExecutor()
	if err != nil {
		return nil, err
	}

	return &StateContext{
		QueryExecutor: qe,
	}, nil
}

// StateContext defines an execution context that interacts with the state
type StateContext struct {
	ledger.QueryExecutor
}

// GetTransientByTXID returns the private data associated with this transaction ID.
// Currently not implemented yet, see: https://jira.hyperledger.org/browse/FAB-9675
func (*StateContext) GetTransientByTXID(txID string) ([]*rwset.TxPvtReadWriteSet, error) {
	panic("Not implemented")
}
