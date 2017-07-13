/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package chaincode

import (
	"context"

	"fmt"

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// ccProviderFactory implements the ccprovider.ChaincodeProviderFactory
// interface and returns instances of ccprovider.ChaincodeProvider
type ccProviderFactory struct {
}

// NewChaincodeProvider returns pointers to ccProviderImpl as an
// implementer of the ccprovider.ChaincodeProvider interface
func (c *ccProviderFactory) NewChaincodeProvider() ccprovider.ChaincodeProvider {
	return &ccProviderImpl{}
}

// init is called when this package is loaded. This implementation registers the factory
func init() {
	ccprovider.RegisterChaincodeProviderFactory(&ccProviderFactory{})
}

// ccProviderImpl is an implementation of the ccprovider.ChaincodeProvider interface
type ccProviderImpl struct {
	txsim ledger.TxSimulator
}

// ccProviderContextImpl contains the state that is passed around to calls to methods of ccProviderImpl
type ccProviderContextImpl struct {
	ctx *ccprovider.CCContext
}

// GetContext returns a context for the supplied ledger, with the appropriate tx simulator
func (c *ccProviderImpl) GetContext(ledger ledger.PeerLedger) (context.Context, error) {
	var err error
	// get context for the chaincode execution
	c.txsim, err = ledger.NewTxSimulator()
	if err != nil {
		return nil, err
	}
	ctxt := context.WithValue(context.Background(), TXSimulatorKey, c.txsim)
	return ctxt, nil
}

// GetCCContext returns an interface that encapsulates a
// chaincode context; the interface is required to avoid
// referencing the chaincode package from the interface definition
func (c *ccProviderImpl) GetCCContext(cid, name, version, txid string, syscc bool, signedProp *pb.SignedProposal, prop *pb.Proposal) interface{} {
	ctx := ccprovider.NewCCContext(cid, name, version, txid, syscc, signedProp, prop)
	return &ccProviderContextImpl{ctx: ctx}
}

// GetCCValidationInfoFromLSCC returns the VSCC and the policy listed in LSCC for the supplied chaincode
func (c *ccProviderImpl) GetCCValidationInfoFromLSCC(ctxt context.Context, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal, chainID string, chaincodeID string) (string, []byte, error) {
	// LSCC does not have any notion about its own
	// endorsing policy - we should never call this
	// function with lscc as the chaincodeID
	if chaincodeID == "lscc" {
		panic("GetCCValidationInfoFromLSCC invoke for LSCC")
	}

	data, err := GetChaincodeDataFromLSCC(ctxt, txid, signedProp, prop, chainID, chaincodeID)
	if err != nil {
		return "", nil, err
	}

	if data == nil || data.Vscc == "" || data.Policy == nil {
		return "", nil, fmt.Errorf("Incorrect validation info in LSCC")
	}

	return data.Vscc, data.Policy, nil
}

// ExecuteChaincode executes the chaincode specified in the context with the specified arguments
func (c *ccProviderImpl) ExecuteChaincode(ctxt context.Context, cccid interface{}, args [][]byte) (*pb.Response, *pb.ChaincodeEvent, error) {
	return ExecuteChaincode(ctxt, cccid.(*ccProviderContextImpl).ctx, args)
}

// Execute executes the chaincode given context and spec (invocation or deploy)
func (c *ccProviderImpl) Execute(ctxt context.Context, cccid interface{}, spec interface{}) (*pb.Response, *pb.ChaincodeEvent, error) {
	return Execute(ctxt, cccid.(*ccProviderContextImpl).ctx, spec)
}

// ExecuteWithErrorFilder executes the chaincode given context and spec and returns payload
func (c *ccProviderImpl) ExecuteWithErrorFilter(ctxt context.Context, cccid interface{}, spec interface{}) ([]byte, *pb.ChaincodeEvent, error) {
	return ExecuteWithErrorFilter(ctxt, cccid.(*ccProviderContextImpl).ctx, spec)
}

// ExecuteWithErrorFilder executes the chaincode given context and spec and returns payload
func (c *ccProviderImpl) Stop(ctxt context.Context, cccid interface{}, spec *pb.ChaincodeDeploymentSpec) error {
	if theChaincodeSupport != nil {
		return theChaincodeSupport.Stop(ctxt, cccid.(*ccProviderContextImpl).ctx, spec)
	}
	panic("ChaincodeSupport not initialized")
}

// ReleaseContext frees up resources held by the context
func (c *ccProviderImpl) ReleaseContext() {
	c.txsim.Done()
}
