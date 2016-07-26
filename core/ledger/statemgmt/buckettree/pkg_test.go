/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package buckettree

import (
	"fmt"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/tecbot/gorocksdb"
)

var testDBWrapper = db.NewTestDBWrapper()
var testParams []string

func TestMain(m *testing.M) {
	testParams = testutil.ParseTestParams()
	testutil.SetupTestConfig()
	os.Exit(m.Run())
}

// testHasher is a hash function for testing.
// It returns the hash for a key from pre-populated map
type testHasher struct {
	testHashFunctionInput map[string]uint32
}

func newTestHasher() *testHasher {
	return &testHasher{make(map[string]uint32)}
}

func (testHasher *testHasher) populate(chaincodeID string, key string, hash uint32) {
	testHasher.testHashFunctionInput[string(statemgmt.ConstructCompositeKey(chaincodeID, key))] = hash
}

func (testHasher *testHasher) getHashFunction() hashFunc {
	return func(data []byte) uint32 {
		key := string(data)
		value, ok := testHasher.testHashFunctionInput[key]
		if !ok {
			panic(fmt.Sprintf("A test should add entry before looking up. Entry looked up = [%s]", key))
		}
		return value
	}
}

type stateImplTestWrapper struct {
	configMap map[string]interface{}
	stateImpl *StateImpl
	t         testing.TB
}

func newStateImplTestWrapper(t testing.TB) *stateImplTestWrapper {
	var configMap map[string]interface{}
	stateImpl := NewStateImpl()
	err := stateImpl.Initialize(configMap)
	testutil.AssertNoError(t, err, "Error while constrcuting stateImpl")
	return &stateImplTestWrapper{configMap, stateImpl, t}
}

func newStateImplTestWrapperWithCustomConfig(t testing.TB, numBuckets int, maxGroupingAtEachLevel int) *stateImplTestWrapper {
	configMap := map[string]interface{}{ConfigNumBuckets: numBuckets, ConfigMaxGroupingAtEachLevel: maxGroupingAtEachLevel}
	stateImpl := NewStateImpl()
	err := stateImpl.Initialize(configMap)
	testutil.AssertNoError(t, err, "Error while constrcuting stateImpl")
	return &stateImplTestWrapper{configMap, stateImpl, t}
}

func createFreshDBAndInitTestStateImplWithCustomHasher(t testing.TB, numBuckets int, maxGroupingAtEachLevel int) (*testHasher, *stateImplTestWrapper, *statemgmt.StateDelta) {
	testHasher := newTestHasher()
	configMap := map[string]interface{}{
		ConfigNumBuckets:             numBuckets,
		ConfigMaxGroupingAtEachLevel: maxGroupingAtEachLevel,
		ConfigHashFunction:           testHasher.getHashFunction(),
	}

	testDBWrapper.CleanDB(t)
	stateImpl := NewStateImpl()
	stateImpl.Initialize(configMap)
	stateImplTestWrapper := &stateImplTestWrapper{configMap, stateImpl, t}
	stateDelta := statemgmt.NewStateDelta()
	return testHasher, stateImplTestWrapper, stateDelta
}

func (testWrapper *stateImplTestWrapper) constructNewStateImpl() {
	stateImpl := NewStateImpl()
	err := stateImpl.Initialize(testWrapper.configMap)
	testutil.AssertNoError(testWrapper.t, err, "Error while constructing new state tree")
	testWrapper.stateImpl = stateImpl
}

func (testWrapper *stateImplTestWrapper) get(chaincodeID string, key string) []byte {
	value, err := testWrapper.stateImpl.Get(chaincodeID, key)
	testutil.AssertNoError(testWrapper.t, err, "Error while getting value")
	testWrapper.t.Logf("state value for chaincodeID,key=[%s,%s] = [%s], ", chaincodeID, key, string(value))
	return value
}

func (testWrapper *stateImplTestWrapper) prepareWorkingSet(stateDelta *statemgmt.StateDelta) {
	err := testWrapper.stateImpl.PrepareWorkingSet(stateDelta)
	testutil.AssertNoError(testWrapper.t, err, "Error while PrepareWorkingSet")
}

