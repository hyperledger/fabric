/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgermgmt

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("ledgermgmt")

// ErrLedgerAlreadyOpened is thrown by a CreateLedger call if a ledger with the given id is already opened
var ErrLedgerAlreadyOpened = errors.New("ledger already opened")

// ErrLedgerMgmtNotInitialized is thrown when ledger mgmt is used before initializing this
var ErrLedgerMgmtNotInitialized = errors.New("ledger mgmt should be initialized before using")

// LedgerMgr manages ledgers for all channels
type LedgerMgr struct {
	lock               sync.Mutex
	openedLedgers      map[string]ledger.PeerLedger
	ledgerProvider     ledger.PeerLedgerProvider
	ebMetadataProvider MetadataProvider
}

type MetadataProvider interface {
	PackageMetadata(ccid string) ([]byte, error)
}

// Initializer encapsulates all the external dependencies for the ledger module
type Initializer struct {
	CustomTxProcessors              map[common.HeaderType]ledger.CustomTxProcessor
	StateListeners                  []ledger.StateListener
	DeployedChaincodeInfoProvider   ledger.DeployedChaincodeInfoProvider
	MembershipInfoProvider          ledger.MembershipInfoProvider
	ChaincodeLifecycleEventProvider ledger.ChaincodeLifecycleEventProvider
	MetricsProvider                 metrics.Provider
	HealthCheckRegistry             ledger.HealthCheckRegistry
	Config                          *ledger.Config
	Hasher                          ledger.Hasher
	EbMetadataProvider              MetadataProvider
}

// NewLedgerMgr creates a new LedgerMgr
func NewLedgerMgr(initializer *Initializer) *LedgerMgr {
	logger.Info("Initializing LedgerMgr")
	finalStateListeners := addListenerForCCEventsHandler(
		initializer.DeployedChaincodeInfoProvider,
		initializer.StateListeners,
	)
	provider, err := kvledger.NewProvider(
		&ledger.Initializer{
			StateListeners:                  finalStateListeners,
			DeployedChaincodeInfoProvider:   initializer.DeployedChaincodeInfoProvider,
			MembershipInfoProvider:          initializer.MembershipInfoProvider,
			ChaincodeLifecycleEventProvider: initializer.ChaincodeLifecycleEventProvider,
			MetricsProvider:                 initializer.MetricsProvider,
			HealthCheckRegistry:             initializer.HealthCheckRegistry,
			Config:                          initializer.Config,
			CustomTxProcessors:              initializer.CustomTxProcessors,
			Hasher:                          initializer.Hasher,
		},
	)
	if err != nil {
		panic(fmt.Sprintf("Error in instantiating ledger provider: %s", err))
	}
	ledgerMgr := &LedgerMgr{
		openedLedgers:      make(map[string]ledger.PeerLedger),
		ledgerProvider:     provider,
		ebMetadataProvider: initializer.EbMetadataProvider,
	}
	// TODO remove the following package level init
	cceventmgmt.Initialize(&chaincodeInfoProviderImpl{
		ledgerMgr,
		initializer.DeployedChaincodeInfoProvider,
	})
	logger.Info("Initialized LedgerMgr")
	return ledgerMgr
}

// CreateLedger creates a new ledger with the given genesis block.
// This function guarantees that the creation of ledger and committing the genesis block would an atomic action
// The chain id retrieved from the genesis block is treated as a ledger id
func (m *LedgerMgr) CreateLedger(id string, genesisBlock *common.Block) (ledger.PeerLedger, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	logger.Infof("Creating ledger [%s] with genesis block", id)
	l, err := m.ledgerProvider.Create(genesisBlock)
	if err != nil {
		return nil, err
	}
	m.openedLedgers[id] = l
	logger.Infof("Created ledger [%s] with genesis block", id)
	return &closableLedger{
		ledgerMgr:  m,
		id:         id,
		PeerLedger: l,
	}, nil
}

