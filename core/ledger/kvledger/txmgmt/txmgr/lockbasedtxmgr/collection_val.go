/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package lockbasedtxmgr

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
)

// collNameValidator validates the presence of a collection in a namespace
// This is expected to be instantiated in the context of a simulator/queryexecutor
type collNameValidator struct {
	ccInfoProvider ledger.DeployedChaincodeInfoProvider
	queryExecutor  *lockBasedQueryExecutor
	cache          collConfigCache
}

func newCollNameValidator(ccInfoProvider ledger.DeployedChaincodeInfoProvider, qe *lockBasedQueryExecutor) *collNameValidator {
	return &collNameValidator{ccInfoProvider, qe, make(collConfigCache)}
}

func (v *collNameValidator) validateCollName(ns, coll string) error {
	if !v.cache.isPopulatedFor(ns) {
		conf, err := v.retrieveCollConfigFromStateDB(ns)
		if err != nil {
			return err
		}
		v.cache.populate(ns, conf)
	}
	if !v.cache.containsCollName(ns, coll) {
		return &ledger.InvalidCollNameError{
			Ns:   ns,
			Coll: coll,
		}
	}
	return nil
}

func (v *collNameValidator) retrieveCollConfigFromStateDB(ns string) (*common.CollectionConfigPackage, error) {
	logger.Debugf("retrieveCollConfigFromStateDB() begin - ns=[%s]", ns)
	ccInfo, err := v.ccInfoProvider.ChaincodeInfo(ns, v.queryExecutor)
	if err != nil {
		return nil, err
	}
	if ccInfo == nil || ccInfo.CollectionConfigPkg == nil {
		return nil, &ledger.CollConfigNotDefinedError{Ns: ns}
	}
	confPkg := ccInfo.CollectionConfigPkg
	logger.Debugf("retrieveCollConfigFromStateDB() successfully retrieved - ns=[%s], confPkg=[%s]", ns, confPkg)
	return confPkg, nil
}

type collConfigCache map[collConfigkey]bool

type collConfigkey struct {
	ns, coll string
}

func (c collConfigCache) populate(ns string, pkg *common.CollectionConfigPackage) {
	// an entry with an empty collection name to indicate that the cache is populated for the namespace 'ns'
	// see function 'isPopulatedFor'
	c[collConfigkey{ns, ""}] = true
	for _, config := range pkg.Config {
		sConfig := config.GetStaticCollectionConfig()
		if sConfig == nil {
			continue
		}
		c[collConfigkey{ns, sConfig.Name}] = true
	}
}

func (c collConfigCache) isPopulatedFor(ns string) bool {
	return c[collConfigkey{ns, ""}]
}

func (c collConfigCache) containsCollName(ns, coll string) bool {
	return c[collConfigkey{ns, coll}]
}
