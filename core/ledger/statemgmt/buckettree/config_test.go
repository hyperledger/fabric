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
	"testing"

	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/spf13/viper"
)

func TestConfigInit(t *testing.T) {
	configs := viper.GetStringMap("ledger.state.dataStructure.configs")
	t.Logf("Configs loaded from yaml = %#v", configs)
	testDBWrapper.CleanDB(t)
	stateImpl := NewStateImpl()
	stateImpl.Initialize(configs)
	testutil.AssertEquals(t, conf.getNumBucketsAtLowestLevel(), configs[ConfigNumBuckets])
	testutil.AssertEquals(t, conf.getMaxGroupingAtEachLevel(), configs[ConfigMaxGroupingAtEachLevel])
}

func TestConfig(t *testing.T) {
	testConf := newConfig(26, 2, fnvHash)
	t.Logf("conf.levelToNumBucketsMap: [%#v]", testConf.levelToNumBucketsMap)
	testutil.AssertEquals(t, testConf.getLowestLevel(), 5)
	testutil.AssertEquals(t, testConf.getNumBuckets(0), 1)
	testutil.AssertEquals(t, testConf.getNumBuckets(1), 2)
	testutil.AssertEquals(t, testConf.getNumBuckets(2), 4)
	testutil.AssertEquals(t, testConf.getNumBuckets(3), 7)
	testutil.AssertEquals(t, testConf.getNumBuckets(4), 13)
	testutil.AssertEquals(t, testConf.getNumBuckets(5), 26)

	testutil.AssertEquals(t, testConf.computeParentBucketNumber(25), 13)
	testutil.AssertEquals(t, testConf.computeParentBucketNumber(9), 5)
	testutil.AssertEquals(t, testConf.computeParentBucketNumber(10), 5)

	testConf = newConfig(26, 3, fnvHash)
	t.Logf("conf.levelToNumBucketsMap: [%#v]", testConf.levelToNumBucketsMap)
	testutil.AssertEquals(t, testConf.getLowestLevel(), 3)
	testutil.AssertEquals(t, testConf.getNumBuckets(0), 1)
	testutil.AssertEquals(t, testConf.getNumBuckets(1), 3)
	testutil.AssertEquals(t, testConf.getNumBuckets(2), 9)
	testutil.AssertEquals(t, testConf.getNumBuckets(3), 26)

	testutil.AssertEquals(t, testConf.computeParentBucketNumber(24), 8)
	testutil.AssertEquals(t, testConf.computeParentBucketNumber(25), 9)
}
