/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package cscc chaincode configer provides functions to manage
// configuration transactions as the network is being reconfigured. The
// configuration transactions arrive from the ordering service to the committer
// who calls this chaincode. The chaincode also provides peer configuration
// services such as joining a chain or getting configuration data.
package cscc

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/plugindispatcher"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// New creates a new instance of the CSCC.
// Typically, only one will be created per peer instance.
func New(
	sccp sysccprovider.SystemChaincodeProvider,
	aclProvider aclmgmt.ACLProvider,
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider,
	lr plugindispatcher.LifecycleResources,
	nr plugindispatcher.CollectionAndLifecycleResources,
	policyChecker policy.PolicyChecker,
) *PeerConfiger {
	return &PeerConfiger{
		policyChecker:          policyChecker,
		configMgr:              peer.NewConfigSupport(),
		sccp:                   sccp,
		aclProvider:            aclProvider,
		deployedCCInfoProvider: deployedCCInfoProvider,
		legacyLifecycle:        lr,
		newLifecycle:           nr,
		peer:                   peer.Default,
	}
}

func (e *PeerConfiger) Name() string              { return "cscc" }
func (e *PeerConfiger) Path() string              { return "github.com/hyperledger/fabric/core/scc/cscc" }
func (e *PeerConfiger) InitArgs() [][]byte        { return nil }
func (e *PeerConfiger) Chaincode() shim.Chaincode { return e }
func (e *PeerConfiger) InvokableExternal() bool   { return true }
func (e *PeerConfiger) InvokableCC2CC() bool      { return false }
func (e *PeerConfiger) Enabled() bool             { return true }

// PeerConfiger implements the configuration handler for the peer. For every
// configuration transaction coming in from the ordering service, the
// committer calls this system chaincode to process the transaction.
type PeerConfiger struct {
	policyChecker          policy.PolicyChecker
	configMgr              config.Manager
	sccp                   sysccprovider.SystemChaincodeProvider
	aclProvider            aclmgmt.ACLProvider
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider
	legacyLifecycle        plugindispatcher.LifecycleResources
	newLifecycle           plugindispatcher.CollectionAndLifecycleResources
	peer                   *peer.Peer
}

var cnflogger = flogging.MustGetLogger("cscc")

// These are function names from Invoke first parameter
const (
	JoinChain      string = "JoinChain"
	GetConfigBlock string = "GetConfigBlock"
	GetChannels    string = "GetChannels"
)

// Init is mostly useless from an SCC perspective
func (e *PeerConfiger) Init(stub shim.ChaincodeStubInterface) pb.Response {
	cnflogger.Info("Init CSCC")
	return shim.Success(nil)
}

// Invoke is called for the following:
// # to process joining a chain (called by app as a transaction proposal)
// # to get the current configuration block (called by app)
// # to update the configuration block (called by committer)
// Peer calls this function with 2 arguments:
// # args[0] is the function name, which must be JoinChain, GetConfigBlock or
// UpdateConfigBlock
// # args[1] is a configuration Block if args[0] is JoinChain or
// UpdateConfigBlock; otherwise it is the chain id
// TODO: Improve the scc interface to avoid marshal/unmarshal args
func (e *PeerConfiger) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()

	if len(args) < 1 {
		return shim.Error(fmt.Sprintf("Incorrect number of arguments, %d", len(args)))
	}

	fname := string(args[0])

	if fname != GetChannels && len(args) < 2 {
		return shim.Error(fmt.Sprintf("Incorrect number of arguments, %d", len(args)))
	}

	cnflogger.Debugf("Invoke function: %s", fname)

	// Handle ACL:
	// 1. get the signed proposal
	sp, err := stub.GetSignedProposal()
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed getting signed proposal from stub: [%s]", err))
	}

	return e.InvokeNoShim(args, sp)
}

