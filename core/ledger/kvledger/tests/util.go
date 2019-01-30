/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	mmsp "github.com/hyperledger/fabric/common/mocks/msp"
	lutils "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	protopeer "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
)

var logger = flogging.MustGetLogger("test2")

// collConf helps writing tests with less verbose code by specifying coll configuration
// in a simple struct in place of 'common.CollectionConfigPackage'. (the test heplers' apis
// use 'collConf' as parameters and return values and transform back and forth to/from proto
// message internally (using func 'convertToCollConfigProtoBytes' and 'convertFromCollConfigProto')
type collConf struct {
	name    string
	btl     uint64
	members []string
}

type txAndPvtdata struct {
	Txid     string
	Envelope *common.Envelope
	Pvtws    *rwset.TxPvtReadWriteSet
}

func convertToCollConfigProtoBytes(collConfs []*collConf) ([]byte, error) {
	var protoConfArray []*common.CollectionConfig
	for _, c := range collConfs {
		protoConf := &common.CollectionConfig{
			Payload: &common.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &common.StaticCollectionConfig{
					Name:             c.name,
					BlockToLive:      c.btl,
					MemberOrgsPolicy: convertToMemberOrgsPolicy(c.members),
				},
			},
		}
		protoConfArray = append(protoConfArray, protoConf)
	}
	return proto.Marshal(&common.CollectionConfigPackage{Config: protoConfArray})
}

func convertToMemberOrgsPolicy(members []string) *common.CollectionPolicyConfig {
	var data [][]byte
	for _, member := range members {
		data = append(data, []byte(member))
	}
	return &common.CollectionPolicyConfig{
		Payload: &common.CollectionPolicyConfig_SignaturePolicy{
			SignaturePolicy: cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), data),
		},
	}
}

func convertFromMemberOrgsPolicy(policy *common.CollectionPolicyConfig) []string {
	ids := policy.GetSignaturePolicy().Identities
	var members []string
	for _, id := range ids {
		members = append(members, string(id.Principal))
	}
	return members
}

func convertFromCollConfigProto(collConfPkg *common.CollectionConfigPackage) []*collConf {
	var collConfs []*collConf
	protoConfArray := collConfPkg.Config
	for _, protoConf := range protoConfArray {
		p := protoConf.GetStaticCollectionConfig()
		collConfs = append(collConfs,
			&collConf{
				name:    p.Name,
				btl:     p.BlockToLive,
				members: convertFromMemberOrgsPolicy(p.MemberOrgsPolicy),
			},
		)
	}
	return collConfs
}

func constructTransaction(txid string, simulationResults []byte) (*common.Envelope, error) {
	channelid := "dummyChannel"
	ccid := &protopeer.ChaincodeID{
		Name:    "dummyCC",
		Version: "dummyVer",
	}
	txenv, _, err := constructUnsignedTxEnv(channelid, ccid, &protopeer.Response{Status: 200}, simulationResults, txid, nil, nil)
	return txenv, err
}

// constructUnsignedTxEnv creates a Transaction envelope from given inputs
func constructUnsignedTxEnv(chainID string, ccid *protopeer.ChaincodeID, response *protopeer.Response, simulationResults []byte, txid string, events []byte, visibility []byte) (*common.Envelope, string, error) {
	mspLcl := mmsp.NewNoopMsp()
	sigId, _ := mspLcl.GetDefaultSigningIdentity()

	ss, err := sigId.Serialize()
	if err != nil {
		return nil, "", err
	}

	var prop *protopeer.Proposal
	if txid == "" {
		// if txid is not set, then we need to generate one while creating the proposal message
		prop, txid, err = protoutil.CreateChaincodeProposal(common.HeaderType_ENDORSER_TRANSACTION, chainID, &protopeer.ChaincodeInvocationSpec{ChaincodeSpec: &protopeer.ChaincodeSpec{ChaincodeId: ccid}}, ss)

	} else {
		// if txid is set, we should not generate a txid instead reuse the given txid
		nonce, err := crypto.GetRandomNonce()
		if err != nil {
			return nil, "", err
		}
		prop, txid, err = protoutil.CreateChaincodeProposalWithTxIDNonceAndTransient(txid, common.HeaderType_ENDORSER_TRANSACTION, chainID, &protopeer.ChaincodeInvocationSpec{ChaincodeSpec: &protopeer.ChaincodeSpec{ChaincodeId: ccid}}, nonce, ss, nil)
	}
	if err != nil {
		return nil, "", err
	}

	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response, simulationResults, nil, ccid, nil, sigId)
	if err != nil {
		return nil, "", err
	}

	env, err := protoutil.CreateSignedTx(prop, sigId, presp)
	if err != nil {
		return nil, "", err
	}
	return env, txid, nil
}

func constructTestGenesisBlock(channelid string) (*common.Block, error) {
	blk, err := configtxtest.MakeGenesisBlock(channelid)
	if err != nil {
		return nil, err
	}
	setBlockFlagsToValid(blk)
	return blk, nil
}

func setBlockFlagsToValid(block *common.Block) {
	protoutil.InitBlockMetadata(block)
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] =
		lutils.NewTxValidationFlagsSetValue(len(block.Data.Data), protopeer.TxValidationCode_VALID)
}
