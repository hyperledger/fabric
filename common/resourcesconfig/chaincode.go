/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/pkg/errors"
)

type chaincodeProtos struct {
	ChaincodeIdentifier  *pb.ChaincodeIdentifier
	ChaincodeValidation  *pb.ChaincodeValidation
	ChaincodeEndorsement *pb.ChaincodeEndorsement
}

// ChaincodeGroup represents the ConfigGroup named Chaincodes off the resources group
type ChaincodeGroup struct {
	name   string
	protos *chaincodeProtos
}

// Name returns the name of this chaincode (the name it was put in the ChaincodeRegistry with).
func (cg *ChaincodeGroup) CCName() string {
	return cg.name
}

// Hash returns the hash of the chaincode.
func (cg *ChaincodeGroup) Hash() []byte {
	return cg.protos.ChaincodeIdentifier.Hash
}

// Version returns the version of the chaincode.
func (cg *ChaincodeGroup) CCVersion() string {
	return cg.protos.ChaincodeIdentifier.Version
}

// Validation returns how to validate transactions for this chaincode.
// The string returned is the name of the validation method (usually 'vscc')
// and the bytes returned are the argument to the validation (in the case of
// 'vscc', this is a marshaled pb.VSCCArgs message).
func (cg *ChaincodeGroup) Validation() (string, []byte) {
	return cg.protos.ChaincodeValidation.Name, cg.protos.ChaincodeValidation.Argument
}

// Endorsement returns how to endorse proposals for this chaincode.
// The string returns is the name of the endorsement method (usually 'escc').
func (cg *ChaincodeGroup) Endorsement() string {
	return cg.protos.ChaincodeEndorsement.Name
}

func newChaincodeGroup(name string, group *cb.ConfigGroup) (*ChaincodeGroup, error) {
	if len(group.Groups) > 0 {
		return nil, errors.New("chaincode group does not support sub-groups")
	}

	protos := &chaincodeProtos{}
	if err := channelconfig.DeserializeProtoValuesFromGroup(group, protos); err != nil {
		logger.Panicf("Programming error in structure definition: %s", err)
	}

	return &ChaincodeGroup{
		name:   name,
		protos: protos,
	}, nil
}
