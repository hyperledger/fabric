/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cclifecycle_test

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	"github.com/hyperledger/fabric/core/cclifecycle"
	"github.com/hyperledger/fabric/core/cclifecycle/mocks"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewQuery(t *testing.T) {
	// This tests that the QueryCreatorFunc can cast the below function to the interface type
	var q cclifecycle.Query
	queryCreator := func() (cclifecycle.Query, error) {
		q := &mocks.Query{}
		q.On("Done")
		return q, nil
	}
	q, _ = cclifecycle.QueryCreatorFunc(queryCreator).NewQuery()
	q.Done()
}

func TestHandleMetadataUpdate(t *testing.T) {
	f := func(channel string, chaincodes chaincode.MetadataSet) {
		require.Len(t, chaincodes, 2)
		require.Equal(t, "mychannel", channel)
	}
	cclifecycle.HandleMetadataUpdateFunc(f).HandleMetadataUpdate("mychannel", chaincode.MetadataSet{{}, {}})
}

func TestEnumerate(t *testing.T) {
	f := func() ([]chaincode.InstalledChaincode, error) {
		return []chaincode.InstalledChaincode{{}, {}}, nil
	}
	ccs, err := cclifecycle.EnumerateFunc(f).Enumerate()
	require.NoError(t, err)
	require.Len(t, ccs, 2)
}

func TestLifecycleInitFailure(t *testing.T) {
	listCCs := &mocks.Enumerator{}
	listCCs.On("Enumerate").Return(nil, errors.New("failed accessing DB"))
	m, err := cclifecycle.NewMetadataManager(listCCs)
	require.Nil(t, m)
	require.Contains(t, err.Error(), "failed accessing DB")
}

func TestHandleChaincodeDeployGreenPath(t *testing.T) {
	recorder, restoreLogger := newLogRecorder(t)
	defer restoreLogger()

	cc1Bytes := protoutil.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc1",
		Version: "1.0",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	})

	cc2Bytes := protoutil.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc2",
		Version: "1.0",
		Id:      []byte{42},
	})

	cc3Bytes := protoutil.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc3",
		Version: "1.0",
		Id:      []byte{42},
	})

	query := &mocks.Query{}
	query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil)
	query.On("GetState", "lscc", "cc2").Return(cc2Bytes, nil)
	query.On("GetState", "lscc", "cc3").Return(cc3Bytes, nil).Once()
	query.On("Done")
	queryCreator := &mocks.QueryCreator{}
	queryCreator.On("NewQuery").Return(query, nil)

	enum := &mocks.Enumerator{}
	enum.On("Enumerate").Return([]chaincode.InstalledChaincode{
		{
			Name:    "cc1",
			Version: "1.0",
			Hash:    []byte{42},
		},
		{
			// This chaincode has a different version installed than is instantiated
			Name:    "cc2",
			Version: "1.1",
			Hash:    []byte{50},
		},
		{
			// This chaincode isn't instantiated on the channel (the Id is 50 but in the state its 42), but is installed
			Name:    "cc3",
			Version: "1.0",
			Hash:    []byte{50},
		},
	}, nil)

	m, err := cclifecycle.NewMetadataManager(enum)
	require.NoError(t, err)

	lsnr := &mocks.MetadataChangeListener{}
	lsnr.On("HandleMetadataUpdate", mock.Anything, mock.Anything)
	m.AddListener(lsnr)

	sub, err := m.NewChannelSubscription("mychannel", queryCreator)
	require.NoError(t, err)
	require.NotNil(t, sub)

	// Ensure that the listener was updated
	assertLogged(t, recorder, "Listeners for channel mychannel invoked")
	lsnr.AssertCalled(t, "HandleMetadataUpdate", "mychannel", chaincode.MetadataSet{chaincode.Metadata{
		Name:    "cc1",
		Version: "1.0",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	}})

	// Signal a deployment of a new chaincode and make sure the chaincode listener is updated with both chaincodes
	cc3Bytes = protoutil.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc3",
		Version: "1.0",
		Id:      []byte{50},
	})
	query.On("GetState", "lscc", "cc3").Return(cc3Bytes, nil).Once()
	sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{Name: "cc3", Version: "1.0", Hash: []byte{50}}, nil)
	sub.ChaincodeDeployDone(true)
	// Ensure that the listener is called with the new chaincode and the old chaincode metadata
	assertLogged(t, recorder, "Listeners for channel mychannel invoked")
	require.Len(t, lsnr.Calls, 2)
	sortedMetadata := sortedMetadataSet(lsnr.Calls[1].Arguments.Get(1).(chaincode.MetadataSet)).sort()
	require.Equal(t, sortedMetadata, chaincode.MetadataSet{{
		Name:    "cc1",
		Version: "1.0",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	}, {
		Name:    "cc3",
		Version: "1.0",
		Id:      []byte{50},
	}})

	// Next, update the chaincode metadata of the second chaincode to ensure that the listener is called with the updated
	// metadata and not with the old metadata.
	cc3Bytes = protoutil.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc3",
		Version: "1.1",
		Id:      []byte{50},
	})
	query.On("GetState", "lscc", "cc3").Return(cc3Bytes, nil).Once()
	sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{Name: "cc3", Version: "1.1", Hash: []byte{50}}, nil)
	sub.ChaincodeDeployDone(true)
	// Ensure that the listener is called with the new chaincode and the old chaincode metadata
	assertLogged(t, recorder, "Listeners for channel mychannel invoked")
	require.Len(t, lsnr.Calls, 3)
	sortedMetadata = sortedMetadataSet(lsnr.Calls[2].Arguments.Get(1).(chaincode.MetadataSet)).sort()
	require.Equal(t, sortedMetadata, chaincode.MetadataSet{{
		Name:    "cc1",
		Version: "1.0",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	}, {
		Name:    "cc3",
		Version: "1.1",
		Id:      []byte{50},
	}})
}

