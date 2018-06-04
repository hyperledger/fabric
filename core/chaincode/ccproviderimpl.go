/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// ccProviderImpl is an implementation of the ccprovider.ChaincodeProvider interface
type CCProviderImpl struct {
	cs *ChaincodeSupport
}

func NewProvider(cs *ChaincodeSupport) *CCProviderImpl {
	return &CCProviderImpl{cs: cs}
}

// GetContext returns a context for the supplied ledger, with the appropriate tx simulator
func (c *CCProviderImpl) GetContext(ledger ledger.PeerLedger, txid string) (context.Context, ledger.TxSimulator, error) {
	// get context for the chaincode execution
	txsim, err := ledger.NewTxSimulator(txid)
	if err != nil {
		return nil, nil, err
	}
	ctxt := context.WithValue(context.Background(), TXSimulatorKey, txsim)
	return ctxt, txsim, nil
}

// ExecuteChaincode executes the chaincode specified in the context with the specified arguments
func (c *CCProviderImpl) ExecuteChaincode(ctxt context.Context, cccid *ccprovider.CCContext, args [][]byte) (*pb.Response, *pb.ChaincodeEvent, error) {
	invocationSpec := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_GOLANG,
			ChaincodeId: &pb.ChaincodeID{Name: cccid.Name},
			Input:       &pb.ChaincodeInput{Args: args},
		},
	}
	return c.cs.Execute(ctxt, cccid, invocationSpec)
}

// Execute executes the chaincode given context and spec (invocation or deploy)
func (c *CCProviderImpl) Execute(ctxt context.Context, cccid *ccprovider.CCContext, spec *pb.ChaincodeInvocationSpec) (*pb.Response, *pb.ChaincodeEvent, error) {
	return c.cs.Execute(ctxt, cccid, spec)
}

// ExecuteInit executes the chaincode given context and spec (invocation or deploy)
func (c *CCProviderImpl) ExecuteInit(ctxt context.Context, cccid *ccprovider.CCContext, spec *pb.ChaincodeDeploymentSpec) (*pb.Response, *pb.ChaincodeEvent, error) {
	return c.cs.ExecuteInit(ctxt, cccid, spec)
}

// Stop stops the chaincode given context and spec
func (c *CCProviderImpl) Stop(cccid *ccprovider.CCContext, spec *pb.ChaincodeDeploymentSpec) error {
	return c.cs.Stop(cccid, spec)
}
