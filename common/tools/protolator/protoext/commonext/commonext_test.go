/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commonext_test

import (
	"github.com/hyperledger/fabric/common/tools/protolator"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/commonext"
)

// ensure structs implement expected interfaces
var (
	_ protolator.StaticallyOpaqueFieldProto      = &commonext.Envelope{}
	_ protolator.DecoratedProto                  = &commonext.Envelope{}
	_ protolator.VariablyOpaqueFieldProto        = &commonext.Payload{}
	_ protolator.DecoratedProto                  = &commonext.Payload{}
	_ protolator.StaticallyOpaqueFieldProto      = &commonext.Header{}
	_ protolator.DecoratedProto                  = &commonext.Header{}
	_ protolator.StaticallyOpaqueFieldProto      = &commonext.SignatureHeader{}
	_ protolator.DecoratedProto                  = &commonext.SignatureHeader{}
	_ protolator.StaticallyOpaqueSliceFieldProto = &commonext.BlockData{}
	_ protolator.DecoratedProto                  = &commonext.BlockData{}

	_ protolator.StaticallyOpaqueFieldProto    = &commonext.ConfigUpdateEnvelope{}
	_ protolator.DecoratedProto                = &commonext.ConfigUpdateEnvelope{}
	_ protolator.StaticallyOpaqueFieldProto    = &commonext.ConfigSignature{}
	_ protolator.DecoratedProto                = &commonext.ConfigSignature{}
	_ protolator.DynamicFieldProto             = &commonext.Config{}
	_ protolator.DecoratedProto                = &commonext.Config{}
	_ protolator.StaticallyOpaqueMapFieldProto = &commonext.ConfigUpdate{}
	_ protolator.DecoratedProto                = &commonext.ConfigUpdate{}

	_ protolator.DynamicMapFieldProto       = &commonext.DynamicChannelGroup{}
	_ protolator.DecoratedProto             = &commonext.DynamicChannelGroup{}
	_ protolator.StaticallyOpaqueFieldProto = &commonext.DynamicChannelConfigValue{}
	_ protolator.DecoratedProto             = &commonext.DynamicChannelConfigValue{}
	_ protolator.DynamicMapFieldProto       = &commonext.DynamicConsortiumsGroup{}
	_ protolator.DecoratedProto             = &commonext.DynamicConsortiumsGroup{}
	_ protolator.DynamicMapFieldProto       = &commonext.DynamicConsortiumGroup{}
	_ protolator.DecoratedProto             = &commonext.DynamicConsortiumGroup{}
	_ protolator.VariablyOpaqueFieldProto   = &commonext.DynamicConsortiumConfigValue{}
	_ protolator.DecoratedProto             = &commonext.DynamicConsortiumConfigValue{}
	_ protolator.DynamicMapFieldProto       = &commonext.DynamicConsortiumOrgGroup{}
	_ protolator.DecoratedProto             = &commonext.DynamicConsortiumOrgGroup{}
	_ protolator.StaticallyOpaqueFieldProto = &commonext.DynamicConsortiumOrgConfigValue{}
	_ protolator.DecoratedProto             = &commonext.DynamicConsortiumOrgConfigValue{}
)