func (e *PeerConfiger) InvokeNoShim(args [][]byte, sp *pb.SignedProposal) pb.Response {
	var err error
	fname := string(args[0])

	switch fname {
	case JoinChain:
		if args[1] == nil {
			return shim.Error("Cannot join the channel <nil> configuration block provided")
		}

		block, err := protoutil.GetBlockFromBlockBytes(args[1])
		if err != nil {
			return shim.Error(fmt.Sprintf("Failed to reconstruct the genesis block, %s", err))
		}

		cid, err := protoutil.GetChainIDFromBlock(block)
		if err != nil {
			return shim.Error(fmt.Sprintf("\"JoinChain\" request failed to extract "+
				"channel id from the block due to [%s]", err))
		}

		// 1. check config block's format and capabilities requirement.
		if err := validateConfigBlock(block); err != nil {
			return shim.Error(fmt.Sprintf("\"JoinChain\" for chainID = %s failed because of validation "+
				"of configuration block, because of %s", cid, err))
		}

		// 2. check join policy.
		if err = e.aclProvider.CheckACL(resources.Cscc_JoinChain, "", sp); err != nil {
			return shim.Error(fmt.Sprintf("access denied for [%s][%s]: [%s]", fname, cid, err))
		}

		// Initialize txsFilter if it does not yet exist. We can do this safely since
		// it's the genesis block anyway
		txsFilter := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
		if len(txsFilter) == 0 {
			// add array of validation code hardcoded to valid
			txsFilter = util.NewTxValidationFlagsSetValue(len(block.Data.Data), pb.TxValidationCode_VALID)
			block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter
		}

		return e.joinChain(cid, block, e.sccp, e.deployedCCInfoProvider, e.legacyLifecycle, e.newLifecycle)
	case GetConfigBlock:
		// 2. check policy
		if err = e.aclProvider.CheckACL(resources.Cscc_GetConfigBlock, string(args[1]), sp); err != nil {
			return shim.Error(fmt.Sprintf("access denied for [%s][%s]: %s", fname, args[1], err))
		}

		return e.getConfigBlock(args[1])
	case GetChannels:
		// 2. check get channels policy
		if err = e.aclProvider.CheckACL(resources.Cscc_GetChannels, "", sp); err != nil {
			return shim.Error(fmt.Sprintf("access denied for [%s]: %s", fname, err))
		}

		return e.getChannels()

	}
	return shim.Error(fmt.Sprintf("Requested function %s not found.", fname))
}

// validateConfigBlock validate configuration block to see whenever it's contains valid config transaction
func validateConfigBlock(block *common.Block) error {
	envelopeConfig, err := protoutil.ExtractEnvelope(block, 0)
	if err != nil {
		return errors.Errorf("Failed to %s", err)
	}

	configEnv := &common.ConfigEnvelope{}
	_, err = protoutil.UnmarshalEnvelopeOfType(envelopeConfig, common.HeaderType_CONFIG, configEnv)
	if err != nil {
		return errors.Errorf("Bad configuration envelope: %s", err)
	}

	if configEnv.Config == nil {
		return errors.New("Nil config envelope Config")
	}

	if configEnv.Config.ChannelGroup == nil {
		return errors.New("Nil channel group")
	}

	if configEnv.Config.ChannelGroup.Groups == nil {
		return errors.New("No channel configuration groups are available")
	}

	_, exists := configEnv.Config.ChannelGroup.Groups[channelconfig.ApplicationGroupKey]
	if !exists {
		return errors.Errorf("Invalid configuration block, missing %s "+
			"configuration group", channelconfig.ApplicationGroupKey)
	}

	// Check the capabilities requirement
	if err = channelconfig.ValidateCapabilities(block); err != nil {
		return errors.Errorf("Failed capabilities check: [%s]", err)
	}

	return nil
}

// joinChain will join the specified chain in the configuration block.
// Since it is the first block, it is the genesis block containing configuration
// for this chain, so we want to update the Chain object with this info
func (e *PeerConfiger) joinChain(
	chainID string,
	block *common.Block,
	sccp sysccprovider.SystemChaincodeProvider,
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider,
	lr plugindispatcher.LifecycleResources,
	nr plugindispatcher.CollectionAndLifecycleResources,
) pb.Response {
	if err := e.peer.CreateChannel(block, sccp, deployedCCInfoProvider, lr, nr); err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

// Return the current configuration block for the specified chainID. If the
// peer doesn't belong to the chain, return error
func (e *PeerConfiger) getConfigBlock(chainID []byte) pb.Response {
	if chainID == nil {
		return shim.Error("ChainID must not be nil.")
	}
	block := e.peer.GetCurrConfigBlock(string(chainID))
	if block == nil {
		return shim.Error(fmt.Sprintf("Unknown chain ID, %s", string(chainID)))
	}
	blockBytes, err := protoutil.Marshal(block)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(blockBytes)
}

func (e *PeerConfiger) supportByType(chainID []byte, env *common.Envelope) (config.Config, error) {
	payload := &common.Payload{}

	if err := proto.Unmarshal(env.Payload, payload); err != nil {
		return nil, errors.Errorf("failed unmarshaling payload: %v", err)
	}

	channelHdr := &common.ChannelHeader{}
	if err := proto.Unmarshal(payload.Header.ChannelHeader, channelHdr); err != nil {
		return nil, errors.Errorf("failed unmarshaling payload header: %v", err)
	}

	switch common.HeaderType(channelHdr.Type) {
	case common.HeaderType_CONFIG_UPDATE:
		return e.configMgr.GetChannelConfig(string(chainID)), nil
	}
	return nil, errors.Errorf("invalid payload header type: %d", channelHdr.Type)
}

// getChannels returns information about all channels for this peer
func (e *PeerConfiger) getChannels() pb.Response {
	channelInfoArray := e.peer.GetChannelsInfo()

	// add array with info about all channels for this peer
	cqr := &pb.ChannelQueryResponse{Channels: channelInfoArray}

	cqrbytes, err := proto.Marshal(cqr)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(cqrbytes)
}