func TestHandleChaincodeDeployFailures(t *testing.T) {
	recorder, restoreLogger := newLogRecorder(t)
	defer restoreLogger()

	cc1Bytes := protoutil.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc1",
		Version: "1.0",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	})

	query := &mocks.Query{}
	query.On("Done")
	queryCreator := &mocks.QueryCreator{}
	enum := &mocks.Enumerator{}
	enum.On("Enumerate").Return([]chaincode.InstalledChaincode{
		{
			Name:    "cc1",
			Version: "1.0",
			Hash:    []byte{42},
		},
	}, nil)

	m, err := cclifecycle.NewMetadataManager(enum)
	require.NoError(t, err)

	lsnr := &mocks.MetadataChangeListener{}
	lsnr.On("HandleMetadataUpdate", mock.Anything, mock.Anything)
	m.AddListener(lsnr)

	// Scenario I: A channel subscription is made but obtaining a new query is not possible.
	queryCreator.On("NewQuery").Return(nil, errors.New("failed accessing DB")).Once()
	sub, err := m.NewChannelSubscription("mychannel", queryCreator)
	require.Nil(t, sub)
	require.Contains(t, err.Error(), "failed accessing DB")
	lsnr.AssertNumberOfCalls(t, "HandleMetadataUpdate", 0)

	// Scenario II: A channel subscription is made and obtaining a new query succeeds, however - obtaining it once
	// a deployment notification occurs - fails.
	queryCreator.On("NewQuery").Return(query, nil).Once()
	queryCreator.On("NewQuery").Return(nil, errors.New("failed accessing DB")).Once()
	query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil).Once()
	sub, err = m.NewChannelSubscription("mychannel", queryCreator)
	require.NoError(t, err)
	require.NotNil(t, sub)
	lsnr.AssertNumberOfCalls(t, "HandleMetadataUpdate", 1)
	sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{Name: "cc1", Version: "1.0", Hash: []byte{42}}, nil)
	sub.ChaincodeDeployDone(true)
	assertLogged(t, recorder, "Failed creating a new query for channel mychannel: failed accessing DB")
	lsnr.AssertNumberOfCalls(t, "HandleMetadataUpdate", 1)

	// Scenario III: A channel subscription is made and obtaining a new query succeeds both at subscription initialization
	// and at deployment notification. However - GetState returns an error.
	// Note: Since we subscribe twice to the same channel, the information isn't loaded from the stateDB because it already had.
	queryCreator.On("NewQuery").Return(query, nil).Once()
	query.On("GetState", "lscc", "cc1").Return(nil, errors.New("failed accessing DB")).Once()
	sub, err = m.NewChannelSubscription("mychannel", queryCreator)
	require.NoError(t, err)
	require.NotNil(t, sub)
	lsnr.AssertNumberOfCalls(t, "HandleMetadataUpdate", 2)
	sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{Name: "cc1", Version: "1.0", Hash: []byte{42}}, nil)
	sub.ChaincodeDeployDone(true)
	assertLogged(t, recorder, "Query for channel mychannel for Name=cc1, Version=1.0, Hash=2a failed with error failed accessing DB")
	lsnr.AssertNumberOfCalls(t, "HandleMetadataUpdate", 2)

	// Scenario IV: A channel subscription is made successfully, and obtaining a new query succeeds at subscription initialization,
	// however - the deployment notification indicates the deploy failed.
	// Thus, the lifecycle change listener should not be called.
	sub, err = m.NewChannelSubscription("mychannel", queryCreator)
	lsnr.AssertNumberOfCalls(t, "HandleMetadataUpdate", 3)
	require.NoError(t, err)
	require.NotNil(t, sub)
	sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{Name: "cc1", Version: "1.1", Hash: []byte{42}}, nil)
	sub.ChaincodeDeployDone(false)
	lsnr.AssertNumberOfCalls(t, "HandleMetadataUpdate", 3)
	assertLogged(t, recorder, "Chaincode deploy for updates [Name=cc1, Version=1.1, Hash=2a] failed")
}

