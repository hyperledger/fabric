/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cc_test

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/cclifecycle"
	"github.com/hyperledger/fabric/core/cclifecycle/mocks"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func init() {
	logging.SetLevel(logging.DEBUG, "discovery/lifecycle")
}

func TestNewQuery(t *testing.T) {
	// This tests that the QueryCreatorFunc can cast the below function to the interface type
	var q cc.Query
	queryCreator := func() (cc.Query, error) {
		q := &mocks.Query{}
		q.On("Done")
		return q, nil
	}
	q, _ = cc.QueryCreatorFunc(queryCreator).NewQuery()
	q.Done()
}

func TestHandleMetadataUpdate(t *testing.T) {
	f := func(channel string, chaincodes chaincode.MetadataSet) {
		assert.Len(t, chaincodes, 2)
		assert.Equal(t, "mychannel", channel)
	}
	cc.HandleMetadataUpdate(f).LifeCycleChangeListener("mychannel", chaincode.MetadataSet{{}, {}})
}

func TestEnumerate(t *testing.T) {
	f := func() ([]chaincode.InstalledChaincode, error) {
		return []chaincode.InstalledChaincode{{}, {}}, nil
	}
	ccs, err := cc.Enumerate(f).Enumerate()
	assert.NoError(t, err)
	assert.Len(t, ccs, 2)
}

func TestLifecycleInitFailure(t *testing.T) {
	listCCs := &mocks.Enumerator{}
	listCCs.On("Enumerate").Return(nil, errors.New("failed accessing DB"))
	lc, err := cc.NewLifeCycle(listCCs)
	assert.Nil(t, lc)
	assert.Contains(t, err.Error(), "failed accessing DB")
}

