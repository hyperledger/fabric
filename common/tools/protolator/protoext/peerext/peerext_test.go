/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peerext_test

import (
	"github.com/hyperledger/fabric/common/tools/protolator"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/peerext"
)

// ensure structs implement expected interfaces
var (
	_ protolator.DynamicMapFieldProto       = &peerext.DynamicApplicationGroup{}
	_ protolator.DecoratedProto             = &peerext.DynamicApplicationGroup{}
	_ protolator.DynamicMapFieldProto       = &peerext.DynamicApplicationOrgGroup{}
	_ protolator.DecoratedProto             = &peerext.DynamicApplicationOrgGroup{}
	_ protolator.StaticallyOpaqueFieldProto = &peerext.DynamicApplicationConfigValue{}
	_ protolator.DecoratedProto             = &peerext.DynamicApplicationConfigValue{}
	_ protolator.StaticallyOpaqueFieldProto = &peerext.DynamicApplicationOrgConfigValue{}
	_ protolator.DecoratedProto             = &peerext.DynamicApplicationOrgConfigValue{}

	_ protolator.StaticallyOpaqueFieldProto = &peerext.ChaincodeProposalPayload{}
	_ protolator.DecoratedProto             = &peerext.ChaincodeProposalPayload{}
	_ protolator.StaticallyOpaqueFieldProto = &peerext.ChaincodeAction{}
	_ protolator.DecoratedProto             = &peerext.ChaincodeAction{}

	_ protolator.StaticallyOpaqueFieldProto = &peerext.ProposalResponsePayload{}
	_ protolator.DecoratedProto             = &peerext.ProposalResponsePayload{}

	_ protolator.StaticallyOpaqueFieldProto = &peerext.TransactionAction{}
	_ protolator.DecoratedProto             = &peerext.TransactionAction{}
	_ protolator.StaticallyOpaqueFieldProto = &peerext.ChaincodeActionPayload{}
	_ protolator.DecoratedProto             = &peerext.ChaincodeActionPayload{}
	_ protolator.StaticallyOpaqueFieldProto = &peerext.ChaincodeEndorsedAction{}
	_ protolator.DecoratedProto             = &peerext.ChaincodeEndorsedAction{}
)
