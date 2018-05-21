/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aclmgmt

import (
	"github.com/hyperledger/fabric/common/flogging"
)

var aclMgmtLogger = flogging.MustGetLogger("aclmgmt")

//implementation of aclMgmt. CheckACL calls in fabric result in the following flow
//    if resourceProvider[resourceName]
//       return resourceProvider[resourceName].CheckACL(...)
//    else
//       return defaultProvider[resourceName].CheckACL(...)
//with rescfgProvider encapsulating resourceProvider and defaultProvider
type aclMgmtImpl struct {
	//resource provider gets resource information from config
	rescfgProvider ACLProvider
}

//CheckACL checks the ACL for the resource for the channel using the
//idinfo. idinfo is an object such as SignedProposal from which an
//id can be extracted for testing against a policy
func (am *aclMgmtImpl) CheckACL(resName string, channelID string, idinfo interface{}) error {
	//use the resource based config provider (which will in turn default to 1.0 provider)
	return am.rescfgProvider.CheckACL(resName, channelID, idinfo)
}

//ACLProvider consists of two providers, supplied one and a default one (1.0 ACL management
//using ChannelReaders and ChannelWriters). If supplied provider is nil, a resource based
//ACL provider is created.
func NewACLProvider(rg ResourceGetter) ACLProvider {
	return &aclMgmtImpl{
		rescfgProvider: newResourceProvider(rg, NewDefaultACLProvider()),
	}
}
