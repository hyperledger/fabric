/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// State retrieves data from the state.
type State interface {
	// GetState retrieves the value for the given key in the given namespace
	GetState(namespace string, key string) ([]byte, error)
}

type NoSuchCollectionError CollectionCriteria

func (f NoSuchCollectionError) Error() string {
	return fmt.Sprintf("collection %s/%s/%s could not be found", f.Channel, f.Namespace, f.Collection)
}

// A QueryExecutorFactory is responsible for creating ledger.QueryExectuor
// instances.
type QueryExecutorFactory interface {
	NewQueryExecutor() (ledger.QueryExecutor, error)
}

// ChaincodeInfoProvider provides information about deployed chaincode.
// LSCC module is expected to provide an implementation for this dependencys
type ChaincodeInfoProvider interface {
	// ChaincodeInfo returns the info about a deployed chaincode.
	ChaincodeInfo(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*ledger.DeployedChaincodeInfo, error)
	// CollectionInfo returns the proto msg that defines the named collection.
	// This function can be used for both explicit and implicit collections.
	CollectionInfo(channelName, chaincodeName, collectionName string, qe ledger.SimpleQueryExecutor) (*peer.StaticCollectionConfig, error)
	// AllCollectionsConfigPkg returns a combined collection config pkg that contains both explicit and implicit collections
	AllCollectionsConfigPkg(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*peer.CollectionConfigPackage, error)
}

// IdentityDeserializerFactory creates msp.IdentityDeserializer for
// a chain.
type IdentityDeserializerFactory interface {
	GetIdentityDeserializer(chainID string) msp.IdentityDeserializer
}

// IdentityDeserializerFactoryFunc is a function adapater for
// IdentityDeserializerFactory.
type IdentityDeserializerFactoryFunc func(chainID string) msp.IdentityDeserializer

func (i IdentityDeserializerFactoryFunc) GetIdentityDeserializer(chainID string) msp.IdentityDeserializer {
	return i(chainID)
}

// CollectionCriteria defines an element of a private data that corresponds
// to a certain transaction and collection
type CollectionCriteria struct {
	Channel    string
	Collection string
	Namespace  string
}

type SimpleCollectionStore struct {
	qeFactory             QueryExecutorFactory
	ccInfoProvider        ChaincodeInfoProvider
	idDeserializerFactory IdentityDeserializerFactory
}

func NewSimpleCollectionStore(
	qeFactory QueryExecutorFactory,
	ccInfoProvider ChaincodeInfoProvider,
	idDeserializerFactory IdentityDeserializerFactory,
) *SimpleCollectionStore {
	return &SimpleCollectionStore{
		qeFactory:             qeFactory,
		ccInfoProvider:        ccInfoProvider,
		idDeserializerFactory: idDeserializerFactory,
	}
}

func (c *SimpleCollectionStore) retrieveCollectionConfigPackage(cc CollectionCriteria, qe ledger.QueryExecutor) (*peer.CollectionConfigPackage, error) {
	var err error
	if qe == nil {
		qe, err = c.qeFactory.NewQueryExecutor()
		if err != nil {
			return nil, errors.WithMessagef(err, "could not retrieve query executor for collection criteria %#v", cc)
		}
		defer qe.Done()
	}
	return c.ccInfoProvider.AllCollectionsConfigPkg(cc.Channel, cc.Namespace, qe)
}

