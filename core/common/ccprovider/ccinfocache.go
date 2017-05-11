/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ccprovider

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/protos/peer"
)

// ccInfoCacheImpl implements CCInfoProvider by providing an in-memory
// cache layer on top of the internal CCInfoProvider instance
type ccInfoCacheImpl struct {
	sync.RWMutex

	cache map[string]CCPackage
	ccfs  CCInfoProvider
}

// NewCCInfoCache returns a new cache on top of the supplied CCInfoProvider instance
func NewCCInfoCache(ccfs CCInfoProvider) CCInfoProvider {
	return &ccInfoCacheImpl{
		cache: make(map[string]CCPackage),
		ccfs:  ccfs,
	}
}

func (c *ccInfoCacheImpl) GetChaincode(ccname string, ccversion string) (CCPackage, error) {
	// c.cache is guaranteed to be non-nil

	key := ccname + "/" + ccversion

	c.RLock()
	ccpack, in := c.cache[key]
	c.RUnlock()

	if !in {
		var err error

		// the chaincode data is not in the cache
		// try to look it up from the file system
		ccpack, err = c.ccfs.GetChaincode(ccname, ccversion)
		if err != nil || ccpack == nil {
			return nil, fmt.Errorf("cannot retrieve package for chaincode %s/%s, error %s", ccname, ccversion, err)
		}

		// we have a non-nil CCPackage, put it in the cache
		c.Lock()
		c.cache[key] = ccpack
		c.Unlock()
	}

	return ccpack, nil
}

func (c *ccInfoCacheImpl) PutChaincode(depSpec *peer.ChaincodeDeploymentSpec) (CCPackage, error) {
	// c.cache is guaranteed to be non-nil

	ccname := depSpec.ChaincodeSpec.ChaincodeId.Name
	ccversion := depSpec.ChaincodeSpec.ChaincodeId.Version

	if ccname == "" {
		return nil, fmt.Errorf("the chaincode name cannot be an emoty string")
	}

	if ccversion == "" {
		return nil, fmt.Errorf("the chaincode version cannot be an emoty string")
	}

	key := ccname + "/" + ccversion

	c.RLock()
	_, in := c.cache[key]
	c.RUnlock()

	if in {
		return nil, fmt.Errorf("attempted to put chaincode data twice for %s/%s", ccname, ccversion)
	}

	ccpack, err := c.ccfs.PutChaincode(depSpec)
	if err != nil || ccpack == nil {
		return nil, fmt.Errorf("PutChaincodeIntoFS failed, error %s", err)
	}

	// we have a non-nil CCPackage, put it in the cache
	c.Lock()
	c.cache[key] = ccpack
	c.Unlock()

	return ccpack, nil
}