func TestMultipleUpdates(t *testing.T) {
	recorder, restoreLogger := newLogRecorder(t)
	defer restoreLogger()

	cc1Bytes := protoutil.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc1",
		Version: "1.1",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	})
	cc2Bytes := protoutil.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc2",
		Version: "1.0",
		Id:      []byte{50},
		Policy:  []byte{1, 2, 3, 4, 5},
	})

	query := &mocks.Query{}
	query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil)
	query.On("GetState", "lscc", "cc2").Return(cc2Bytes, nil)
	query.On("Done")
	queryCreator := &mocks.QueryCreator{}
	queryCreator.On("NewQuery").Return(query, nil)

	enum := &mocks.Enumerator{}
	enum.On("Enumerate").Return([]chaincode.InstalledChaincode{
		{
			Name:    "cc1",
			Version: "1.1",
			Hash:    []byte{42},
		},
		{
			Name:    "cc2",
			Version: "1.0",
			Hash:    []byte{50},
		},
	}, nil)

	m, err := cclifecycle.NewMetadataManager(enum)
	require.NoError(t, err)

	var lsnrCalled sync.WaitGroup
	lsnrCalled.Add(3)
	lsnr := &mocks.MetadataChangeListener{}
	lsnr.On("HandleMetadataUpdate", mock.Anything, mock.Anything).Run(func(arguments mock.Arguments) {
		lsnrCalled.Done()
	})
	m.AddListener(lsnr)

	sub, err := m.NewChannelSubscription("mychannel", queryCreator)
	require.NoError(t, err)

	sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{Name: "cc1", Version: "1.1", Hash: []byte{42}}, nil)
	sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{Name: "cc2", Version: "1.0", Hash: []byte{50}}, nil)
	sub.ChaincodeDeployDone(true)

	cc1MD := chaincode.Metadata{
		Name:    "cc1",
		Version: "1.1",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	}
	cc2MD := chaincode.Metadata{
		Name:    "cc2",
		Version: "1.0",
		Id:      []byte{50},
		Policy:  []byte{1, 2, 3, 4, 5},
	}
	metadataSetWithBothChaincodes := chaincode.MetadataSet{cc1MD, cc2MD}

	lsnrCalled.Wait()
	// We need to sort the metadata passed to the call because map iteration is involved in building the
	// metadata set.
	expectedMetadata := sortedMetadataSet(lsnr.Calls[2].Arguments.Get(1).(chaincode.MetadataSet)).sort()
	require.Equal(t, metadataSetWithBothChaincodes, expectedMetadata)

	// Wait for all listeners to fire
	g := NewGomegaWithT(t)
	g.Eventually(func() []string {
		return recorder.EntriesMatching("Listeners for channel mychannel invoked")
	}, time.Second*10).Should(HaveLen(3))
}

