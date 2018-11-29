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
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

// Support is an interface used to inject dependencies
type Support interface {
	// GetQueryExecutorForLedger returns a query executor for the specified channel
	GetQueryExecutorForLedger(cid string) (ledger.QueryExecutor, error)

	// GetIdentityDeserializer returns an IdentityDeserializer
	// instance for the specified chain
	GetIdentityDeserializer(chainID string) msp.IdentityDeserializer
}

// StateGetter retrieves data from the state
type State interface {
	// GetState retrieves the value for the given key in the given namespace
	GetState(namespace string, key string) ([]byte, error)
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

func (c *simpleCollectionStore) retrieveCollectionConfigPackage(cc common.CollectionCriteria, qe ledger.QueryExecutor) (*common.CollectionConfigPackage, error) {
	if qe != nil {
		return RetrieveCollectionConfigPackageFromState(cc, qe)
	}

	qe, err := c.s.GetQueryExecutorForLedger(cc.Channel)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not retrieve query executor for collection criteria %#v", cc))
	}
	defer qe.Done()
	return RetrieveCollectionConfigPackageFromState(cc, qe)
}

// RetrieveCollectionConfigPackageFromState retrieves the collection config package from the given key from the given state
func RetrieveCollectionConfigPackageFromState(cc common.CollectionCriteria, state State) (*common.CollectionConfigPackage, error) {
	cb, err := state.GetState("lscc", BuildCollectionKVSKey(cc.Namespace))
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("error while retrieving collection for collection criteria %#v", cc))
	}
	if cb == nil {
		return nil, NoSuchCollectionError(cc)
	}
	conf, err := ParseCollectionConfig(cb)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid configuration for collection criteria %#v", cc)
	}
	return conf, nil
}

// ParseCollectionConfig parses the collection configuration from the given serialized representation
func ParseCollectionConfig(colBytes []byte) (*common.CollectionConfigPackage, error) {
	collections := &common.CollectionConfigPackage{}
	err := proto.Unmarshal(colBytes, collections)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return collections, nil
}

func (c *simpleCollectionStore) retrieveCollectionConfig(cc common.CollectionCriteria, qe ledger.QueryExecutor) (*common.StaticCollectionConfig, error) {
	collections, err := c.retrieveCollectionConfigPackage(cc, qe)
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
				return cconf.StaticCollectionConfig, nil
			}
		default:
			return nil, errors.New("unexpected collection type")
		}
	}
	return nil, NoSuchCollectionError(cc)
}

func (c *simpleCollectionStore) retrieveSimpleCollection(cc common.CollectionCriteria, qe ledger.QueryExecutor) (*SimpleCollection, error) {
	staticCollectionConfig, err := c.retrieveCollectionConfig(cc, qe)
	if err != nil {
		return nil, err
	}
	sc := &SimpleCollection{}
	err = sc.Setup(staticCollectionConfig, c.s.GetIdentityDeserializer(cc.Channel))
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("error setting up collection for collection criteria %#v", cc))
	}
	return sc, nil
}

func (c *simpleCollectionStore) AccessFilter(channelName string, collectionPolicyConfig *common.CollectionPolicyConfig) (Filter, error) {
	sc := &SimpleCollection{}
	err := sc.setupAccessPolicy(collectionPolicyConfig, c.s.GetIdentityDeserializer(channelName))
	if err != nil {
		return nil, err
	}
	return sc.AccessFilter(), nil
}

func (c *simpleCollectionStore) RetrieveCollection(cc common.CollectionCriteria) (Collection, error) {
	return c.retrieveSimpleCollection(cc, nil)
}

func (c *simpleCollectionStore) RetrieveCollectionAccessPolicy(cc common.CollectionCriteria) (CollectionAccessPolicy, error) {
	return c.retrieveSimpleCollection(cc, nil)
}

func (c *simpleCollectionStore) RetrieveCollectionConfigPackage(cc common.CollectionCriteria) (*common.CollectionConfigPackage, error) {
	return c.retrieveCollectionConfigPackage(cc, nil)
}

// RetrieveCollectionPersistenceConfigs retrieves the collection's persistence related configurations
func (c *simpleCollectionStore) RetrieveCollectionPersistenceConfigs(cc common.CollectionCriteria) (CollectionPersistenceConfigs, error) {
	staticCollectionConfig, err := c.retrieveCollectionConfig(cc, nil)
	if err != nil {
		return nil, err
	}
	return &SimpleCollectionPersistenceConfigs{staticCollectionConfig.BlockToLive}, nil
}

func (c *simpleCollectionStore) HasReadAccess(cc common.CollectionCriteria, signedProposal *pb.SignedProposal, qe ledger.QueryExecutor) (bool, error) {
	accessPolicy, err := c.retrieveSimpleCollection(cc, qe)
	if err != nil {
		return false, err
	}

	if !accessPolicy.IsMemberOnlyRead() {
		return true, nil
	}

	signedData, err := getSignedData(signedProposal)
	if err != nil {
		return false, err
	}

	hasReadAccess := accessPolicy.AccessFilter()
	return hasReadAccess(signedData), nil
}

func getSignedData(signedProposal *pb.SignedProposal) (common.SignedData, error) {
	proposal, err := utils.GetProposal(signedProposal.ProposalBytes)
	if err != nil {
		return common.SignedData{}, err
	}

	creator, _, err := utils.GetChaincodeProposalContext(proposal)
	if err != nil {
		return common.SignedData{}, err
	}

	return common.SignedData{
		Data:      signedProposal.ProposalBytes,
		Identity:  creator,
		Signature: signedProposal.Signature,
	}, nil
}
