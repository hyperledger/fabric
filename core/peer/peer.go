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
	"fmt"
	"math"
	"net"
	"sync"

	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/peer/sharedconfig"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

var peerLogger = logging.MustGetLogger("peer")

type chainSupport struct {
	configtx.Manager
	sharedconfig.Descriptor
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
			peerLogger.Debug("Error while loading ledger %s with message %s. We continue to the next ledger rather than abort.", cid, err)
			continue
		}
		if cb, err = getCurrConfigBlockFromLedger(ledger); err != nil {
			peerLogger.Warningf("Failed to find configuration block on ledger %s(%s)", cid, err)
			peerLogger.Debug("Error while looking for config block on ledger %s with message %s. We continue to the next ledger rather than abort.", cid, err)
			continue
		}
		// Create a chain if we get a valid ledger with config block
		if err = createChain(cid, ledger, cb); err != nil {
			peerLogger.Warningf("Failed to load chain %s(%s)", cid, err)
			peerLogger.Debug("Error reloading chain %s with message %s. We continue to the next chain rather than abort.", cid, err)
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
	// Configuration blocks contain only 1 transaction, so we look for 1-tx
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
			if tx.Header.ChainHeader.Type == int32(common.HeaderType_CONFIGURATION_TRANSACTION) {
				return block, nil
			}
		}
		currBlockNumber = block.Header.Number - 1
	}
	return nil, fmt.Errorf("Failed to find configuration block.")
}

// createChain creates a new chain object and insert it into the chains
func createChain(cid string, ledger ledger.PeerLedger, cb *common.Block) error {

	configEnvelope, err := utils.BreakOutBlockToConfigurationEnvelope(cb)
	if err != nil {
		return err
	}

	sharedConfigHandler := sharedconfig.NewDescriptorImpl()

	gossipEventer := service.GetGossipService().NewConfigEventer()

	gossipCallbackWrapper := func(cm configtx.Manager) {
		gossipEventer.ProcessConfigUpdate(&chainSupport{
			Manager:    cm,
			Descriptor: sharedConfigHandler,
		})
	}

	configtxInitializer := configtx.NewInitializer()
	configtxInitializer.Handlers()[common.ConfigurationItem_Peer] = sharedConfigHandler
	configtxManager, err := configtx.NewManagerImpl(
		configEnvelope,
		configtxInitializer,
		[]func(cm configtx.Manager){gossipCallbackWrapper},
	)
	if err != nil {
		return err
	}

	// TODO remove once all references to mspmgmt are gone from peer code
	// MSP is now initialized above in configtx.Manager
	_, err = mspmgmt.GetMSPManagerFromBlock(cid, cb)
	if err != nil {
		return err
	}

	cs := &chainSupport{
		Manager:    configtxManager,
		Descriptor: sharedConfigHandler,
		ledger:     ledger,
	}

	c := committer.NewLedgerCommitter(ledger, txvalidator.NewTxValidator(cs))
	service.GetGossipService().InitializeChannel(cs.ChainID(), c)

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

	chains.Lock()
	defer chains.Unlock()
	chains.list[cid] = &chain{cs: &chainSupport{ledger: ledger}}

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

// GetMSPMgr returns the MSP manager of the chain with chain ID.
// Note that this call returns nil if chain cid has not been created.
func GetMSPMgr(cid string) msp.MSPManager {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cs.MSPManager()
	}
	return nil
}

// GetCommitter returns the committer of the chain with chain ID. Note that this
// call returns nil if chain cid has not been created.
func GetCommitter(cid string) committer.Committer {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.committer
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

// SetCurrConfigBlock sets the current config block of the specified chain
func SetCurrConfigBlock(block *common.Block, cid string) error {
	chains.Lock()
	defer chains.Unlock()
	if c, ok := chains.list[cid]; ok {
		c.cb = block
		// TODO: Change MSP configuration
		// c.mspmgr.Reconfig(block)

		// TODO: Change gossip configurations
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
