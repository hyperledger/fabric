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

package peer

import (
	"errors"
	"fmt"
	"math"
	"net"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

var peerLogger = logging.MustGetLogger("peer")

var peerServer comm.GRPCServer

// singleton instance to manage CAs for the peer across channel config changes
var rootCASupport = comm.GetCASupport()

type chainSupport struct {
	configtxapi.Manager
	config.Application
	ledger ledger.PeerLedger
}

func (cs *chainSupport) Ledger() ledger.PeerLedger {
	return cs.ledger
}

// chain is a local struct to manage objects in a chain
type chain struct {
	cs        *chainSupport
	cb        *common.Block
	committer committer.Committer
}

// chains is a local map of chainID->chainObject
var chains = struct {
	sync.RWMutex
	list map[string]*chain
}{list: make(map[string]*chain)}

//MockInitialize resets chains for test env
func MockInitialize() {
	ledgermgmt.InitializeTestEnv()
	chains.list = nil
	chains.list = make(map[string]*chain)
	chainInitializer = func(string) { return }
}

var chainInitializer func(string)

// Initialize sets up any chains that the peer has from the persistence. This
// function should be called at the start up when the ledger and gossip
// ready
func Initialize(init func(string)) {
	chainInitializer = init

	var cb *common.Block
	var ledger ledger.PeerLedger
	ledgermgmt.Initialize()
	ledgerIds, err := ledgermgmt.GetLedgerIDs()
	if err != nil {
		panic(fmt.Errorf("Error in initializing ledgermgmt: %s", err))
	}
	for _, cid := range ledgerIds {
		peerLogger.Infof("Loading chain %s", cid)
		if ledger, err = ledgermgmt.OpenLedger(cid); err != nil {
			peerLogger.Warningf("Failed to load ledger %s(%s)", cid, err)
			peerLogger.Debugf("Error while loading ledger %s with message %s. We continue to the next ledger rather than abort.", cid, err)
			continue
		}
		if cb, err = getCurrConfigBlockFromLedger(ledger); err != nil {
			peerLogger.Warningf("Failed to find config block on ledger %s(%s)", cid, err)
			peerLogger.Debugf("Error while looking for config block on ledger %s with message %s. We continue to the next ledger rather than abort.", cid, err)
			continue
		}
		// Create a chain if we get a valid ledger with config block
		if err = createChain(cid, ledger, cb); err != nil {
			peerLogger.Warningf("Failed to load chain %s(%s)", cid, err)
			peerLogger.Debugf("Error reloading chain %s with message %s. We continue to the next chain rather than abort.", cid, err)
			continue
		}

		InitChain(cid)
	}
}

// Take care to initialize chain after peer joined, for example deploys system CCs
func InitChain(cid string) {
	if chainInitializer != nil {
		// Initialize chaincode, namely deploy system CC
		peerLogger.Debugf("Init chain %s", cid)
		chainInitializer(cid)
	}
}

func getCurrConfigBlockFromLedger(ledger ledger.PeerLedger) (*common.Block, error) {
	// Config blocks contain only 1 transaction, so we look for 1-tx
	// blocks and check the transaction type
	var envelope *common.Envelope
	var tx *common.Payload
	var block *common.Block
	var err error
	var currBlockNumber uint64 = math.MaxUint64
	for currBlockNumber >= 0 {
		if block, err = ledger.GetBlockByNumber(currBlockNumber); err != nil {
			return nil, err
		}
		if block.Data != nil && len(block.Data.Data) == 1 {
			if envelope, err = utils.ExtractEnvelope(block, 0); err != nil {
				peerLogger.Warning("Failed to get Envelope from Block %d.", block.Header.Number)
				currBlockNumber = block.Header.Number - 1
				continue
			}
			if tx, err = utils.ExtractPayload(envelope); err != nil {
				peerLogger.Warning("Failed to get Payload from Block %d.", block.Header.Number)
				currBlockNumber = block.Header.Number - 1
				continue
			}
			chdr, err := utils.UnmarshalChannelHeader(tx.Header.ChannelHeader)
			if err != nil {
				peerLogger.Warning("Failed to get ChannelHeader from Block %d, error %s.", block.Header.Number, err)
				currBlockNumber = block.Header.Number - 1
				continue
			}
			if chdr.Type == int32(common.HeaderType_CONFIG) {
				return block, nil
			}
		}
		currBlockNumber = block.Header.Number - 1
	}
	return nil, fmt.Errorf("Failed to find config block.")
}

// createChain creates a new chain object and insert it into the chains
func createChain(cid string, ledger ledger.PeerLedger, cb *common.Block) error {

	envelopeConfig, err := utils.ExtractEnvelope(cb, 0)
	if err != nil {
		return err
	}

	configtxInitializer := configtx.NewInitializer()

	gossipEventer := service.GetGossipService().NewConfigEventer()

	gossipCallbackWrapper := func(cm configtxapi.Manager) {
		gossipEventer.ProcessConfigUpdate(&chainSupport{
			Manager:     cm,
			Application: configtxInitializer.ApplicationConfig(),
		})
	}

	trustedRootsCallbackWrapper := func(cm configtxapi.Manager) {
		updateTrustedRoots(cm)
	}

	configtxManager, err := configtx.NewManagerImpl(
		envelopeConfig,
		configtxInitializer,
		[]func(cm configtxapi.Manager){gossipCallbackWrapper, trustedRootsCallbackWrapper},
	)
	if err != nil {
		return err
	}

	// TODO remove once all references to mspmgmt are gone from peer code
	mspmgmt.XXXSetMSPManager(cid, configtxManager.MSPManager())

	cs := &chainSupport{
		Manager:     configtxManager,
		Application: configtxManager.ApplicationConfig(), // TODO, refactor as this is accessible through Manager
		ledger:      ledger,
	}

	c := committer.NewLedgerCommitter(ledger, txvalidator.NewTxValidator(cs))
	ordererAddresses := configtxManager.ChannelConfig().OrdererAddresses()
	if len(ordererAddresses) == 0 {
		return errors.New("No orderering service endpoint provided in configuration block")
	}
	service.GetGossipService().InitializeChannel(cs.ChainID(), c, ordererAddresses)

	chains.Lock()
	defer chains.Unlock()
	chains.list[cid] = &chain{
		cs:        cs,
		cb:        cb,
		committer: c,
	}
	return nil
}

// CreateChainFromBlock creates a new chain from config block
func CreateChainFromBlock(cb *common.Block) error {
	cid, err := utils.GetChainIDFromBlock(cb)
	if err != nil {
		return err
	}
	var ledger ledger.PeerLedger
	if ledger, err = createLedger(cid); err != nil {
		return err
	}

	if err := ledger.Commit(cb); err != nil {
		peerLogger.Errorf("Unable to get genesis block committed into the ledger, chainID %v", cid)
		return err
	}

	return createChain(cid, ledger, cb)
}

// MockCreateChain used for creating a ledger for a chain for tests
// without havin to join
func MockCreateChain(cid string) error {
	var ledger ledger.PeerLedger
	var err error
	if ledger, err = createLedger(cid); err != nil {
		return err
	}

	i := mockconfigtx.Initializer{
		Resources: mockconfigtx.Resources{
			PolicyManagerVal: &mockpolicies.Manager{
				Policy: &mockpolicies.Policy{},
			},
		},
	}

	chains.Lock()
	defer chains.Unlock()
	chains.list[cid] = &chain{
		cs: &chainSupport{
			ledger:  ledger,
			Manager: &mockconfigtx.Manager{Initializer: i},
		},
	}

	return nil
}

// GetLedger returns the ledger of the chain with chain ID. Note that this
// call returns nil if chain cid has not been created.
func GetLedger(cid string) ledger.PeerLedger {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cs.ledger
	}
	return nil
}