func (testWrapper *stateImplTestWrapper) computeCryptoHash() []byte {
	cryptoHash, err := testWrapper.stateImpl.ComputeCryptoHash()
	testutil.AssertNoError(testWrapper.t, err, "Error while computing crypto hash")
	return cryptoHash
}

func (testWrapper *stateImplTestWrapper) prepareWorkingSetAndComputeCryptoHash(stateDelta *statemgmt.StateDelta) []byte {
	testWrapper.prepareWorkingSet(stateDelta)
	return testWrapper.computeCryptoHash()
}

func (testWrapper *stateImplTestWrapper) addChangesForPersistence(writeBatch *gorocksdb.WriteBatch) {
	err := testWrapper.stateImpl.AddChangesForPersistence(writeBatch)
	testutil.AssertNoError(testWrapper.t, err, "Error while adding changes to db write-batch")
}

func (testWrapper *stateImplTestWrapper) persistChangesAndResetInMemoryChanges() {
	writeBatch := gorocksdb.NewWriteBatch()
	defer writeBatch.Destroy()
	testWrapper.addChangesForPersistence(writeBatch)
	testDBWrapper.WriteToDB(testWrapper.t, writeBatch)
	testWrapper.stateImpl.ClearWorkingSet(true)
}

func (testWrapper *stateImplTestWrapper) getRangeScanIterator(chaincodeID string, startKey string, endKey string) statemgmt.RangeScanIterator {
	itr, err := testWrapper.stateImpl.GetRangeScanIterator(chaincodeID, startKey, endKey)
	testutil.AssertNoError(testWrapper.t, err, "Error while getting iterator")
	return itr
}

func expectedBucketHashForTest(data ...[]string) []byte {
	return testutil.ComputeCryptoHash(expectedBucketHashContentForTest(data...))
}

func expectedBucketHashContentForTest(data ...[]string) []byte {
	expectedContent := []byte{}
	for _, chaincodeData := range data {
		expectedContent = append(expectedContent, encodeNumberForTest(len(chaincodeData[0]))...)
		expectedContent = append(expectedContent, chaincodeData[0]...)
		expectedContent = append(expectedContent, encodeNumberForTest((len(chaincodeData)-1)/2)...)
		for i := 1; i < len(chaincodeData); i++ {
			expectedContent = append(expectedContent, encodeNumberForTest(len(chaincodeData[i]))...)
			expectedContent = append(expectedContent, chaincodeData[i]...)
		}
	}
	return expectedContent
}

func encodeNumberForTest(i int) []byte {
	return proto.EncodeVarint(uint64(i))
}

func TestExpectedBucketHashContentForTest(t *testing.T) {
	expectedHashContent1 := expectedBucketHashContentForTest(
		[]string{"chaincodeID1", "key1", "value1"},
		[]string{"chaincodeID_2", "key_1", "value_1", "key_2", "value_2"},
		[]string{"chaincodeID3", "key1", "value1", "key2", "value2", "key3", "value3"},
	)

	expectedHashContent2 := testutil.AppendAll(
		encodeNumberForTest(len("chaincodeID1")), []byte("chaincodeID1"),
		encodeNumberForTest(1),
		encodeNumberForTest(len("key1")), []byte("key1"), encodeNumberForTest(len("value1")), []byte("value1"),

		encodeNumberForTest(len("chaincodeID_2")), []byte("chaincodeID_2"),
		encodeNumberForTest(2),
		encodeNumberForTest(len("key_1")), []byte("key_1"), encodeNumberForTest(len("value_1")), []byte("value_1"),
		encodeNumberForTest(len("key_2")), []byte("key_2"), encodeNumberForTest(len("value_2")), []byte("value_2"),

		encodeNumberForTest(len("chaincodeID3")), []byte("chaincodeID3"),
		encodeNumberForTest(3),
		encodeNumberForTest(len("key1")), []byte("key1"), encodeNumberForTest(len("value1")), []byte("value1"),
		encodeNumberForTest(len("key2")), []byte("key2"), encodeNumberForTest(len("value2")), []byte("value2"),
		encodeNumberForTest(len("key3")), []byte("key3"), encodeNumberForTest(len("value3")), []byte("value3"),
	)
	testutil.AssertEquals(t, expectedHashContent1, expectedHashContent2)
}