func TestHandleChaincodeDeployGreenPath(t *testing.T) {
	logger, restoreLogger := newLogAsserter(t)
	defer restoreLogger()

	cc1Bytes := utils.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc1",
		Version: "1.0",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	})

	cc2Bytes := utils.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc2",
		Version: "1.0",
		Id:      []byte{42},
	})

	cc3Bytes := utils.MarshalOrPanic(&ccprovider.ChaincodeData{
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
			Id:      []byte{42},
		},
		{
			// This chaincode has a different version installed than is instantiated
			Name:    "cc2",
			Version: "1.1",
			Id:      []byte{50},
		},
		{
			// This chaincode isn't instantiated on the channel (the Id is 50 but in the state its 42), but is installed
			Name:    "cc3",
			Version: "1.0",
			Id:      []byte{50},
		},
	}, nil)

	lc, err := cc.NewLifeCycle(enum)
	assert.NoError(t, err)

	lsnr := &mocks.LifeCycleChangeListener{}
	lsnr.On("LifeCycleChangeListener", mock.Anything, mock.Anything)
	lc.AddListener(lsnr)

	sub, err := lc.NewChannelSubscription("mychannel", queryCreator)
	assert.NoError(t, err)
	assert.NotNil(t, sub)

	// Ensure that the listener was updated
	logger.AssertLogged("Listeners for channel mychannel invoked")
	lsnr.AssertCalled(t, "LifeCycleChangeListener", "mychannel", chaincode.MetadataSet{chaincode.Metadata{
		Name:    "cc1",
		Version: "1.0",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	}})

	// Signal a deployment of a new chaincode and make sure the chaincode listener is updated with both chaincodes
	cc3Bytes = utils.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc3",
		Version: "1.0",
		Id:      []byte{50},
	})
	query.On("GetState", "lscc", "cc3").Return(cc3Bytes, nil).Once()
	sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{Name: "cc3", Version: "1.0", Hash: []byte{50}}, nil)
	sub.ChaincodeDeployDone(true)
	// Ensure that the listener is called with the new chaincode and the old chaincode metadata
	logger.AssertLogged("Listeners for channel mychannel invoked")
	assert.Len(t, lsnr.Calls, 2)
	sortedMetadata := sortedMetadataSet(lsnr.Calls[1].Arguments.Get(1).(chaincode.MetadataSet)).sort()
	assert.Equal(t, sortedMetadata, chaincode.MetadataSet{{
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
	cc3Bytes = utils.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc3",
		Version: "1.1",
		Id:      []byte{50},
	})
	query.On("GetState", "lscc", "cc3").Return(cc3Bytes, nil).Once()
	sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{Name: "cc3", Version: "1.1", Hash: []byte{50}}, nil)
	sub.ChaincodeDeployDone(true)
	// Ensure that the listener is called with the new chaincode and the old chaincode metadata
	logger.AssertLogged("Listeners for channel mychannel invoked")
	assert.Len(t, lsnr.Calls, 3)
	sortedMetadata = sortedMetadataSet(lsnr.Calls[2].Arguments.Get(1).(chaincode.MetadataSet)).sort()
	assert.Equal(t, sortedMetadata, chaincode.MetadataSet{{
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
	logger, restoreLogger := newLogAsserter(t)
	defer restoreLogger()

	cc1Bytes := utils.MarshalOrPanic(&ccprovider.ChaincodeData{
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
			Id:      []byte{42},
		},
	}, nil)

	lc, err := cc.NewLifeCycle(enum)
	assert.NoError(t, err)

	lsnr := &mocks.LifeCycleChangeListener{}
	lsnr.On("LifeCycleChangeListener", mock.Anything, mock.Anything)
	lc.AddListener(lsnr)

	// Scenario I: A channel subscription is made but obtaining a new query is not possible.
	queryCreator.On("NewQuery").Return(nil, errors.New("failed accessing DB")).Once()
	sub, err := lc.NewChannelSubscription("mychannel", queryCreator)
	assert.Nil(t, sub)
	assert.Contains(t, err.Error(), "failed accessing DB")
	lsnr.AssertNumberOfCalls(t, "LifeCycleChangeListener", 0)

	// Scenario II: A channel subscription is made and obtaining a new query succeeds, however - obtaining it once
	// a deployment notification occurs - fails.
	queryCreator.On("NewQuery").Return(query, nil).Once()
	queryCreator.On("NewQuery").Return(nil, errors.New("failed accessing DB")).Once()
	query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil).Once()
	sub, err = lc.NewChannelSubscription("mychannel", queryCreator)
	assert.NoError(t, err)
	assert.NotNil(t, sub)
	lsnr.AssertNumberOfCalls(t, "LifeCycleChangeListener", 1)
	sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{Name: "cc1", Version: "1.0", Hash: []byte{42}}, nil)
	sub.ChaincodeDeployDone(true)
	logger.AssertLogged("Failed creating a new query for channel mychannel: failed accessing DB")
	lsnr.AssertNumberOfCalls(t, "LifeCycleChangeListener", 1)

	// Scenario III: A channel subscription is made and obtaining a new query succeeds both at subscription initialization
	// and at deployment notification. However - GetState returns an error.
	// Note: Since we subscribe twice to the same channel, the information isn't loaded from the stateDB because it already had.
	queryCreator.On("NewQuery").Return(query, nil).Once()
	query.On("GetState", "lscc", "cc1").Return(nil, errors.New("failed accessing DB")).Once()
	sub, err = lc.NewChannelSubscription("mychannel", queryCreator)
	assert.NoError(t, err)
	assert.NotNil(t, sub)
	lsnr.AssertNumberOfCalls(t, "LifeCycleChangeListener", 2)
	sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{Name: "cc1", Version: "1.0", Hash: []byte{42}}, nil)
	sub.ChaincodeDeployDone(true)
	logger.AssertLogged("Query for channel mychannel for Name=cc1, Version=1.0, Hash=[]byte{0x2a} failed with error failed accessing DB")
	lsnr.AssertNumberOfCalls(t, "LifeCycleChangeListener", 2)

	// Scenario IV: A channel subscription is made successfully, and obtaining a new query succeeds at subscription initialization,
	// however - the deployment notification indicates the deploy failed.
	// Thus, the lifecycle change listener should not be called.
	sub, err = lc.NewChannelSubscription("mychannel", queryCreator)
	lsnr.AssertNumberOfCalls(t, "LifeCycleChangeListener", 3)
	assert.NoError(t, err)
	assert.NotNil(t, sub)
	sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{Name: "cc1", Version: "1.1", Hash: []byte{42}}, nil)
	sub.ChaincodeDeployDone(false)
	lsnr.AssertNumberOfCalls(t, "LifeCycleChangeListener", 3)
	logger.AssertLogged("Chaincode deploy for cc1 failed")
}