// GetPolicyManager returns the policy manager of the chain with chain ID. Note that this
// call returns nil if chain cid has not been created.
func GetPolicyManager(cid string) policies.Manager {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cs.PolicyManager()
	}
	return nil
}

// GetCurrConfigBlock returns the cached config block of the specified chain.
// Note that this call returns nil if chain cid has not been created.
func GetCurrConfigBlock(cid string) *common.Block {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cb
	}
	return nil
}

// updates the trusted roots for the peer based on updates to channels
func updateTrustedRoots(cm configtxapi.Manager) {
	// this is triggered on per channel basis so first update the roots for the channel
	peerLogger.Debugf("Updating trusted root authorities for channel %s", cm.ChainID())
	var secureConfig comm.SecureServerConfig
	var err error
	// only run is TLS is enabled
	secureConfig, err = GetSecureConfig()
	if err == nil && secureConfig.UseTLS {
		buildTrustedRootsForChain(cm)

		// now iterate over all roots for all app and orderer chains
		trustedRoots := [][]byte{}
		rootCASupport.RLock()
		defer rootCASupport.RUnlock()
		for _, roots := range rootCASupport.AppRootCAsByChain {
			trustedRoots = append(trustedRoots, roots...)
		}
		// also need to append statically configured root certs
		if len(secureConfig.ClientRootCAs) > 0 {
			trustedRoots = append(trustedRoots, secureConfig.ClientRootCAs...)
		}
		if len(secureConfig.ServerRootCAs) > 0 {
			trustedRoots = append(trustedRoots, secureConfig.ServerRootCAs...)
		}

		server := GetPeerServer()
		// now update the client roots for the peerServer
		if server != nil {
			err := server.SetClientRootCAs(trustedRoots)
			if err != nil {
				msg := "Failed to update trusted roots for peer from latest config " +
					"block.  This peer may not be able to communicate " +
					"with members of channel %s (%s)"
				peerLogger.Warningf(msg, cm.ChainID(), err)
			}
		}
	}
}

