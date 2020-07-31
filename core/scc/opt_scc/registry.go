/*
 Copyright 2020 Intel Corporation

SPDX-License-Identifier: Apache-2.0
*/

package opt_scc

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
)

var logger = flogging.MustGetLogger("opt_scc")
var factories = make([]FactoryFunc, 0)

// FactoryFunc defines the type for factory function registring an optional system chaincode
type FactoryFunc func(aclProvider aclmgmt.ACLProvider, p *peer.Peer) scc.SelfDescribingSysCC

// AddFactoryFunc allows to register a factory function for an optional system chaincode
func AddFactoryFunc(ff FactoryFunc) {
	if ff == nil {
		logger.Panic("Illegal factory function")
	}
	factories = append(factories, ff)
}

//ListFactoryFuncs lists all registered factory functions
func ListFactoryFuncs() []FactoryFunc {
	logger.Debugf("ListFactoryFuncs: %v", factories)
	return factories
}
