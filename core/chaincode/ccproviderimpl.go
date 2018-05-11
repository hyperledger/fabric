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
func (c *CCProviderImpl) Execute(ctxt context.Context, cccid *ccprovider.CCContext, spec ccprovider.ChaincodeSpecGetter) (*pb.Response, *pb.ChaincodeEvent, error) {
	return c.cs.Execute(ctxt, cccid, spec)
}

// Stop stops the chaincode given context and spec
func (c *CCProviderImpl) Stop(ctxt context.Context, cccid *ccprovider.CCContext, spec *pb.ChaincodeDeploymentSpec) error {
	return c.cs.Stop(ctxt, cccid, spec)
}
