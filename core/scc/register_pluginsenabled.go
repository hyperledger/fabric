// +build pluginsenabled,cgo
// +build darwin,go1.10 linux,go1.10 linux,go1.9,!ppc64le

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

// CreateSysCCs creates all of the system chaincodes which are compiled into fabric
// as well as those which are loaded by plugin
func CreateSysCCs(p *Provider) []*SystemChaincode {
	return append(builtInSystemChaincodes(p), loadSysCCs(p)...)
}
