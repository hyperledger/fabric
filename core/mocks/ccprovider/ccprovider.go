package ccprovider

import (
	"context"

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/peer"
)

// MockCcProviderFactory is a factory that returns
// mock implementations of the ccprovider.ChaincodeProvider interface
type MockCcProviderFactory struct {
}

// NewChaincodeProvider returns a mock implementation of the ccprovider.ChaincodeProvider interface
func (c *MockCcProviderFactory) NewChaincodeProvider() ccprovider.ChaincodeProvider {
	return &mockCcProviderImpl{}
}

// mockCcProviderImpl is a mock implementation of the chaincode provider
type mockCcProviderImpl struct {
}

type mockCcProviderContextImpl struct {
}

// GetContext does nothing
func (c *mockCcProviderImpl) GetContext(ledger ledger.ValidatedLedger) (context.Context, error) {
	return nil, nil
}

// GetCCContext does nothing
func (c *mockCcProviderImpl) GetCCContext(cid, name, version, txid string, syscc bool, prop *peer.Proposal) interface{} {
	return &mockCcProviderContextImpl{}
}

// GetVSCCFromLCCC does nothing
func (c *mockCcProviderImpl) GetVSCCFromLCCC(ctxt context.Context, txid string, prop *peer.Proposal, chainID string, chaincodeID string) (string, error) {
	return "vscc", nil
}

// ExecuteChaincode does nothing
func (c *mockCcProviderImpl) ExecuteChaincode(ctxt context.Context, cccid interface{}, args [][]byte) ([]byte, *peer.ChaincodeEvent, error) {
	return nil, nil, nil
}

// ReleaseContext does nothing
func (c *mockCcProviderImpl) ReleaseContext() {
}
