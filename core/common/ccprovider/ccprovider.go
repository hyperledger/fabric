package ccprovider

import (
	"context"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/peer"
)

// ChaincodeProvider provides an abstraction layer that is
// used for different packages to interact with code in the
// chaincode package without importing it; more methods
// should be added below if necessary
type ChaincodeProvider interface {
	// GetContext returns a ledger context
	GetContext(ledger ledger.ValidatedLedger) (context.Context, error)
	// GetCCContext returns an opaque chaincode context
	GetCCContext(cid, name, version, txid string, syscc bool, prop *peer.Proposal) interface{}
	// GetVSCCFromLCCC returns the VSCC listed by LCCC for the supplied chaincode
	GetVSCCFromLCCC(ctxt context.Context, txid string, prop *peer.Proposal, chainID string, chaincodeID string) (string, error)
	// ExecuteChaincode executes the chaincode given context and args
	ExecuteChaincode(ctxt context.Context, cccid interface{}, args [][]byte) ([]byte, *peer.ChaincodeEvent, error)
	// ReleaseContext releases the context returned previously by GetContext
	ReleaseContext()
}

var ccFactory ChaincodeProviderFactory

// ChaincodeProviderFactory defines a factory interface so
// that the actual implementation can be injected
type ChaincodeProviderFactory interface {
	NewChaincodeProvider() ChaincodeProvider
}

// RegisterChaincodeProviderFactory is to be called once to set
// the factory that will be used to obtain instances of ChaincodeProvider
func RegisterChaincodeProviderFactory(ccfact ChaincodeProviderFactory) {
	ccFactory = ccfact
}

// GetChaincodeProvider returns instances of ChaincodeProvider;
// the actual implementation is controlled by the factory that
// is registered via RegisterChaincodeProviderFactory
func GetChaincodeProvider() ChaincodeProvider {
	if ccFactory == nil {
		panic("The factory must be set first via RegisterChaincodeProviderFactory")
	}
	return ccFactory.NewChaincodeProvider()
}
