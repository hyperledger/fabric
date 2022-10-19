/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

// client helps in a transction simulation. The client keeps accumlating the results of each simulated transaction
// in a slice and at a later stage can be used to cut a test block for committing.
// In a test, for each instantiated ledger, a single instance of a client is typically sufficient.
type client struct {
	lgr            ledger.PeerLedger
	lgrID          string
	simulatedTrans []*txAndPvtdata // accumulates the results of transactions simulations
	missingPvtData ledger.TxMissingPvtData
	assert         *require.Assertions
}

func newClient(lgr ledger.PeerLedger, lgrID string, t *testing.T) *client {
	return &client{lgr, lgrID, nil, make(ledger.TxMissingPvtData), require.New(t)}
}

// simulateDataTx takes a simulation logic and wraps it between
// (A) the pre-simulation tasks (such as obtaining a fresh simulator) and
// (B) the post simulation tasks (such as gathering (public and pvt) simulation results and constructing a transaction)
// Since (A) and (B) both are handled in this function, the test code can be kept simple by just supplying the simulation logic
func (c *client) simulateDataTx(txid string, simulationLogic func(s *simulator)) *txAndPvtdata {
	if txid == "" {
		txid = util.GenerateUUID()
	}
	ledgerSimulator, err := c.lgr.NewTxSimulator(txid)
	c.assert.NoError(err)
	sim := &simulator{ledgerSimulator, txid, c.assert}
	simulationLogic(sim)
	txAndPvtdata := sim.done()
	c.simulatedTrans = append(c.simulatedTrans, txAndPvtdata)
	return txAndPvtdata
}

func (c *client) submitHandCraftedTx(txAndPvtdata *txAndPvtdata) {
	c.simulatedTrans = append(c.simulatedTrans, txAndPvtdata)
}

func (c *client) addPostOrderTx(txid string, customTxType common.HeaderType) *txAndPvtdata {
	if txid == "" {
		txid = util.GenerateUUID()
	}
	channelHeader := protoutil.MakeChannelHeader(customTxType, 0, c.lgrID, 0)
	channelHeader.TxId = txid
	paylBytes := protoutil.MarshalOrPanic(
		&common.Payload{
			Header: protoutil.MakePayloadHeader(channelHeader, &common.SignatureHeader{}),
			Data:   nil,
		},
	)
	env := &common.Envelope{
		Payload:   paylBytes,
		Signature: nil,
	}
	txAndPvtdata := &txAndPvtdata{Txid: txid, Envelope: env}
	c.simulatedTrans = append(c.simulatedTrans, txAndPvtdata)
	return txAndPvtdata
}

// simulateDeployTx mimics a transction that deploys a chaincode. This in turn calls the function 'simulateDataTx'
// with supplying the simulation logic that mimics the invoke function of 'lscc' for the ledger tests
func (c *client) simulateDeployTx(ccName string, collConfs []*collConf) *txAndPvtdata {
	ccData := &ccprovider.ChaincodeData{Name: ccName}
	ccDataBytes, err := proto.Marshal(ccData)
	c.assert.NoError(err)

	psudoLSCCInvokeFunc := func(s *simulator) {
		s.setState("lscc", ccName, string(ccDataBytes))
		if collConfs != nil {
			protoBytes, err := convertToCollConfigProtoBytes(collConfs)
			c.assert.NoError(err)
			s.setState("lscc", privdata.BuildCollectionKVSKey(ccName), string(protoBytes))
		}
	}
	return c.simulateDataTx("", psudoLSCCInvokeFunc)
}

// simulateUpgradeTx see comments on function 'simulateDeployTx'
func (c *client) simulateUpgradeTx(ccName string, collConfs []*collConf) *txAndPvtdata {
	return c.simulateDeployTx(ccName, collConfs)
}

func (c *client) causeMissingPvtData(txIndex uint64) {
	pvtws := c.simulatedTrans[txIndex].Pvtws
	for _, nsPvtRwset := range pvtws.NsPvtRwset {
		for _, collPvtRwset := range nsPvtRwset.CollectionPvtRwset {
			c.missingPvtData.Add(txIndex, nsPvtRwset.Namespace, collPvtRwset.CollectionName, true)
		}
	}
	c.simulatedTrans[txIndex].Pvtws = nil
}

func (c *client) discardSimulation() {
	c.simulatedTrans = nil
}

func (c *client) retrieveCommittedBlocksAndPvtdata(startNum, endNum uint64) []*ledger.BlockAndPvtData {
	data := []*ledger.BlockAndPvtData{}
	for i := startNum; i <= endNum; i++ {
		d, err := c.lgr.GetPvtDataAndBlockByNum(i, nil)
		c.assert.NoError(err)
		data = append(data, d)
	}
	return data
}

func (c *client) currentHeight() uint64 {
	bcInfo, err := c.lgr.GetBlockchainInfo()
	c.assert.NoError(err)
	return bcInfo.Height
}

func (c *client) currentCommitHash() []byte {
	block, err := c.lgr.GetBlockByNumber(c.currentHeight() - 1)
	c.assert.NoError(err)
	if len(block.Metadata.Metadata) < int(common.BlockMetadataIndex_COMMIT_HASH+1) {
		return nil
	}
	commitHash := &common.Metadata{}
	err = proto.Unmarshal(block.Metadata.Metadata[common.BlockMetadataIndex_COMMIT_HASH], commitHash)
	c.assert.NoError(err)
	return commitHash.Value
}

// /////////////////////   simulator wrapper functions  ///////////////////////
type simulator struct {
	ledger.TxSimulator
	txid   string
	assert *require.Assertions
}

func (s *simulator) getState(ns, key string) string {
	val, err := s.GetState(ns, key)
	s.assert.NoError(err)
	return string(val)
}

func (s *simulator) setState(ns, key string, val string) {
	s.assert.NoError(
		s.SetState(ns, key, []byte(val)),
	)
}

func (s *simulator) setPvtdata(ns, coll, key string, val string) {
	s.assert.NoError(
		s.SetPrivateData(ns, coll, key, []byte(val)),
	)
}

func (s *simulator) purgePvtdata(ns, coll, key string) {
	s.assert.NoError(
		s.PurgePrivateData(ns, coll, key),
	)
}

func (s *simulator) done() *txAndPvtdata {
	s.Done()
	simRes, err := s.GetTxSimulationResults()
	s.assert.NoError(err)
	pubRwsetBytes, err := simRes.GetPubSimulationBytes()
	s.assert.NoError(err)
	envelope, err := constructTransaction(s.txid, pubRwsetBytes)
	s.assert.NoError(err)
	txAndPvtdata := &txAndPvtdata{Txid: s.txid, Envelope: envelope, Pvtws: simRes.PvtSimulationResults}
	return txAndPvtdata
}
