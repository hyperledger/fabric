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
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/events/producer"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

// PeerConfiger implements the configuration handler for the peer. For every
// configuration transaction coming in from the ordering service, the
// committer calls this system chaincode to process the transaction.
type PeerConfiger struct {
	policyChecker policy.PolicyChecker
	configMgr     config.Manager
}

var cnflogger = flogging.MustGetLogger("cscc")

// These are function names from Invoke first parameter
const (
	JoinChain                string = "JoinChain"
	GetConfigBlock           string = "GetConfigBlock"
	GetChannels              string = "GetChannels"
	GetConfigTree            string = "GetConfigTree"
	SimulateConfigTreeUpdate string = "SimulateConfigTreeUpdate"
)

// Init is called once per chain when the chain is created.
// This allows the chaincode to initialize any variables on the ledger prior
// to any transaction execution on the chain.
func (e *PeerConfiger) Init(stub shim.ChaincodeStubInterface) pb.Response {
	cnflogger.Info("Init CSCC")

	// Init policy checker for access control
	e.policyChecker = policy.NewPolicyChecker(
		peer.NewChannelPolicyManagerGetter(),
		mgmt.GetLocalMSP(),
		mgmt.NewLocalMSPPrincipalGetter(),
	)
	e.configMgr = peer.NewConfigSupport()
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

	switch fname {
	case JoinChain:
		if args[1] == nil {
			return shim.Error("Cannot join the channel <nil> configuration block provided")
		}

		block, err := utils.GetBlockFromBlockBytes(args[1])
		if err != nil {
			return shim.Error(fmt.Sprintf("Failed to reconstruct the genesis block, %s", err))
		}

		cid, err := utils.GetChainIDFromBlock(block)
		if err != nil {
			return shim.Error(fmt.Sprintf("\"JoinChain\" request failed to extract "+
				"channel id from the block due to [%s]", err))
		}

		if err := validateConfigBlock(block); err != nil {
			return shim.Error(fmt.Sprintf("\"JoinChain\" for chainID = %s failed because of validation "+
				"of configuration block, because of %s", cid, err))
		}

		// 2. check local MSP Admins policy
		// TODO: move to ACLProvider once it will support chainless ACLs
		if err = e.policyChecker.CheckPolicyNoChannel(mgmt.Admins, sp); err != nil {
			return shim.Error(fmt.Sprintf("\"JoinChain\" request failed authorization check "+
				"for channel [%s]: [%s]", cid, err))
		}

		return joinChain(cid, block)
	case GetConfigBlock:
		// 2. check policy
		if err = aclmgmt.GetACLProvider().CheckACL(resources.CSCC_GetConfigBlock, string(args[1]), sp); err != nil {
			return shim.Error(fmt.Sprintf("\"GetConfigBlock\" request failed authorization check for channel [%s]: [%s]", args[1], err))
		}

		return getConfigBlock(args[1])
	case GetConfigTree:
		// 2. check policy
		if err = aclmgmt.GetACLProvider().CheckACL(resources.CSCC_GetConfigTree, string(args[1]), sp); err != nil {
			return shim.Error(fmt.Sprintf("\"GetConfigTree\" request failed authorization check for channel [%s]: [%s]", args[1], err))
		}

		return e.getConfigTree(args[1])
	case SimulateConfigTreeUpdate:
		// Check policy
		if err = aclmgmt.GetACLProvider().CheckACL(resources.CSCC_SimulateConfigTreeUpdate, string(args[1]), sp); err != nil {
			return shim.Error(fmt.Sprintf("\"SimulateConfigTreeUpdate\" request failed authorization check for channel [%s]: [%s]", args[1], err))
		}
		return e.simulateConfigTreeUpdate(args[1], args[2])
	case GetChannels:
		// 2. check local MSP Members policy
		// TODO: move to ACLProvider once it will support chainless ACLs
		if err = e.policyChecker.CheckPolicyNoChannel(mgmt.Members, sp); err != nil {
			return shim.Error(fmt.Sprintf("\"GetChannels\" request failed authorization check: [%s]", err))
		}

		return getChannels()

	}
	return shim.Error(fmt.Sprintf("Requested function %s not found.", fname))
}

