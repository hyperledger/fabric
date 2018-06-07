/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lscc

import (
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

type supportImpl struct {
}

// PutChaincodeToLocalStorage stores the supplied chaincode
// package to local storage (i.e. the file system)
func (s *supportImpl) PutChaincodeToLocalStorage(ccpack ccprovider.CCPackage) error {
	if err := ccpack.PutChaincodeToFS(); err != nil {
		return errors.Errorf("error installing chaincode code %s:%s(%s)", ccpack.GetChaincodeData().CCName(), ccpack.GetChaincodeData().CCVersion(), err)
	}

	return nil
}

// GetChaincodeFromLocalStorage retrieves the chaincode package
// for the requested chaincode, specified by name and version
func (s *supportImpl) GetChaincodeFromLocalStorage(ccname string, ccversion string) (ccprovider.CCPackage, error) {
	return ccprovider.GetChaincodeFromFS(ccname, ccversion)
}

// GetChaincodesFromLocalStorage returns an array of all chaincode
// data that have previously been persisted to local storage
func (s *supportImpl) GetChaincodesFromLocalStorage() (*pb.ChaincodeQueryResponse, error) {
	return ccprovider.GetInstalledChaincodes()
}

// GetInstantiationPolicy returns the instantiation policy for the
// supplied chaincode (or the channel's default if none was specified)
func (s *supportImpl) GetInstantiationPolicy(channel string, ccpack ccprovider.CCPackage) ([]byte, error) {
	var ip []byte
	var err error
	// if ccpack is a SignedCDSPackage, return its IP, otherwise use a default IP
	sccpack, isSccpack := ccpack.(*ccprovider.SignedCDSPackage)
	if isSccpack {
		ip = sccpack.GetInstantiationPolicy()
		if ip == nil {
			return nil, errors.Errorf("instantiation policy cannot be nil for a SignedCCDeploymentSpec")
		}
	} else {
		// the default instantiation policy allows any of the channel MSP admins
		// to be able to instantiate
		mspids := peer.GetMSPIDs(channel)

		p := cauthdsl.SignedByAnyAdmin(mspids)
		ip, err = utils.Marshal(p)
		if err != nil {
			return nil, errors.Errorf("error marshalling default instantiation policy")
		}

	}
	return ip, nil
}

// CheckInstantiationPolicy checks whether the supplied signed proposal
// complies with the supplied instantiation policy
func (s *supportImpl) CheckInstantiationPolicy(signedProp *pb.SignedProposal, chainName string, instantiationPolicy []byte) error {
	// create a policy object from the policy bytes
	mgr := mgmt.GetManagerForChain(chainName)
	if mgr == nil {
		return errors.Errorf("error checking chaincode instantiation policy: MSP manager for channel %s not found", chainName)
	}
	npp := cauthdsl.NewPolicyProvider(mgr)
	instPol, _, err := npp.NewPolicy(instantiationPolicy)
	if err != nil {
		return err
	}
	proposal, err := utils.GetProposal(signedProp.ProposalBytes)
	if err != nil {
		return err
	}
	// get the signature header of the proposal
	header, err := utils.GetHeader(proposal.Header)
	if err != nil {
		return err
	}
	shdr, err := utils.GetSignatureHeader(header.SignatureHeader)
	if err != nil {
		return err
	}
	// construct signed data we can evaluate the instantiation policy against
	sd := []*common.SignedData{{
		Data:      signedProp.ProposalBytes,
		Identity:  shdr.Creator,
		Signature: signedProp.Signature,
	}}
	err = instPol.Evaluate(sd)
	if err != nil {
		return errors.WithMessage(err, "instantiation policy violation")
	}
	return nil
}
