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

package multichain

import (
	"reflect"
	"testing"

	"github.com/hyperledger/fabric/common/chainconfig"
	"github.com/hyperledger/fabric/common/configtx"
	mockchainconfig "github.com/hyperledger/fabric/common/mocks/chainconfig"
	"github.com/hyperledger/fabric/common/policies"
	coreutil "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	mocksharedconfig "github.com/hyperledger/fabric/orderer/mocks/sharedconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

type mockPolicy struct {
	err error
}

func (mp *mockPolicy) Evaluate(sd []*cb.SignedData) error {
	return mp.err
}

type mockPolicyManager struct {
	mp *mockPolicy
}

func (mpm *mockPolicyManager) GetPolicy(id string) (policies.Policy, bool) {
	return mpm.mp, mpm.mp != nil
}

type mockSupport struct {
	mpm         *mockPolicyManager
	msc         *mocksharedconfig.Manager
	chainID     string
	queue       []*cb.Envelope
	chainConfig *mockchainconfig.Descriptor
}

func newMockSupport(chainID string) *mockSupport {
	return &mockSupport{
		mpm:         &mockPolicyManager{},
		msc:         &mocksharedconfig.Manager{},
		chainID:     chainID,
		chainConfig: &mockchainconfig.Descriptor{},
	}
}

func (ms *mockSupport) Enqueue(msg *cb.Envelope) bool {
	ms.queue = append(ms.queue, msg)
	return true
}

func (ms *mockSupport) ChainID() string {
	return ms.chainID
}

func (ms *mockSupport) PolicyManager() policies.Manager {
	return ms.mpm
}

func (ms *mockSupport) SharedConfig() sharedconfig.Manager {
	return ms.msc
}

func (ms *mockSupport) ChainConfig() chainconfig.Descriptor {
	return ms.chainConfig
}

type mockChainCreator struct {
	newChains []*cb.Envelope
	ms        *mockSupport
	sysChain  *systemChain
}

func newMockChainCreator() *mockChainCreator {
	mcc := &mockChainCreator{
		ms: newMockSupport(provisional.TestChainID),
	}
	mcc.sysChain = newSystemChain(mcc.ms)
	return mcc
}

func (mcc *mockChainCreator) newChain(configTx *cb.Envelope) {
	mcc.newChains = append(mcc.newChains, configTx)
}

func (mcc *mockChainCreator) systemChain() *systemChain {
	return mcc.sysChain
}

func TestGoodProposal(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.msc.ChainCreationPolicyNamesVal = []string{provisional.AcceptAllPolicyKey}
	mcc.ms.mpm.mp = &mockPolicy{}

	chainCreateTx := &cb.ConfigurationItem{
		Key:  configtx.CreationPolicyKey,
		Type: cb.ConfigurationItem_Orderer,
		Value: utils.MarshalOrPanic(&ab.CreationPolicy{
			Policy: provisional.AcceptAllPolicyKey,
			Digest: mcc.ms.ChainConfig().HashingAlgorithm()([]byte{}),
		}),
	}
	ingressTx := makeConfigTxWithItems(newChainID, chainCreateTx)
	status := mcc.sysChain.proposeChain(ingressTx)
	if status != cb.Status_SUCCESS {
		t.Fatalf("Should have successfully proposed chain")
	}

	expected := 1
	if len(mcc.ms.queue) != expected {
		t.Fatalf("Expected %d creation txs in the chain, but found %d", expected, len(mcc.ms.queue))
	}

	wrapped := mcc.ms.queue[0]
	payload := utils.UnmarshalPayloadOrPanic(wrapped.Payload)
	if payload.Header.ChainHeader.Type != int32(cb.HeaderType_ORDERER_TRANSACTION) {
		t.Fatalf("Wrapped transaction should be of type ORDERER_TRANSACTION")
	}
	envelope := utils.UnmarshalEnvelopeOrPanic(payload.Data)
	if !reflect.DeepEqual(envelope, ingressTx) {
		t.Fatalf("Received different configtx than ingressed into the system")
	}

	sysFilter := newSystemChainFilter(mcc)
	action, committer := sysFilter.Apply(wrapped)

	if action != filter.Accept {
		t.Fatalf("Should have accepted the transaction, as it was already validated")
	}

	if !committer.Isolated() {
		t.Fatalf("Chain creation transactions should be isolated on commit")
	}

	committer.Commit()
	if len(mcc.newChains) != 1 {
		t.Fatalf("Proposal should only have created 1 new chain")
	}

	if !reflect.DeepEqual(mcc.newChains[0], ingressTx) {
		t.Fatalf("New chain should have been created with ingressTx")
	}
}

func TestProposalWithBadPolicy(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.mpm.mp = &mockPolicy{}

	chainCreateTx := &cb.ConfigurationItem{
		Key:  configtx.CreationPolicyKey,
		Type: cb.ConfigurationItem_Orderer,

		Value: utils.MarshalOrPanic(&ab.CreationPolicy{
			Policy: provisional.AcceptAllPolicyKey,
			Digest: coreutil.ComputeCryptoHash([]byte{}),
		}),
	}
	ingressTx := makeConfigTxWithItems(newChainID, chainCreateTx)

	status := mcc.sysChain.proposeChain(ingressTx)

	if status == cb.Status_SUCCESS {
		t.Fatalf("Should not have validated the transaction with no authorized chain creation policies")
	}
}

func TestProposalWithMissingPolicy(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.msc.ChainCreationPolicyNamesVal = []string{provisional.AcceptAllPolicyKey}

	chainCreateTx := &cb.ConfigurationItem{
		Key:  configtx.CreationPolicyKey,
		Type: cb.ConfigurationItem_Orderer,
		Value: utils.MarshalOrPanic(&ab.CreationPolicy{
			Policy: provisional.AcceptAllPolicyKey,
			Digest: coreutil.ComputeCryptoHash([]byte{}),
		}),
	}
	ingressTx := makeConfigTxWithItems(newChainID, chainCreateTx)

	status := mcc.sysChain.proposeChain(ingressTx)

	if status == cb.Status_SUCCESS {
		t.Fatalf("Should not have validated the transaction with missing policy")
	}
}

func TestProposalWithBadDigest(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.mpm.mp = &mockPolicy{}
	mcc.ms.msc.ChainCreationPolicyNamesVal = []string{provisional.AcceptAllPolicyKey}

	chainCreateTx := &cb.ConfigurationItem{
		Key:  configtx.CreationPolicyKey,
		Type: cb.ConfigurationItem_Orderer,
		Value: utils.MarshalOrPanic(&ab.CreationPolicy{
			Policy: provisional.AcceptAllPolicyKey,
			Digest: coreutil.ComputeCryptoHash([]byte("BAD_DIGEST")),
		}),
	}
	ingressTx := makeConfigTxWithItems(newChainID, chainCreateTx)

	status := mcc.sysChain.proposeChain(ingressTx)

	if status == cb.Status_SUCCESS {
		t.Fatalf("Should not have validated the transaction with missing policy")
	}
}
