/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aclmgmt

import (
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
)

var aclLogger = flogging.MustGetLogger("aclmgmt")

type ACLProvider interface {
	//CheckACL checks the ACL for the resource for the channel using the
	//idinfo. idinfo is an object such as SignedProposal from which an
	//id can be extracted for testing against a policy
	CheckACL(resName string, channelID string, idinfo interface{}) error
}

//---------- custom tx processor initialized once by peer -------
var configtxLock sync.RWMutex

//---------- ACLProvider intialized once SCCs are brought up by peer ---------
var aclProvider ACLProvider

var once sync.Once

//---------- ACLProvider intialized once SCCs are brought up by peer ---------
//RegisterACLProvider will be called to register an ACLProvider.
//Users can write their own ACLProvider and register. If not provided,
//the standard resource based ACLProvider will be created and registered
func RegisterACLProvider(prov ACLProvider) {
	once.Do(func() {
		configtxLock.Lock()
		defer configtxLock.Unlock()

		//if an external prov is not supplied, create
		//a resource based ACLProvider and register
		if aclProvider = prov; aclProvider == nil {
			aclProvider = newACLMgmt(nil)
		}
	})
}

//GetACLProvider returns ACLProvider
func GetACLProvider() ACLProvider {
	if aclProvider == nil {
		panic("-----RegisterACLProvider not called -----")
	}
	return aclProvider
}
