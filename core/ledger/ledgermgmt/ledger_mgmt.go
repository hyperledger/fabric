/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgermgmt

import (
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("ledgermgmt")

// ErrLedgerAlreadyOpened is thrown by a CreateLedger call if a ledger with the given id is already opened
var ErrLedgerAlreadyOpened = errors.New("ledger already opened")

// ErrLedgerMgmtNotInitialized is thrown when ledger mgmt is used before initializing this
var ErrLedgerMgmtNotInitialized = errors.New("ledger mgmt should be initialized before using")

var openedLedgers map[string]ledger.PeerLedger
var ledgerProvider ledger.PeerLedgerProvider
var lock sync.Mutex
var initialized bool
var once sync.Once

// Initializer encapsulates all the external dependencies for the ledger module
type Initializer struct {
	CustomTxProcessors            customtx.Processors
	PlatformRegistry              *platforms.Registry
	DeployedChaincodeInfoProvider ledger.DeployedChaincodeInfoProvider
	MembershipInfoProvider        ledger.MembershipInfoProvider
}

// Initialize initializes ledgermgmt
func Initialize(initializer *Initializer) {
	once.Do(func() {
		initialize(initializer)
	})
}

func initialize(initializer *Initializer) {
	logger.Info("Initializing ledger mgmt")
	lock.Lock()
	defer lock.Unlock()
	initialized = true
	openedLedgers = make(map[string]ledger.PeerLedger)
	customtx.Initialize(initializer.CustomTxProcessors)
	cceventmgmt.Initialize(initializer.PlatformRegistry)
	finalStateListeners := addListenerForCCEventsHandler([]ledger.StateListener{})
	provider, err := kvledger.NewProvider()
	if err != nil {
		panic(errors.WithMessage(err, "Error in instantiating ledger provider"))
	}
	provider.Initialize(&ledger.Initializer{
		StateListeners:                finalStateListeners,
		DeployedChaincodeInfoProvider: initializer.DeployedChaincodeInfoProvider,
		MembershipInfoProvider:        initializer.MembershipInfoProvider,
	})

	ledgerProvider = provider
	logger.Info("ledger mgmt initialized")
}

// CreateLedger creates a new ledger with the given genesis block.
// This function guarantees that the creation of ledger and committing the genesis block would an atomic action
// The chain id retrieved from the genesis block is treated as a ledger id
func CreateLedger(genesisBlock *common.Block) (ledger.PeerLedger, error) {
	lock.Lock()
	defer lock.Unlock()
	if !initialized {
		return nil, ErrLedgerMgmtNotInitialized
	}
	id, err := utils.GetChainIDFromBlock(genesisBlock)
	if err != nil {
		return nil, err
	}

	logger.Infof("Creating ledger [%s] with genesis block", id)
	l, err := ledgerProvider.Create(genesisBlock)
	if err != nil {
		return nil, err
	}
	l = wrapLedger(id, l)
	openedLedgers[id] = l
	logger.Infof("Created ledger [%s] with genesis block", id)
	return l, nil
}

// OpenLedger returns a ledger for the given id
func OpenLedger(id string) (ledger.PeerLedger, error) {
	logger.Infof("Opening ledger with id = %s", id)
	lock.Lock()
	defer lock.Unlock()
	if !initialized {
		return nil, ErrLedgerMgmtNotInitialized
	}
	l, ok := openedLedgers[id]
	if ok {
		return nil, ErrLedgerAlreadyOpened
	}
	l, err := ledgerProvider.Open(id)
	if err != nil {
		return nil, err
	}
	l = wrapLedger(id, l)
	openedLedgers[id] = l
	logger.Infof("Opened ledger with id = %s", id)
	return l, nil
}

// GetLedgerIDs returns the ids of the ledgers created
func GetLedgerIDs() ([]string, error) {
	lock.Lock()
	defer lock.Unlock()
	if !initialized {
		return nil, ErrLedgerMgmtNotInitialized
	}
	return ledgerProvider.List()
}

// Close closes all the opened ledgers and any resources held for ledger management
func Close() {
	logger.Infof("Closing ledger mgmt")
	lock.Lock()
	defer lock.Unlock()
	if !initialized {
		return
	}
	for _, l := range openedLedgers {
		l.(*closableLedger).closeWithoutLock()
	}
	ledgerProvider.Close()
	openedLedgers = nil
	logger.Infof("ledger mgmt closed")
}

func wrapLedger(id string, l ledger.PeerLedger) ledger.PeerLedger {
	return &closableLedger{id, l}
}

// closableLedger extends from actual validated ledger and overwrites the Close method
type closableLedger struct {
	id string
	ledger.PeerLedger
}

// Close closes the actual ledger and removes the entries from opened ledgers map
func (l *closableLedger) Close() {
	lock.Lock()
	defer lock.Unlock()
	l.closeWithoutLock()
}

func (l *closableLedger) closeWithoutLock() {
	l.PeerLedger.Close()
	delete(openedLedgers, l.id)
}

// lscc namespace listener for chaincode instantiate transactions (which manipulates data in 'lscc' namespace)
// this code should be later moved to peer and passed via `Initialize` function of ledgermgmt
func addListenerForCCEventsHandler(stateListeners []ledger.StateListener) []ledger.StateListener {
	return append(stateListeners, &cceventmgmt.KVLedgerLSCCStateListener{})
}