// validateConfigBlock validate configuration block to see whenever it's contains valid config transaction
func validateConfigBlock(block *common.Block) error {
	envelopeConfig, err := utils.ExtractEnvelope(block, 0)
	if err != nil {
		return errors.Errorf("Failed to %s", err)
	}

	configEnv := &common.ConfigEnvelope{}
	_, err = utils.UnmarshalEnvelopeOfType(envelopeConfig, common.HeaderType_CONFIG, configEnv)
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

	return nil
}

// joinChain will join the specified chain in the configuration block.
// Since it is the first block, it is the genesis block containing configuration
// for this chain, so we want to update the Chain object with this info
func joinChain(chainID string, block *common.Block) pb.Response {
	if err := peer.CreateChainFromBlock(block); err != nil {
		return shim.Error(err.Error())
	}

	peer.InitChain(chainID)

	bevent, _, _, err := producer.CreateBlockEvents(block)
	if err != nil {
		cnflogger.Errorf("Error processing block events for block number [%d]: %s", block.Header.Number, err)
	} else {
		if err := producer.Send(bevent); err != nil {
			cnflogger.Errorf("Channel [%s] Error sending block event for block number [%d]: %s", chainID, block.Header.Number, err)
		}
	}

	return shim.Success(nil)
}

// Return the current configuration block for the specified chainID. If the
// peer doesn't belong to the chain, return error
func getConfigBlock(chainID []byte) pb.Response {
	if chainID == nil {
		return shim.Error("ChainID must not be nil.")
	}
	block := peer.GetCurrConfigBlock(string(chainID))
	if block == nil {
		return shim.Error(fmt.Sprintf("Unknown chain ID, %s", string(chainID)))
	}
	blockBytes, err := utils.Marshal(block)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(blockBytes)
}

// getConfigTree returns the current channel and resources configuration for the specified chainID.
// If the peer doesn't belong to the chain, returns error
func (e *PeerConfiger) getConfigTree(chainID []byte) pb.Response {
	if chainID == nil {
		return shim.Error("Chain ID must not be nil")
	}
	channelCfg := e.configMgr.GetChannelConfig(string(chainID)).ConfigProto()
	if channelCfg == nil {
		return shim.Error(fmt.Sprintf("Unknown chain ID, %s", string(chainID)))
	}
	resCfg := e.configMgr.GetResourceConfig(string(chainID)).ConfigProto()
	if resCfg == nil {
		return shim.Error(fmt.Sprintf("Unknown chain ID, %s", string(chainID)))
	}
	agCfg := &pb.ConfigTree{ChannelConfig: channelCfg, ResourcesConfig: resCfg}
	configBytes, err := utils.Marshal(agCfg)
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success(configBytes)
}

func (e *PeerConfiger) simulateConfigTreeUpdate(chainID []byte, envb []byte) pb.Response {
	if chainID == nil {
		return shim.Error("Chain ID must not be nil")
	}
	if envb == nil {
		return shim.Error("Config delta bytes must not be nil")
	}
	env := &common.Envelope{}
	err := proto.Unmarshal(envb, env)
	if err != nil {
		return shim.Error(err.Error())
	}
	cfg, err := supportByType(e, chainID, env)
	if err != nil {
		return shim.Error(err.Error())
	}
	_, err = cfg.ProposeConfigUpdate(env)
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success([]byte("Simulation is successful"))
}

func supportByType(pc *PeerConfiger, chainID []byte, env *common.Envelope) (config.Config, error) {
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
		return pc.configMgr.GetChannelConfig(string(chainID)), nil
	case common.HeaderType_PEER_RESOURCE_UPDATE:
		return pc.configMgr.GetResourceConfig(string(chainID)), nil
	}
	return nil, errors.Errorf("invalid payload header type: %d", channelHdr.Type)
}

// getChannels returns information about all channels for this peer
func getChannels() pb.Response {
	channelInfoArray := peer.GetChannelsInfo()

	// add array with info about all channels for this peer
	cqr := &pb.ChannelQueryResponse{Channels: channelInfoArray}

	cqrbytes, err := proto.Marshal(cqr)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(cqrbytes)
}