// OpenLedger returns a ledger for the given id
func (m *LedgerMgr) OpenLedger(id string) (ledger.PeerLedger, error) {
	logger.Infof("Opening ledger with id = %s", id)
	m.lock.Lock()
	defer m.lock.Unlock()
	_, ok := m.openedLedgers[id]
	if ok {
		return nil, ErrLedgerAlreadyOpened
	}
	l, err := m.ledgerProvider.Open(id)
	if err != nil {
		return nil, err
	}
	m.openedLedgers[id] = l
	logger.Infof("Opened ledger with id = %s", id)
	return &closableLedger{
		ledgerMgr:  m,
		id:         id,
		PeerLedger: l,
	}, nil
}

// GetLedgerIDs returns the ids of the ledgers created
func (m *LedgerMgr) GetLedgerIDs() ([]string, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.ledgerProvider.List()
}

// Close closes all the opened ledgers and any resources held for ledger management
func (m *LedgerMgr) Close() {
	logger.Infof("Closing ledger mgmt")
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, l := range m.openedLedgers {
		l.Close()
	}
	m.ledgerProvider.Close()
	m.openedLedgers = nil
	logger.Infof("ledger mgmt closed")
}

func (m *LedgerMgr) getOpenedLedger(ledgerID string) (ledger.PeerLedger, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	l, ok := m.openedLedgers[ledgerID]
	if !ok {
		return nil, errors.Errorf("Ledger not opened [%s]", ledgerID)
	}
	return l, nil
}

func (m *LedgerMgr) closeLedger(ledgerID string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	l, ok := m.openedLedgers[ledgerID]
	if ok {
		l.Close()
		delete(m.openedLedgers, ledgerID)
	}
}

// closableLedger extends from actual validated ledger and overwrites the Close method
type closableLedger struct {
	ledgerMgr *LedgerMgr
	id        string
	ledger.PeerLedger
}

// Close closes the actual ledger and removes the entries from opened ledgers map
func (l *closableLedger) Close() {
	l.ledgerMgr.closeLedger(l.id)
}

// lscc namespace listener for chaincode instantiate transactions (which manipulates data in 'lscc' namespace)
// this code should be later moved to peer and passed via `Initialize` function of ledgermgmt
func addListenerForCCEventsHandler(
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider,
	stateListeners []ledger.StateListener) []ledger.StateListener {
	return append(stateListeners, &cceventmgmt.KVLedgerLSCCStateListener{DeployedChaincodeInfoProvider: deployedCCInfoProvider})
}

// chaincodeInfoProviderImpl implements interface cceventmgmt.ChaincodeInfoProvider
type chaincodeInfoProviderImpl struct {
	ledgerMgr              *LedgerMgr
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider
}

// GetDeployedChaincodeInfo implements function in the interface cceventmgmt.ChaincodeInfoProvider
func (p *chaincodeInfoProviderImpl) GetDeployedChaincodeInfo(chainid string,
	chaincodeDefinition *cceventmgmt.ChaincodeDefinition) (*ledger.DeployedChaincodeInfo, error) {
	ledger, err := p.ledgerMgr.getOpenedLedger(chainid)
	if err != nil {
		return nil, err
	}
	qe, err := ledger.NewQueryExecutor()
	if err != nil {
		return nil, err
	}
	defer qe.Done()
	deployedChaincodeInfo, err := p.deployedCCInfoProvider.ChaincodeInfo(chainid, chaincodeDefinition.Name, qe)
	if err != nil || deployedChaincodeInfo == nil {
		return nil, err
	}
	if deployedChaincodeInfo.Version != chaincodeDefinition.Version ||
		!bytes.Equal(deployedChaincodeInfo.Hash, chaincodeDefinition.Hash) {
		// if the deployed chaincode with the given name has different version or different hash, return nil
		return nil, nil
	}
	return deployedChaincodeInfo, nil
}

// RetrieveChaincodeArtifacts implements function in the interface cceventmgmt.ChaincodeInfoProvider
func (p *chaincodeInfoProviderImpl) RetrieveChaincodeArtifacts(chaincodeDefinition *cceventmgmt.ChaincodeDefinition) (installed bool, dbArtifactsTar []byte, err error) {
	ccid := chaincodeDefinition.Name + ":" + chaincodeDefinition.Version
	md, err := p.ledgerMgr.ebMetadataProvider.PackageMetadata(ccid)
	if err != nil {
		return false, nil, err
	}
	if md != nil {
		return true, md, nil
	}
	return ccprovider.ExtractStatedbArtifactsForChaincode(ccid)
}
