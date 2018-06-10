/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// ccProviderImpl is an implementation of the ccprovider.ChaincodeProvider interface
type CCProviderImpl struct {
	cs *ChaincodeSupport
}

func NewProvider(cs *ChaincodeSupport) *CCProviderImpl {
	return &CCProviderImpl{cs: cs}
}

// Execute executes the chaincode given context and spec (invocation or deploy)
func (c *CCProviderImpl) Execute(txParams *ccprovider.TransactionParams, cccid *ccprovider.CCContext, input *pb.ChaincodeInput) (*pb.Response, *pb.ChaincodeEvent, error) {
	return c.cs.Execute(txParams, cccid, input)
}

// ExecuteLegacyInit executes a chaincode which is not in the LSCC table
func (c *CCProviderImpl) ExecuteLegacyInit(txParams *ccprovider.TransactionParams, cccid *ccprovider.CCContext, spec *pb.ChaincodeDeploymentSpec) (*pb.Response, *pb.ChaincodeEvent, error) {
	return c.cs.ExecuteLegacyInit(txParams, cccid, spec)
}

// Stop stops the chaincode given context and spec
func (c *CCProviderImpl) Stop(ccci *ccprovider.ChaincodeContainerInfo) error {
	return c.cs.Stop(ccci)
}
