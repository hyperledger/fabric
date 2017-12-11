/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import "github.com/hyperledger/fabric/protos/gossip"

// InstalledChaincode defines metadata about an installed chaincode
type InstalledChaincode struct {
	Name    string
	Version string
	Id      []byte
}

// InstantiatedChaincode defines channel-scoped metadata of a chaincode
type InstantiatedChaincode struct {
	Name    string
	Version string
	Policy  []byte
	Id      []byte
}

// InstantiatedChaincodes defines an aggregation of InstantiatedChaincodes
type InstantiatedChaincodes []InstantiatedChaincode

// ToChaincodes converts this InstantiatedChaincodes to a slice of gossip.Chaincodes
func (ccs InstantiatedChaincodes) ToChaincodes() []*gossip.Chaincode {
	var res []*gossip.Chaincode
	for _, cc := range ccs {
		res = append(res, &gossip.Chaincode{
			Name:    cc.Name,
			Version: cc.Version,
		})
	}
	return res
}
