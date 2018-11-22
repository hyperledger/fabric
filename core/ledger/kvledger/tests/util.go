/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/flogging"
	lutils "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	protopeer "github.com/hyperledger/fabric/protos/peer"
	prototestutils "github.com/hyperledger/fabric/protos/testutils"
	"github.com/hyperledger/fabric/protos/utils"
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
	txenv, _, err := prototestutils.ConstructUnsignedTxEnv(channelid, ccid, &protopeer.Response{Status: 200}, simulationResults, txid, nil, nil)
	return txenv, err
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
	utils.InitBlockMetadata(block)
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] =
		lutils.NewTxValidationFlagsSetValue(len(block.Data.Data), protopeer.TxValidationCode_VALID)
}
