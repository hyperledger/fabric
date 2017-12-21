/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
)

// Support is an interface used to inject dependencies
type Support interface {
	// GetQueryExecutorForLedger returns a query executor for the specified channel
	GetQueryExecutorForLedger(cid string) (ledger.QueryExecutor, error)

	// GetCollectionKVSKey returns the name of the collection
	// given the collection criteria
	GetCollectionKVSKey(cc common.CollectionCriteria) string

	// GetIdentityDeserializer returns an IdentityDeserializer
	// instance for the specified chain
	GetIdentityDeserializer(chainID string) msp.IdentityDeserializer
}

type NoSuchCollectionError common.CollectionCriteria

func (f NoSuchCollectionError) Error() string {
	return fmt.Sprintf("collection %s/%s/%s could not be found", f.Channel, f.Namespace, f.Collection)
}

type simpleCollectionStore struct {
	s Support
}

// NewSimpleCollectionStore returns a collection stored backed
// by a ledger supplied by the specified ledgerGetter with
// an internal name formed as specified by the supplied
// collectionNamer function
func NewSimpleCollectionStore(s Support) CollectionStore {
	return &simpleCollectionStore{s}
}

func (c *simpleCollectionStore) retrieveCollectionConfigPackage(cc common.CollectionCriteria) (*common.CollectionConfigPackage, error) {
	qe, err := c.s.GetQueryExecutorForLedger(cc.Channel)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not retrieve query executor for collection criteria %#v", cc))
	}
	defer qe.Done()

	cb, err := qe.GetState("lscc", c.s.GetCollectionKVSKey(cc))
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("error while retrieving collection for collection criteria %#v", cc))
	}
	if cb == nil {
		return nil, NoSuchCollectionError(cc)
	}

	collections := &common.CollectionConfigPackage{}
	err = proto.Unmarshal(cb, collections)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid configuration for collection criteria %#v", cc)
	}

	return collections, nil
}

func (c *simpleCollectionStore) retrieveSimpleCollection(cc common.CollectionCriteria) (*SimpleCollection, error) {
	collections, err := c.retrieveCollectionConfigPackage(cc)
	if err != nil {
		return nil, err
	}
	if collections == nil {
		return nil, nil
	}

	for _, cconf := range collections.Config {
		switch cconf := cconf.Payload.(type) {
		case *common.CollectionConfig_StaticCollectionConfig:
			if cconf.StaticCollectionConfig.Name == cc.Collection {
				sc := &SimpleCollection{}

				err = sc.Setup(cconf.StaticCollectionConfig, c.s.GetIdentityDeserializer(cc.Channel))
				if err != nil {
					return nil, errors.WithMessage(err, fmt.Sprintf("error setting up collection for collection criteria %#v", cc))
				}

				return sc, nil
			}
		default:
			return nil, errors.New("unexpected collection type")
		}
	}

	return nil, NoSuchCollectionError(cc)
}

func (c *simpleCollectionStore) RetrieveCollection(cc common.CollectionCriteria) (Collection, error) {
	return c.retrieveSimpleCollection(cc)
}

func (c *simpleCollectionStore) RetrieveCollectionAccessPolicy(cc common.CollectionCriteria) (CollectionAccessPolicy, error) {
	return c.retrieveSimpleCollection(cc)
}

func (c *simpleCollectionStore) RetrieveCollectionConfigPackage(cc common.CollectionCriteria) (*common.CollectionConfigPackage, error) {
	return c.retrieveCollectionConfigPackage(cc)
}
