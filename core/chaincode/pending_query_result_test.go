/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestPendingQueryResult(t *testing.T) {
	pqr := chaincode.PendingQueryResult{}
	assert.Equal(t, 0, pqr.Size())
	assert.Nil(t, pqr.Cut())

	for i := 1; i <= 5; i++ {
		kv := &queryresult.KV{Key: fmt.Sprintf("key-%d", i)}
		err := pqr.Add(kv)
		assert.NoError(t, err)
		assert.Equal(t, i, pqr.Size())
	}

	results := pqr.Cut()
	assert.Len(t, results, 5)
	assert.Equal(t, 0, pqr.Size())
	for i, bytes := range results {
		var kv queryresult.KV
		err := proto.Unmarshal(bytes.ResultBytes, &kv)
		assert.NoError(t, err)
		assert.Equal(t, kv.Key, fmt.Sprintf("key-%d", i+1))
	}
}

func TestPendingQueryResultBadQueryResult(t *testing.T) {
	pqr := chaincode.PendingQueryResult{}
	err := pqr.Add(brokenProto{})
	assert.EqualError(t, err, "marshal-failed")
}

type brokenProto struct{}

func (brokenProto) Reset()                   {}
func (brokenProto) String() string           { return "" }
func (brokenProto) ProtoMessage()            {}
func (brokenProto) Marshal() ([]byte, error) { return nil, errors.New("marshal-failed") }