func TestMetadata(t *testing.T) {
	recorder, restoreLogger := newLogRecorder(t)
	defer restoreLogger()

	cc1Bytes := protoutil.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc1",
		Version: "1.0",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	})

	cc2Bytes := protoutil.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc2",
		Version: "1.0",
		Id:      []byte{42},
	})

	query := &mocks.Query{}
	query.On("GetState", "lscc", "cc3").Return(cc1Bytes, nil)
	query.On("Done")
	queryCreator := &mocks.QueryCreator{}

	enum := &mocks.Enumerator{}
	enum.On("Enumerate").Return([]chaincode.InstalledChaincode{
		{
			Name:    "cc1",
			Version: "1.0",
			Hash:    []byte{42},
		},
	}, nil)

	m, err := cclifecycle.NewMetadataManager(enum)
	require.NoError(t, err)

	// Scenario I: No subscription was invoked on the lifecycle
	md := m.Metadata("mychannel", "cc1")
	require.Nil(t, md)
	assertLogged(t, recorder, "Requested Metadata for non-existent channel mychannel")

	// Scenario II: A subscription was made on the lifecycle, and the metadata for the chaincode exists
	// because the chaincode is installed prior to the subscription, hence it was loaded during the subscription.
	query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil).Once()
	queryCreator.On("NewQuery").Return(query, nil).Once()
	sub, err := m.NewChannelSubscription("mychannel", queryCreator)
	defer sub.ChaincodeDeployDone(true)
	require.NoError(t, err)
	require.NotNil(t, sub)
	md = m.Metadata("mychannel", "cc1")
	require.Equal(t, &chaincode.Metadata{
		Name:    "cc1",
		Version: "1.0",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	}, md)
	assertLogged(t, recorder, "Returning metadata for channel mychannel , chaincode cc1")

	// Scenario III: A metadata retrieval is made and the chaincode is not in memory yet,
	// and when the query is attempted to be made - it fails.
	queryCreator.On("NewQuery").Return(nil, errors.New("failed obtaining query executor")).Once()
	md = m.Metadata("mychannel", "cc2")
	require.Nil(t, md)
	assertLogged(t, recorder, "Failed obtaining new query for channel mychannel : failed obtaining query executor")

	// Scenario IV:  A metadata retrieval is made and the chaincode is not in memory yet,
	// and when the query is attempted to be made - it succeeds, but GetState fails.
	queryCreator.On("NewQuery").Return(query, nil).Once()
	query.On("GetState", "lscc", "cc2").Return(nil, errors.New("GetState failed")).Once()
	md = m.Metadata("mychannel", "cc2")
	require.Nil(t, md)
	assertLogged(t, recorder, "Failed querying LSCC for channel mychannel : GetState failed")

	// Scenario V: A metadata retrieval is made and the chaincode is not in memory yet,
	// and both the query and the GetState succeed, however - GetState returns nil
	queryCreator.On("NewQuery").Return(query, nil).Once()
	query.On("GetState", "lscc", "cc2").Return(nil, nil).Once()
	md = m.Metadata("mychannel", "cc2")
	require.Nil(t, md)
	assertLogged(t, recorder, "Chaincode cc2 isn't defined in channel mychannel")

	// Scenario VI: A metadata retrieval is made and the chaincode is not in memory yet,
	// and both the query and the GetState succeed, however - GetState returns a valid metadata
	queryCreator.On("NewQuery").Return(query, nil).Once()
	query.On("GetState", "lscc", "cc2").Return(cc2Bytes, nil).Once()
	md = m.Metadata("mychannel", "cc2")
	require.Equal(t, &chaincode.Metadata{
		Name:    "cc2",
		Version: "1.0",
		Id:      []byte{42},
	}, md)

	// Scenario VII: A metadata retrieval is made and the chaincode is in the memory,
	// but a collection is also specified, thus - the retrieval should bypass the memory cache
	// and go straight into the stateDB.
	queryCreator.On("NewQuery").Return(query, nil).Once()
	query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil).Once()
	query.On("GetState", "lscc", privdata.BuildCollectionKVSKey("cc1")).Return(protoutil.MarshalOrPanic(&peer.CollectionConfigPackage{}), nil).Once()
	md = m.Metadata("mychannel", "cc1", "col1")
	require.Equal(t, &chaincode.Metadata{
		Name:              "cc1",
		Version:           "1.0",
		Id:                []byte{42},
		Policy:            []byte{1, 2, 3, 4, 5},
		CollectionsConfig: &peer.CollectionConfigPackage{},
	}, md)
	assertLogged(t, recorder, "Retrieved collection config for cc1 from cc1~collection")

	// Scenario VIII: A metadata retrieval is made and the chaincode is in the memory,
	// but a collection is also specified, thus - the retrieval should bypass the memory cache
	// and go straight into the stateDB. However - the retrieval fails
	queryCreator.On("NewQuery").Return(query, nil).Once()
	query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil).Once()
	query.On("GetState", "lscc", privdata.BuildCollectionKVSKey("cc1")).Return(nil, errors.New("foo")).Once()
	md = m.Metadata("mychannel", "cc1", "col1")
	require.Nil(t, md)
	assertLogged(t, recorder, "Failed querying lscc namespace for cc1~collection: foo")
}