func TestMetadata(t *testing.T) {
	logger, restoreLogger := newLogAsserter(t)
	defer restoreLogger()

	cc1Bytes := utils.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc1",
		Version: "1.0",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	})

	cc2Bytes := utils.MarshalOrPanic(&ccprovider.ChaincodeData{
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
			Id:      []byte{42},
		},
	}, nil)

	lc, err := cc.NewLifeCycle(enum)
	assert.NoError(t, err)

	// Scenario I: No subscription was invoked on the lifecycle
	md := lc.Metadata("mychannel", "cc1", false)
	assert.Nil(t, md)
	logger.AssertLogged("Requested Metadata for non-existent channel mychannel")

	// Scenario II: A subscription was made on the lifecycle, and the metadata for the chaincode exists
	// because the chaincode is installed prior to the subscription, hence it was loaded during the subscription.
	query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil).Once()
	queryCreator.On("NewQuery").Return(query, nil).Once()
	sub, err := lc.NewChannelSubscription("mychannel", queryCreator)
	defer sub.ChaincodeDeployDone(true)
	assert.NoError(t, err)
	assert.NotNil(t, sub)
	md = lc.Metadata("mychannel", "cc1", false)
	assert.Equal(t, &chaincode.Metadata{
		Name:    "cc1",
		Version: "1.0",
		Id:      []byte{42},
		Policy:  []byte{1, 2, 3, 4, 5},
	}, md)
	logger.AssertLogged("Returning metadata for channel mychannel , chaincode cc1")

	// Scenario III: A metadata retrieval is made and the chaincode is not in memory yet,
	// and when the query is attempted to be made - it fails.
	queryCreator.On("NewQuery").Return(nil, errors.New("failed obtaining query executor")).Once()
	md = lc.Metadata("mychannel", "cc2", false)
	assert.Nil(t, md)
	logger.AssertLogged("Failed obtaining new query for channel mychannel : failed obtaining query executor")

	// Scenario IV:  A metadata retrieval is made and the chaincode is not in memory yet,
	// and when the query is attempted to be made - it succeeds, but GetState fails.
	queryCreator.On("NewQuery").Return(query, nil).Once()
	query.On("GetState", "lscc", "cc2").Return(nil, errors.New("GetState failed")).Once()
	md = lc.Metadata("mychannel", "cc2", false)
	assert.Nil(t, md)
	logger.AssertLogged("Failed querying LSCC for channel mychannel : GetState failed")

	// Scenario V: A metadata retrieval is made and the chaincode is not in memory yet,
	// and both the query and the GetState succeed, however - GetState returns nil
	queryCreator.On("NewQuery").Return(query, nil).Once()
	query.On("GetState", "lscc", "cc2").Return(nil, nil).Once()
	md = lc.Metadata("mychannel", "cc2", false)
	assert.Nil(t, md)
	logger.AssertLogged("Chaincode cc2 isn't defined in channel mychannel")

	// Scenario VI: A metadata retrieval is made and the chaincode is not in memory yet,
	// and both the query and the GetState succeed, however - GetState returns a valid metadata
	queryCreator.On("NewQuery").Return(query, nil).Once()
	query.On("GetState", "lscc", "cc2").Return(cc2Bytes, nil).Once()
	md = lc.Metadata("mychannel", "cc2", false)
	assert.Equal(t, &chaincode.Metadata{
		Name:    "cc2",
		Version: "1.0",
		Id:      []byte{42},
	}, md)

	// Scenario VII: A metadata retrieval is made and the chaincode is in the memory,
	// but a collection is also specified, thus - the retrieval should bypass the memory cache
	// and go straight into the stateDB.
	queryCreator.On("NewQuery").Return(query, nil).Once()
	query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil).Once()
	query.On("GetState", "lscc", privdata.BuildCollectionKVSKey("cc1")).Return([]byte{10, 10, 10}, nil).Once()
	md = lc.Metadata("mychannel", "cc1", true)
	assert.Equal(t, &chaincode.Metadata{
		Name:              "cc1",
		Version:           "1.0",
		Id:                []byte{42},
		Policy:            []byte{1, 2, 3, 4, 5},
		CollectionsConfig: []byte{10, 10, 10},
	}, md)
	logger.AssertLogged("Retrieved collection config for cc1 from cc1~collection")

	// Scenario VIII: A metadata retrieval is made and the chaincode is in the memory,
	// but a collection is also specified, thus - the retrieval should bypass the memory cache
	// and go straight into the stateDB. However - the retrieval fails
	queryCreator.On("NewQuery").Return(query, nil).Once()
	query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil).Once()
	query.On("GetState", "lscc", privdata.BuildCollectionKVSKey("cc1")).Return(nil, errors.New("foo")).Once()
	md = lc.Metadata("mychannel", "cc1", true)
	assert.Nil(t, md)
	logger.AssertLogged("Failed querying lscc namespace for cc1~collection: foo")
}

type logAsserter struct {
	logEntries chan string
	t          *testing.T
}

func newLogAsserter(t *testing.T) (*logAsserter, func()) {
	logAsserter := &logAsserter{
		t:          t,
		logEntries: make(chan string, 100),
	}

	cc.Logger.SetBackend(logAsserter)
	return logAsserter, func() {
		cc.Logger = flogging.MustGetLogger("discovery/lifecycle")
	}
}

func (l *logAsserter) AssertLogged(s string) {
	defer l.clearLogsQueue()
	for {
		select {
		case lastLogMsg := <-l.logEntries:
			l.t.Log(lastLogMsg)
			if strings.Contains(lastLogMsg, s) {
				return
			}
		case <-time.After(time.Second):
			l.t.Fatalf("Log entries didn't contain '%s'", s)
		}
	}
}

func (l *logAsserter) clearLogsQueue() {
	for len(l.logEntries) > 0 {
		<-l.logEntries
	}
}

func (l *logAsserter) Log(lvl logging.Level, n int, r *logging.Record) error {
	l.logEntries <- fmt.Sprint(r.Message())
	return nil
}

func (*logAsserter) GetLevel(string) logging.Level {
	return logging.DEBUG
}

func (*logAsserter) SetLevel(logging.Level, string) {
	panic("implement me")
}

func (*logAsserter) IsEnabledFor(logging.Level, string) bool {
	return true
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
