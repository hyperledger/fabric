/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	lm "github.com/hyperledger/fabric/common/mocks/ledger"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb/errors"
)

type mockStoreSupport struct {
	Qe   *lm.MockQueryExecutor
	QErr error
}

func (c *mockStoreSupport) GetQueryExecutorForLedger(cid string) (ledger.QueryExecutor, error) {
	return c.Qe, c.QErr
}

func (c *mockStoreSupport) GetCollectionKVSKey(cc common.CollectionCriteria) string {
	return cc.Channel + cc.Namespace
}

func (c *mockStoreSupport) GetIdentityDeserializer(chainID string) msp.IdentityDeserializer {
	return &mockDeserializer{}
}

func TestCollectionStore(t *testing.T) {
	wState := make(map[string]map[string][]byte)
	support := &mockStoreSupport{Qe: &lm.MockQueryExecutor{wState}}
	cs := NewSimpleCollectionStore(support)
	assert.NotNil(t, cs)

	support.QErr = errors.New("")
	_, err := cs.RetrieveCollection(common.CollectionCriteria{})
	assert.Error(t, err)

	support.QErr = nil
	wState["lscc"] = make(map[string][]byte)

	_, err = cs.RetrieveCollection(common.CollectionCriteria{})
	assert.Error(t, err)

	ccr := common.CollectionCriteria{Channel: "ch", Namespace: "cc", Collection: "mycollection"}

	wState["lscc"][support.GetCollectionKVSKey(ccr)] = []byte("barf")

	_, err = cs.RetrieveCollection(ccr)
	assert.Error(t, err)

	cc := &common.CollectionConfig{Payload: &common.CollectionConfig_StaticCollectionConfig{&common.StaticCollectionConfig{Name: "mycollection"}}}
	ccp := &common.CollectionConfigPackage{[]*common.CollectionConfig{cc}}
	ccpBytes, err := proto.Marshal(ccp)
	assert.NoError(t, err)
	assert.NotNil(t, ccpBytes)

	wState["lscc"][support.GetCollectionKVSKey(ccr)] = ccpBytes

	_, err = cs.RetrieveCollection(ccr)
	assert.Error(t, err)

	var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), signers)
	accessPolicy := createCollectionPolicyConfig(policyEnvelope)

	cc = &common.CollectionConfig{Payload: &common.CollectionConfig_StaticCollectionConfig{&common.StaticCollectionConfig{Name: "mycollection", MemberOrgsPolicy: accessPolicy}}}
	ccp = &common.CollectionConfigPackage{[]*common.CollectionConfig{cc}}
	ccpBytes, err = proto.Marshal(ccp)
	assert.NoError(t, err)
	assert.NotNil(t, ccpBytes)

	wState["lscc"][support.GetCollectionKVSKey(ccr)] = ccpBytes

	c, err := cs.RetrieveCollection(ccr)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	ca, err := cs.RetrieveCollectionAccessPolicy(ccr)
	assert.NoError(t, err)
	assert.NotNil(t, ca)

	c, err = cs.RetrieveCollection(common.CollectionCriteria{Channel: "ch", Namespace: "cc", Collection: "asd"})
	assert.Error(t, err)
	assert.Nil(t, c)

	ccc, err := cs.RetrieveCollectionConfigPackage(ccr)
	assert.NoError(t, err)
	assert.NotNil(t, ccc)
}
