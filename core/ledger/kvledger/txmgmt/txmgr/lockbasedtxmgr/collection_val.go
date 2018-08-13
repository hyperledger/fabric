/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package lockbasedtxmgr

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/protos/common"
)

const (
	lsccNamespace = "lscc"
)

// collNameValidator validates the presence of a collection in a namespace
// This is expected to be instantiated in the context of a simulator/queryexecutor
type collNameValidator struct {
	queryHelper *queryHelper
	cache       collConfigCache
}

func newCollNameValidator(queryHelper *queryHelper) *collNameValidator {
	return &collNameValidator{queryHelper, make(collConfigCache)}
}

func (v *collNameValidator) validateCollName(ns, coll string) error {
	logger.Debugf("validateCollName() begin - ns=[%s], coll=[%s]", ns, coll)
	if !v.cache.isPopulatedFor(ns) {
		conf, err := v.retrieveCollConfigFromStateDB(ns)
		if err != nil {
			return err
		}
		v.cache.populate(ns, conf)
	}
	if !v.cache.containsCollName(ns, coll) {
		return &errInvalidCollName{ns, coll}
	}
	logger.Debugf("validateCollName() validated successfully - ns=[%s], coll=[%s]", ns, coll)
	return nil
}

func (v *collNameValidator) retrieveCollConfigFromStateDB(ns string) (*common.CollectionConfigPackage, error) {
	logger.Debugf("retrieveCollConfigFromStateDB() begin - ns=[%s]", ns)
	configPkgBytes, err := v.queryHelper.getState(lsccNamespace, constructCollectionConfigKey(ns))
	if err != nil {
		return nil, err
	}
	if configPkgBytes == nil {
		return nil, &errCollConfigNotDefined{ns}
	}
	confPkg := &common.CollectionConfigPackage{}
	if err := proto.Unmarshal(configPkgBytes, confPkg); err != nil {
		return nil, err
	}
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

func constructCollectionConfigKey(chaincodeName string) string {
	return privdata.BuildCollectionKVSKey(chaincodeName)
}

type errInvalidCollName struct {
	ns, coll string
}

func (e *errInvalidCollName) Error() string {
	return fmt.Sprintf("collection [%s] not defined in the collection config for chaincode [%s]", e.coll, e.ns)
}

type errCollConfigNotDefined struct {
	ns string
}

func (e *errCollConfigNotDefined) Error() string {
	return fmt.Sprintf("collection config not defined for chaincode [%s], pass the collection configuration upon chaincode definition/instantiation", e.ns)
}
