/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txmgr

import (
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
)

// collNameValidator validates the presence of a collection in a namespace
// This is expected to be instantiated in the context of a simulator/queryexecutor
type collNameValidator struct {
	ledgerID       string
	ccInfoProvider ledger.DeployedChaincodeInfoProvider
	queryExecutor  *queryExecutor
	cache          collConfigCache
	noop           bool
}

func newCollNameValidator(ledgerID string, ccInfoProvider ledger.DeployedChaincodeInfoProvider, qe *queryExecutor, noop bool) *collNameValidator {
	return &collNameValidator{ledgerID, ccInfoProvider, qe, make(collConfigCache), noop}
}

func (v *collNameValidator) validateCollName(ns, coll string) error {
	if v.noop {
		return nil
	}
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

func (v *collNameValidator) retrieveCollConfigFromStateDB(ns string) (*peer.CollectionConfigPackage, error) {
	logger.Debugf("retrieveCollConfigFromStateDB() begin - ns=[%s]", ns)
	confPkg, err := v.ccInfoProvider.AllCollectionsConfigPkg(v.ledgerID, ns, v.queryExecutor)
	if err != nil {
		return nil, err
	}
	if confPkg == nil {
		return nil, &ledger.CollConfigNotDefinedError{Ns: ns}
	}
	logger.Debugf("retrieveCollConfigFromStateDB() successfully retrieved - ns=[%s], confPkg=[%s]", ns, confPkg)
	return confPkg, nil
}

type collConfigCache map[collConfigkey]bool

type collConfigkey struct {
	ns, coll string
}

func (c collConfigCache) populate(ns string, pkg *peer.CollectionConfigPackage) {
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
