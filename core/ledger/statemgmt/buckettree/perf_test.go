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
	"flag"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/op/go-logging"
)

func BenchmarkStateHash(b *testing.B) {
	b.StopTimer()
	b.Logf("testParams:%q", testParams)
	flags := flag.NewFlagSet("testParams", flag.ExitOnError)
	numBuckets := flags.Int("NumBuckets", 10009, "Number of buckets")
	maxGroupingAtEachLevel := flags.Int("MaxGroupingAtEachLevel", 10, "max grouping at each level")
	chaincodeIDPrefix := flags.String("ChaincodeIDPrefix", "cID", "The chaincodeID used in the generated workload will be  ChaincodeIDPrefix_1, ChaincodeIDPrefix_2, and so on")
	numChaincodes := flags.Int("NumChaincodes", 1, "Number of chaincodes to assume")
	maxKeySuffix := flags.Int("MaxKeySuffix", 1, "the keys are appended with _1, _2,.. upto MaxKeySuffix")
	numKeysToInsert := flags.Int("NumKeysToInsert", 1, "how many keys to insert in a single batch")
	kvSize := flags.Int("KVSize", 1000, "size of the value")
	debugMsgsOn := flags.Bool("DebugOn", false, "Trun on/off debug messages during benchmarking")
	flags.Parse(testParams)

	b.Logf(`Running test with params:
		numbBuckets=%d, maxGroupingAtEachLevel=%d, chaincodeIDPrefix=%s, numChaincodes=%d, maxKeySuffix=%d, numKeysToInsert=%d, valueSize=%d, debugMsgs=%t`,
		*numBuckets, *maxGroupingAtEachLevel, *chaincodeIDPrefix, *numChaincodes, *maxKeySuffix, *numKeysToInsert, *kvSize, *debugMsgsOn)

	if !*debugMsgsOn {
		testutil.SetLogLevel(logging.ERROR, "statemgmt")
		testutil.SetLogLevel(logging.ERROR, "buckettree")
		testutil.SetLogLevel(logging.ERROR, "db")
	}

	stateImplTestWrapper := newStateImplTestWrapperWithCustomConfig(b, *numBuckets, *maxGroupingAtEachLevel)
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		delta := statemgmt.ConstructRandomStateDelta(b, *chaincodeIDPrefix, *numChaincodes, *maxKeySuffix, *numKeysToInsert, *kvSize)
		b.StartTimer()
		stateImplTestWrapper.prepareWorkingSet(delta)
		stateImplTestWrapper.computeCryptoHash()
		if i == b.N-1 {
			stateImplTestWrapper.persistChangesAndResetInMemoryChanges()
			testDBWrapper.CloseDB(b)
		}
	}
}
