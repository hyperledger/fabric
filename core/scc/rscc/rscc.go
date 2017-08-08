/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rscc

import (
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

const (
	//CHANNEL name
	CHANNEL = "channel"

	//POLICY for the channel
	POLICY = "policy"

	//Init Errors (for UT)

	//NOCHANNEL channel not found
	NOCHANNEL = "nochannel"

	//NOPOLICY policy not found for channel
	NOPOLICY = "nopolicy"

	//BADPOLICY bad policy
	BADPOLICY = "badpolicy"
)

//the basic policyProvider for the channel, consists of a
//default and rscc policy providers
type policyProvider struct {
	//maybe we should use a separate default policy provider
	//than the only from aclmgmt (which is 1.0 defaults)
	defaultProvider aclmgmt.ACLProvider

	rsccProvider rsccPolicyProvider
}

//Rscc SCC implementing resouce->Policy mapping for the fabric
type Rscc struct {
	sync.RWMutex

	//the cache of RSCC policies for all the channels
	policyCache map[string]*policyProvider
}

var rsccLogger = flogging.MustGetLogger("rscc")

//NewRscc get an initialzed new Rscc
func NewRscc() *Rscc {
	return &Rscc{policyCache: make(map[string]*policyProvider)}
}

//--------- errors ---------

//NoPolicyProviderInCache in cache for channel
type NoPolicyProviderInCache string

func (e NoPolicyProviderInCache) Error() string {
	return fmt.Sprintf("cannot find policy provider in cache for channel %s", string(e))
}

//PolicyProviderNotFound for channel
type PolicyProviderNotFound string

func (e PolicyProviderNotFound) Error() string {
	return fmt.Sprintf("cannot find policy provider for channel %s", string(e))
}

//-------- ACLProvider interface ------

//CheckACL rscc implements AClProvider's CheckACL interface. This is the key interface
//   . CheckACL works off the cache
//   . CheckACL uses two providers - the RSCC provider from channel config and default provider
//     that implements 1.0 functions
//   . If a resource in RSCC Provider it'll use the policy defined there. Otherwise it'll defer
//     to default provider
func (rscc *Rscc) CheckACL(resName string, channelID string, idinfo interface{}) error {
	rsccLogger.Debugf("acl check(%s, %s)", resName, channelID)
	rscc.RLock()
	defer rscc.RUnlock()
	pp := rscc.policyCache[channelID]
	if pp == nil {
		return NoPolicyProviderInCache(channelID)
	}

	//found policyProvider
	if pp.rsccProvider != nil {
		//get the policy mapping if any
		if policyName := pp.rsccProvider.GetPolicyName(resName); policyName != "" {
			return pp.rsccProvider.CheckACL(policyName, idinfo)
		}
	}

	//try default provider
	if pp.defaultProvider != nil {
		return pp.defaultProvider.CheckACL(resName, channelID, idinfo)
	}

	return PolicyProviderNotFound(channelID)
}

//---------- misc functions ---------
func (rscc *Rscc) putPolicyProvider(channel string, pp *policyProvider) {
	rscc.Lock()
	defer rscc.Unlock()
	rscc.policyCache[channel] = pp
}

func (rscc *Rscc) setPolicyProvider(channel string, defProv aclmgmt.ACLProvider, rsccProv rsccPolicyProvider) {
	pp := &policyProvider{defProv, rsccProv}
	rscc.putPolicyProvider(channel, pp)
}

//-------- CC interface ------------

// Init RSCC - Init is just used to initialize policy assuming it is found in the
// channel ledger. Don't return error and get out. For example, we might still serve
// Invokes to set policy
func (rscc *Rscc) Init(stub shim.ChaincodeStubInterface) pb.Response {
	rsccLogger.Info("Init RSCC")

	b, err := stub.GetState(CHANNEL)
	if err != nil || len(b) == 0 {
		rsccLogger.Errorf("cannot find channel name")
		return shim.Success([]byte(NOCHANNEL))
	}

	channel := string(b)

	defProv := aclmgmt.NewDefaultACLProvider()

	b, err = stub.GetState(POLICY)
	if err != nil || len(b) == 0 {
		//we do not have policy, create just defaults
		rscc.setPolicyProvider(channel, defProv, nil)
		rsccLogger.Errorf("cannot find policy for channel %s", channel)
		return shim.Success([]byte(NOPOLICY))
	}

	//TODO this is a place holder.. will change based on what is stored in
	//ledger
	cg := &common.ConfigGroup{}
	err = proto.Unmarshal(b, cg)
	if err != nil {
		rscc.setPolicyProvider(channel, defProv, nil)
		rsccLogger.Errorf("cannot unmarshal policy for channel %s", channel)
		return shim.Success([]byte(BADPOLICY))
	}

	rsccpp, err := newRsccPolicyProvider(cg)
	if err != nil {
		rsccLogger.Errorf("cannot create policy provider for channel %s", channel)
	}

	rscc.setPolicyProvider(channel, defProv, rsccpp)

	return shim.Success(nil)
}

//Invoke - update policies
func (rscc *Rscc) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Error("--TBD---")
}
