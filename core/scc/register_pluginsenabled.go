// +build pluginsenabled,cgo
// +build darwin,go1.10 linux,go1.10 linux,go1.9,!ppc64le

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/peer"
)

//RegisterSysCCs is the hook for system chaincodes where system chaincodes are registered with the fabric
//note the chaincode must still be deployed and launched like a user chaincode will be
// TODO, this is named poorly, it should actually return only the provider, and not do side-effect
// initialization for registration.  To be tacked in a future CR.
func RegisterSysCCs() sysccprovider.SystemChaincodeProvider {
	systemChaincodes = append(systemChaincodes, loadSysCCs()...)

	sccp := &ProviderImpl{
		Peer:        peer.Default,
		PeerSupport: peer.DefaultSupport,
	}

	for _, sysCC := range systemChaincodes {
		registerSysCC(sysCC, sccp)
	}

	return sccp
}
