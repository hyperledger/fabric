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

package testutil

import (
	"testing"

	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
)

// ConstructBlockForSimulationResults constructs a block that includes a number of transactions - one per simulationResults
func ConstructBlockForSimulationResults(t *testing.T, simulationResults [][]byte, signer msp.SigningIdentity) *pb.Block2 {
	envs := []*common.Envelope{}
	for i := 0; i < len(simulationResults); i++ {
		env, err := ConstructTestTransaction(t, simulationResults[i], signer)
		if err != nil {
			t.Fatalf("ConstructTestTransaction failed, err %s", err)
		}
		envs = append(envs, env)
	}
	return newBlockEnv(envs)
}

// ConstructTestBlocks constructs 'numBlocks' number of blocks for testing
func ConstructTestBlocks(t *testing.T, numBlocks int) []*pb.Block2 {
	blocks := []*pb.Block2{}
	for i := 0; i < numBlocks; i++ {
		blocks = append(blocks, ConstructTestBlock(t, 10, 100, i*10))
	}
	return blocks
}

// ConstructTestBlock constructs a block with 'numTx' number of transactions for testing
func ConstructTestBlock(t *testing.T, numTx int, txSize int, startingTxID int) *pb.Block2 {
	txs := []*pb.Transaction2{}
	for i := startingTxID; i < numTx+startingTxID; i++ {
		tx, _ := putils.CreateTx(common.HeaderType_ENDORSER_TRANSACTION, []byte{}, []byte{}, ConstructRandomBytes(t, txSize), []*pb.Endorsement{})
		txs = append(txs, tx)
	}
	return newBlock(txs)
}

// ConstructTestTransaction constructs a transaction for testing
func ConstructTestTransaction(t *testing.T, simulationResults []byte, signer msp.SigningIdentity) (*common.Envelope, error) {
	ss, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	prop, err := putils.CreateChaincodeProposal(&pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: &pb.ChaincodeID{Name: "foo"}}}, ss)
	if err != nil {
		return nil, err
	}

	presp, err := putils.CreateProposalResponse(prop.Header, prop.Payload, simulationResults, nil, nil, signer)
	if err != nil {
		return nil, err
	}

	env, err := putils.CreateSignedTx(prop, signer, presp)
	if err != nil {
		return nil, err
	}

	return env, nil
}

// ComputeBlockHash computes the crypto-hash of a block
func ComputeBlockHash(t testing.TB, block *pb.Block2) []byte {
	serBlock, err := pb.ConstructSerBlock2(block)
	AssertNoError(t, err, "Error while getting hash from block")
	return serBlock.ComputeHash()
}

func newBlock(txs []*pb.Transaction2) *pb.Block2 {
	block := &pb.Block2{}
	block.PreviousBlockHash = []byte{}
	for i := 0; i < len(txs); i++ {
		txBytes, _ := proto.Marshal(txs[i])
		block.Transactions = append(block.Transactions, txBytes)
	}
	return block
}

func newBlockEnv(env []*common.Envelope) *pb.Block2 {
	block := &pb.Block2{}
	block.PreviousBlockHash = []byte{}
	for i := 0; i < len(env); i++ {
		txBytes, _ := proto.Marshal(env[i])
		block.Transactions = append(block.Transactions, txBytes)
	}
	return block
}
