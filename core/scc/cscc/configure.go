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
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/plugindispatcher"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// New creates a new instance of the CSCC.
// Typically, only one will be created per peer instance.
func New(
	aclProvider aclmgmt.ACLProvider,
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider,
	lr plugindispatcher.LifecycleResources,
	nr plugindispatcher.CollectionAndLifecycleResources,
	p *peer.Peer,
	bccsp bccsp.BCCSP,
) *PeerConfiger {
	return &PeerConfiger{
		aclProvider:            aclProvider,
		deployedCCInfoProvider: deployedCCInfoProvider,
		legacyLifecycle:        lr,
		newLifecycle:           nr,
		peer:                   p,
		bccsp:                  bccsp,
	}
}

func (e *PeerConfiger) Name() string              { return "cscc" }
func (e *PeerConfiger) Chaincode() shim.Chaincode { return e }

// PeerConfiger implements the configuration handler for the peer. For every
// configuration transaction coming in from the ordering service, the
// committer calls this system chaincode to process the transaction.
type PeerConfiger struct {
	aclProvider            aclmgmt.ACLProvider
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider
	legacyLifecycle        plugindispatcher.LifecycleResources
	newLifecycle           plugindispatcher.CollectionAndLifecycleResources
	peer                   *peer.Peer
	bccsp                  bccsp.BCCSP
}

var cnflogger = flogging.MustGetLogger("cscc")

