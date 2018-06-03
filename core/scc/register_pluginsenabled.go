// +build pluginsenabled,cgo
// +build darwin,go1.10 linux,go1.10 linux,go1.9,!ppc64le

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

// CreatePluginSysCCs creates all of the system chaincodes which are loaded by plugin
func CreatePluginSysCCs(p *Provider) []SelfDescribingSysCC {
	var sdscs []SelfDescribingSysCC
	for _, pscc := range loadSysCCs(p) {
		sdscs = append(sdscs, &SysCCWrapper{SCC: pscc})
	}
	return sdscs
}