// populates the appRootCAs and orderRootCAs maps by getting the
// root and intermediate certs for all msps assocaited with the MSPManager
func buildTrustedRootsForChain(cm configtxapi.Manager) {
	rootCASupport.Lock()
	defer rootCASupport.Unlock()

	appRootCAs := [][]byte{}
	ordererRootCAs := [][]byte{}
	cid := cm.ChainID()
	msps, err := cm.MSPManager().GetMSPs()
	if err != nil {
		peerLogger.Errorf("Error getting getting root CA for channel %s (%s)", cid, err)
	}
	if err == nil {
		for _, v := range msps {
			// check to see if this is a FABRIC MSP
			if v.GetType() == msp.FABRIC {
				for _, root := range v.GetRootCerts() {
					sid, err := root.Serialize()
					if err == nil {
						id := &msp.SerializedIdentity{}
						err = proto.Unmarshal(sid, id)
						if err == nil {
							appRootCAs = append(appRootCAs, id.IdBytes)
						}
					}
				}
				for _, intermediate := range v.GetIntermediateCerts() {
					sid, err := intermediate.Serialize()
					if err == nil {
						id := &msp.SerializedIdentity{}
						err = proto.Unmarshal(sid, id)
						if err == nil {
							appRootCAs = append(appRootCAs, id.IdBytes)
						}
					}
				}
			}
		}
		// TODO: separate app and orderer CAs
		ordererRootCAs = appRootCAs
		rootCASupport.AppRootCAsByChain[cid] = appRootCAs
		rootCASupport.OrdererRootCAsByChain[cid] = ordererRootCAs
	}
}

// GetMSPIDs returns the ID of each application MSP defined on this chain
func GetMSPIDs(cid string) []string {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		if c == nil || c.cs == nil ||
			c.cs.ApplicationConfig() == nil ||
			c.cs.ApplicationConfig().Organizations() == nil {
			return nil
		}

		orgs := c.cs.ApplicationConfig().Organizations()
		toret := make([]string, len(orgs))
		i := 0
		for _, org := range orgs {
			toret[i] = org.MSPID()
			i++
		}

		return toret
	}
	return nil
}

// SetCurrConfigBlock sets the current config block of the specified chain
func SetCurrConfigBlock(block *common.Block, cid string) error {
	chains.Lock()
	defer chains.Unlock()
	if c, ok := chains.list[cid]; ok {
		c.cb = block
		// TODO: Change MSP config
		// c.mspmgr.Reconfig(block)

		// TODO: Change gossip configs
		return nil
	}
	return fmt.Errorf("Chain %s doesn't exist on the peer", cid)
}

// All ledgers are located under `peer.fileSystemPath`
func createLedger(cid string) (ledger.PeerLedger, error) {
	var ledger ledger.PeerLedger
	if ledger = GetLedger(cid); ledger != nil {
		return ledger, nil
	}
	return ledgermgmt.CreateLedger(cid)
}

// NewPeerClientConnection Returns a new grpc.ClientConn to the configured local PEER.
func NewPeerClientConnection() (*grpc.ClientConn, error) {
	return NewPeerClientConnectionWithAddress(viper.GetString("peer.address"))
}

// GetLocalIP returns the non loopback local IP of the host
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback then display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

// NewPeerClientConnectionWithAddress Returns a new grpc.ClientConn to the configured local PEER.
func NewPeerClientConnectionWithAddress(peerAddress string) (*grpc.ClientConn, error) {
	if comm.TLSEnabled() {
		return comm.NewClientConnectionWithAddress(peerAddress, true, true, comm.InitTLSForPeer())
	}
	return comm.NewClientConnectionWithAddress(peerAddress, true, false, nil)
}

// GetChannelsInfo returns an array with information about all channels for
// this peer
func GetChannelsInfo() []*pb.ChannelInfo {
	// array to store metadata for all channels
	var channelInfoArray []*pb.ChannelInfo

	chains.RLock()
	defer chains.RUnlock()
	for key := range chains.list {
		channelInfo := &pb.ChannelInfo{ChannelId: key}

		// add this specific chaincode's metadata to the array of all chaincodes
		channelInfoArray = append(channelInfoArray, channelInfo)
	}

	return channelInfoArray
}

// NewChannelPolicyManagerGetter returns a new instance of ChannelPolicyManagerGetter
func NewChannelPolicyManagerGetter() policies.ChannelPolicyManagerGetter {
	return &channelPolicyManagerGetter{}
}

type channelPolicyManagerGetter struct{}

func (c *channelPolicyManagerGetter) Manager(channelID string) (policies.Manager, bool) {
	policyManager := GetPolicyManager(channelID)
	return policyManager, policyManager != nil
}

// CreatePeerServer creates an instance of comm.GRPCServer
// This server is used for peer communications
func CreatePeerServer(listenAddress string,
	secureConfig comm.SecureServerConfig) (comm.GRPCServer, error) {

	var err error
	peerServer, err = comm.NewGRPCServer(listenAddress, secureConfig)
	if err != nil {
		peerLogger.Errorf("Failed to create peer server (%s)", err)
		return nil, err
	}
	return peerServer, nil
}

// GetPeerServer returns the peer server instance
func GetPeerServer() comm.GRPCServer {
	return peerServer
}