// These are function names from Invoke first parameter
const (
	JoinChain            string = "JoinChain"
	JoinChainBySnapshot  string = "JoinChainBySnapshot"
	JoinBySnapshotStatus string = "JoinBySnapshotStatus"
	GetConfigBlock       string = "GetConfigBlock"
	GetChannelConfig     string = "GetChannelConfig"
	GetChannels          string = "GetChannels"
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

	if fname != GetChannels && fname != JoinBySnapshotStatus && len(args) < 2 {
		return shim.Error(fmt.Sprintf("Incorrect number of arguments, %d", len(args)))
	}

	cnflogger.Debugf("Invoke function: %s", fname)

	// Handle ACL:
	// 1. get the signed proposal
	sp, err := stub.GetSignedProposal()
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed getting signed proposal from stub: [%s]", err))
	}

	name, err := protoutil.InvokedChaincodeName(sp.ProposalBytes)
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed to identify the called chaincode: %s", err))
	}

	if name != e.Name() {
		return shim.Error(fmt.Sprintf("Rejecting invoke of CSCC from another chaincode, original invocation for '%s'", name))
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

		block, err := protoutil.UnmarshalBlock(args[1])
		if err != nil {
			return shim.Error(fmt.Sprintf("Failed to reconstruct the genesis block, %s", err))
		}

		cid, err := protoutil.GetChannelIDFromBlock(block)
		if err != nil {
			return shim.Error(fmt.Sprintf("\"JoinChain\" request failed to extract "+
				"channel id from the block due to [%s]", err))
		}

		// 1. check config block's format and capabilities requirement.
		if err := validateConfigBlock(block, e.bccsp); err != nil {
			return shim.Error(fmt.Sprintf("\"JoinChain\" for channelID = %s failed because of validation "+
				"of configuration block, because of %s", cid, err))
		}

		// 2. check join policy.
		if err = e.aclProvider.CheckACL(resources.Cscc_JoinChain, "", sp); err != nil {
			return shim.Error(fmt.Sprintf("access denied for [%s][%s]: [%s]", fname, cid, err))
		}

		// Initialize txsFilter if it does not yet exist. We can do this safely since
		// it's the genesis block anyway
		txsFilter := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
		if len(txsFilter) == 0 {
			// add array of validation code hardcoded to valid
			txsFilter = txflags.NewWithValues(len(block.Data.Data), pb.TxValidationCode_VALID)
			block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter
		}

		return e.joinChain(cid, block, e.deployedCCInfoProvider, e.legacyLifecycle, e.newLifecycle)
	case JoinChainBySnapshot:
		if len(args[1]) == 0 {
			return shim.Error("Cannot join the channel, no snapshot directory provided")
		}
		// check policy
		if err = e.aclProvider.CheckACL(resources.Cscc_JoinChainBySnapshot, "", sp); err != nil {
			return shim.Error(fmt.Sprintf("access denied for [%s]: [%s]", fname, err))
		}
		snapshotDir := string(args[1])
		return e.JoinChainBySnapshot(snapshotDir, e.deployedCCInfoProvider, e.legacyLifecycle, e.newLifecycle)
	case JoinBySnapshotStatus:
		if err = e.aclProvider.CheckACL(resources.Cscc_JoinBySnapshotStatus, "", sp); err != nil {
			return shim.Error(fmt.Sprintf("access denied for [%s]: %s", fname, err))
		}
		return e.joinBySnapshotStatus()
	case GetConfigBlock:
		// 2. check policy
		if err = e.aclProvider.CheckACL(resources.Cscc_GetConfigBlock, string(args[1]), sp); err != nil {
			return shim.Error(fmt.Sprintf("access denied for [%s][%s]: %s", fname, args[1], err))
		}

		return e.getConfigBlock(args[1])
	case GetChannelConfig:
		if len(args[1]) == 0 {
			return shim.Error("empty channel name provided")
		}
		if err = e.aclProvider.CheckACL(resources.Cscc_GetChannelConfig, string(args[1]), sp); err != nil {
			return shim.Error(fmt.Sprintf("access denied for [%s][%s]: %s", fname, args[1], err))
		}
		return e.getChannelConfig(args[1])
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
func validateConfigBlock(block *common.Block, bccsp bccsp.BCCSP) error {
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
	if err = channelconfig.ValidateCapabilities(block, bccsp); err != nil {
		return errors.Errorf("Failed capabilities check: [%s]", err)
	}

	return nil
}

// joinChain will join the specified chain in the configuration block.
// Since it is the first block, it is the genesis block containing configuration
// for this chain, so we want to update the Chain object with this info
func (e *PeerConfiger) joinChain(
	channelID string,
	block *common.Block,
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider,
	lr plugindispatcher.LifecycleResources,
	nr plugindispatcher.CollectionAndLifecycleResources,
) pb.Response {
	if err := e.peer.CreateChannel(channelID, block, deployedCCInfoProvider, lr, nr); err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

// JohnChainBySnapshot will join the channel by the specified snapshot.
func (e *PeerConfiger) JoinChainBySnapshot(
	snapshotDir string,
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider,
	lr plugindispatcher.LifecycleResources,
	nr plugindispatcher.CollectionAndLifecycleResources,
) pb.Response {
	if err := e.peer.CreateChannelFromSnapshot(snapshotDir, deployedCCInfoProvider, lr, nr); err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

// Return the current configuration block for the specified channelID. If the
// peer doesn't belong to the channel, return error
func (e *PeerConfiger) getConfigBlock(channelID []byte) pb.Response {
	if channelID == nil {
		return shim.Error("ChannelID must not be nil.")
	}

	channel := e.peer.Channel(string(channelID))
	if channel == nil {
		return shim.Error(fmt.Sprintf("Unknown channel ID, %s", string(channelID)))
	}
	block, err := peer.ConfigBlockFromLedger(channel.Ledger())
	if err != nil {
		return shim.Error(err.Error())
	}

	blockBytes, err := protoutil.Marshal(block)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(blockBytes)
}

func (e *PeerConfiger) getChannelConfig(channelID []byte) pb.Response {
	channel := e.peer.Channel(string(channelID))
	if channel == nil {
		return shim.Error(fmt.Sprintf("unknown channel ID, %s", string(channelID)))
	}
	channelConfig, err := peer.RetrievePersistedChannelConfig(channel.Ledger())
	if err != nil {
		return shim.Error(err.Error())
	}

	channelConfigBytes, err := protoutil.Marshal(channelConfig)
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success(channelConfigBytes)
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

// joinBySnapshotStatus returns information about joinbysnapshot running status.
func (e *PeerConfiger) joinBySnapshotStatus() pb.Response {
	status := e.peer.JoinBySnaphotStatus()

	statusBytes, err := proto.Marshal(status)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(statusBytes)
}