// RetrieveCollectionConfigPackageFromState retrieves the collection config package from the given key from the given state
func RetrieveCollectionConfigPackageFromState(cc CollectionCriteria, state State) (*peer.CollectionConfigPackage, error) {
	cb, err := state.GetState("lscc", BuildCollectionKVSKey(cc.Namespace))
	if err != nil {
		return nil, errors.WithMessagef(err, "error while retrieving collection for collection criteria %#v", cc)
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

// ParseCollectionConfig parses the collection configuration from the given serialized representation.
func ParseCollectionConfig(colBytes []byte) (*peer.CollectionConfigPackage, error) {
	collections := &peer.CollectionConfigPackage{}
	err := proto.Unmarshal(colBytes, collections)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return collections, nil
}

// RetrieveCollectionConfig retrieves a collection's config
func (c *SimpleCollectionStore) RetrieveCollectionConfig(cc CollectionCriteria) (*peer.StaticCollectionConfig, error) {
	return c.retrieveCollectionConfig(cc, nil)
}

func (c *SimpleCollectionStore) retrieveCollectionConfig(cc CollectionCriteria, qe ledger.QueryExecutor) (*peer.StaticCollectionConfig, error) {
	var err error
	if qe == nil {
		qe, err = c.qeFactory.NewQueryExecutor()
		if err != nil {
			return nil, errors.WithMessagef(err, "could not retrieve query executor for collection criteria %#v", cc)
		}
		defer qe.Done()
	}
	collConfig, err := c.ccInfoProvider.CollectionInfo(cc.Channel, cc.Namespace, cc.Collection, qe)
	if err != nil {
		return nil, err
	}
	if collConfig == nil {
		return nil, NoSuchCollectionError(cc)
	}
	return collConfig, nil
}

func (c *SimpleCollectionStore) retrieveSimpleCollection(cc CollectionCriteria, qe ledger.QueryExecutor) (*SimpleCollection, error) {
	staticCollectionConfig, err := c.retrieveCollectionConfig(cc, qe)
	if err != nil {
		return nil, err
	}
	sc := &SimpleCollection{}
	err = sc.Setup(staticCollectionConfig, c.idDeserializerFactory.GetIdentityDeserializer(cc.Channel))
	if err != nil {
		return nil, errors.WithMessagef(err, "error setting up collection for collection criteria %#v", cc)
	}
	return sc, nil
}

func (c *SimpleCollectionStore) AccessFilter(channelName string, collectionPolicyConfig *peer.CollectionPolicyConfig) (Filter, error) {
	sc := &SimpleCollection{}
	err := sc.setupAccessPolicy(collectionPolicyConfig, c.idDeserializerFactory.GetIdentityDeserializer(channelName))
	if err != nil {
		return nil, err
	}
	return sc.AccessFilter(), nil
}

func (c *SimpleCollectionStore) RetrieveCollection(cc CollectionCriteria) (Collection, error) {
	return c.retrieveSimpleCollection(cc, nil)
}

func (c *SimpleCollectionStore) RetrieveCollectionAccessPolicy(cc CollectionCriteria) (CollectionAccessPolicy, error) {
	return c.retrieveSimpleCollection(cc, nil)
}

func (c *SimpleCollectionStore) RetrieveCollectionConfigPackage(cc CollectionCriteria) (*peer.CollectionConfigPackage, error) {
	return c.retrieveCollectionConfigPackage(cc, nil)
}

// RetrieveCollectionPersistenceConfigs retrieves the collection's persistence related configurations
func (c *SimpleCollectionStore) RetrieveCollectionPersistenceConfigs(cc CollectionCriteria) (CollectionPersistenceConfigs, error) {
	staticCollectionConfig, err := c.retrieveCollectionConfig(cc, nil)
	if err != nil {
		return nil, err
	}
	return &SimpleCollectionPersistenceConfigs{staticCollectionConfig.BlockToLive}, nil
}

// RetrieveReadWritePermission retrieves the read-write permission of the creator of the
// signedProposal for a given collection using collection access policy and flags such as
// memberOnlyRead & memberOnlyWrite
func (c *SimpleCollectionStore) RetrieveReadWritePermission(
	cc CollectionCriteria,
	signedProposal *peer.SignedProposal,
	qe ledger.QueryExecutor,
) (bool, bool, error) {
	collection, err := c.retrieveSimpleCollection(cc, qe)
	if err != nil {
		return false, false, err
	}

	if canAnyoneReadAndWrite(collection) {
		return true, true, nil
	}

	// all members have read-write permission
	if isAMember, err := isCreatorOfProposalAMember(signedProposal, collection); err != nil {
		return false, false, err
	} else if isAMember {
		return true, true, nil
	}

	return !collection.IsMemberOnlyRead(), !collection.IsMemberOnlyWrite(), nil
}

func canAnyoneReadAndWrite(collection *SimpleCollection) bool {
	if !collection.IsMemberOnlyRead() && !collection.IsMemberOnlyWrite() {
		return true
	}
	return false
}

func isCreatorOfProposalAMember(signedProposal *peer.SignedProposal, collection *SimpleCollection) (bool, error) {
	signedData, err := getSignedData(signedProposal)
	if err != nil {
		return false, err
	}

	accessFilter := collection.AccessFilter()
	return accessFilter(signedData), nil
}

func getSignedData(signedProposal *peer.SignedProposal) (protoutil.SignedData, error) {
	proposal, err := protoutil.UnmarshalProposal(signedProposal.ProposalBytes)
	if err != nil {
		return protoutil.SignedData{}, err
	}

	hdr, err := protoutil.UnmarshalHeader(proposal.Header)
	if err != nil {
		return protoutil.SignedData{}, err
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return protoutil.SignedData{}, err
	}

	return protoutil.SignedData{
		Data:      signedProposal.ProposalBytes,
		Identity:  shdr.Creator,
		Signature: signedProposal.Signature,
	}, nil
}