func newLogRecorder(t *testing.T) (*floggingtest.Recorder, func()) {
	oldLogger := cclifecycle.Logger

	logger, recorder := floggingtest.NewTestLogger(t)
	cclifecycle.Logger = logger

	return recorder, func() { cclifecycle.Logger = oldLogger }
}

func assertLogged(t *testing.T, r *floggingtest.Recorder, msg string) {
	gt := NewGomegaWithT(t)
	gt.Eventually(r).Should(gbytes.Say(regexp.QuoteMeta(msg)))
}

type sortedMetadataSet chaincode.MetadataSet

func (mds sortedMetadataSet) Len() int {
	return len(mds)
}

func (mds sortedMetadataSet) Less(i, j int) bool {
	eI := strings.Replace(mds[i].Name, "cc", "", -1)
	eJ := strings.Replace(mds[j].Name, "cc", "", -1)
	nI, _ := strconv.ParseInt(eI, 10, 32)
	nJ, _ := strconv.ParseInt(eJ, 10, 32)
	return nI < nJ
}

func (mds sortedMetadataSet) Swap(i, j int) {
	mds[i], mds[j] = mds[j], mds[i]
}

func (mds sortedMetadataSet) sort() chaincode.MetadataSet {
	sort.Sort(mds)
	return chaincode.MetadataSet(mds)
}
