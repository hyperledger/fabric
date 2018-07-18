/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
)

// Test the invocation of a transaction for private data.
func TestQueriesPrivateData(t *testing.T) {
	// Skipping this tests as this test requires the application configuration to be set such that the private data capability is set to 'true'
	// However, with the latest restructuring of some of the packages, it is not possible to register system chaincodes with desired configurations for test.
	// see function RegisterSysCCs in file 'fabric/core/scc/register.go'. In absence of this lscc returns error while deploying a chaincode with collection configurations.
	// This test should be moved as an integration test outside of chaincode package.
	t.Skip()
	chainID := util.GetTestChainID()
	_, chaincodeSupport, cleanup, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer cleanup()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/map"
	cID := &pb.ChaincodeID{Name: "tmap", Path: url, Version: "0"}

	f := "init"
	args := util.ToChaincodeArgs(f)

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}

	cccid := &ccprovider.CCContext{
		Name:    "tmap",
		Version: "0",
	}

	var nextBlockNumber uint64 = 1
	// this test assumes four collections
	collectionConfig := []*common.StaticCollectionConfig{{Name: "c1"}, {Name: "c2"}, {Name: "c3"}, {Name: "c4"}}
	collectionConfigPkg := constructCollectionConfigPkg(collectionConfig)
	defer chaincodeSupport.Stop(&ccprovider.ChaincodeContainerInfo{
		Name:          cID.Name,
		Version:       cID.Version,
		Path:          cID.Path,
		Type:          "GOLANG",
		ContainerType: "DOCKER",
	})
	_, err = deployWithCollectionConfigs(chainID, cccid, spec, collectionConfigPkg, nextBlockNumber, chaincodeSupport)
	nextBlockNumber++
	ccID := spec.ChaincodeId.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", ccID, err)
		return
	}

	// Add 101 marbles for testing range queries and rich queries (for capable ledgers)
	// on both public and private data. The tests will test both range and rich queries
	// and queries with query limits
	for i := 1; i <= 101; i++ {
		f = "put"

		// 51 owned by tom, 50 by jerry
		owner := "tom"
		if i%2 == 0 {
			owner = "jerry"
		}

		// one marble color is red, 100 are blue
		color := "blue"
		if i == 12 {
			color = "red"
		}

		key := fmt.Sprintf("marble%03d", i)
		argsString := fmt.Sprintf("{\"docType\":\"marble\",\"name\":\"%s\",\"color\":\"%s\",\"size\":35,\"owner\":\"%s\"}", key, color, owner)
		args = util.ToChaincodeArgs(f, key, argsString)
		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, _, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++

		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

		f = "putPrivate"

		key = fmt.Sprintf("pmarble%03d", i)
		args = util.ToChaincodeArgs(f, "c1", key, argsString)
		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, _, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++

		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

	}

	// Insert a marble in 3 private collections
	for i := 2; i <= 4; i++ {
		collection := fmt.Sprintf("c%d", i)
		value := fmt.Sprintf("value_c%d", i)

		f = "putPrivate"
		t.Logf("invoking PutPrivateData with collection:<%s> key:%s", collection, "marble001")
		args = util.ToChaincodeArgs(f, collection, "pmarble001", value)
		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, _, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++

		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}
	}

	// read a marble from collection c3
	f = "getPrivate"
	args = util.ToChaincodeArgs(f, "c3", "pmarble001")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err := invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++

	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	var val string
	err = json.Unmarshal(retval, &val)
	expectedValue := fmt.Sprintf("value_c%d", 3)
	if val != expectedValue {
		t.Fail()
		t.Logf("Error detected with the GetPrivateData: expected '%s' but got '%s'", expectedValue, val)
		return
	}

	// delete a marble from collection c3
	f = "removePrivate"
	args = util.ToChaincodeArgs(f, "c3", "pmarble001")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++

	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	// delete a marble from collection c4
	f = "removePrivate"
	args = util.ToChaincodeArgs(f, "c4", "pmarble001")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++

	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	// read deleted marble from collection c3 to verify whether delete executed correctly
	f = "getPrivate"
	args = util.ToChaincodeArgs(f, "c3", "pmarble001")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++

	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	err = json.Unmarshal(retval, &val)
	if val != "" {
		t.Fail()
		t.Logf("Error detected with the GetPrivateData")
		return
	}

	// try to read the marble inserted in collection c2 from public state to check
	// whether it returns the marble (for correct operation, it should not return)
	f = "get"
	args = util.ToChaincodeArgs(f, "pmarble001")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++

	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	err = json.Unmarshal(retval, &val)
	if val != "" {
		t.Fail()
		t.Logf("Error detected with the GetState: %s", val)
		return
	}
	//The following range query for "marble001" to "marble011" should return 10 marbles
	f = "keysPrivate"
	args = util.ToChaincodeArgs(f, "c1", "pmarble001", "pmarble011")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}
	var keys []interface{}
	err = json.Unmarshal(retval, &keys)
	if len(keys) != 10 {
		t.Fail()
		t.Logf("Error detected with the range query, should have returned 10 but returned %v", len(keys))
		return
	}

	//The following range query for "marble001" to "marble011" should return 10 marbles
	f = "keys"
	args = util.ToChaincodeArgs(f, "marble001", "marble011")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	err = json.Unmarshal(retval, &keys)
	if len(keys) != 10 {
		t.Fail()
		t.Logf("Error detected with the range query, should have returned 10 but returned %v", len(keys))
		return
	}

	//FAB-1163- The following range query should timeout and produce an error
	//the peer should handle this gracefully and not die

	//save the original timeout and set a new timeout of 1 sec
	origTimeout := chaincodeSupport.ExecuteTimeout
	chaincodeSupport.ExecuteTimeout = time.Duration(1) * time.Second

	//chaincode to sleep for 2 secs with timeout 1
	args = util.ToChaincodeArgs(f, "marble001", "marble002", "2000")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	if err == nil {
		t.Fail()
		t.Logf("expected timeout error but succeeded")
		return
	}

	//restore timeout
	chaincodeSupport.ExecuteTimeout = origTimeout

	// querying for all marbles will return 101 marbles
	// this query should return exactly 101 results (one call to Next())
	//The following range query for "marble001" to "marble102" should return 101 marbles
	f = "keys"
	args = util.ToChaincodeArgs(f, "marble001", "marble102")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	//unmarshal the results
	err = json.Unmarshal(retval, &keys)

	//check to see if there are 101 values
	//default query limit of 10000 is used, this query is effectively unlimited
	if len(keys) != 101 {
		t.Fail()
		t.Logf("Error detected with the range query, should have returned 101 but returned %v", len(keys))
		return
	}

	// querying for all simple key. This query should return exactly 101 simple keys (one
	// call to Next()) no composite keys.
	//The following open ended range query for "" to "" should return 101 marbles
	f = "keys"
	args = util.ToChaincodeArgs(f, "", "")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	//unmarshal the results
	err = json.Unmarshal(retval, &keys)

	//check to see if there are 101 values
	//default query limit of 10000 is used, this query is effectively unlimited
	if len(keys) != 101 {
		t.Fail()
		t.Logf("Error detected with the range query, should have returned 101 but returned %v", len(keys))
		return
	}

	// ExecuteQuery supported only for CouchDB and
	// query limits apply for CouchDB range and rich queries only
	if ledgerconfig.IsCouchDBEnabled() == true {

		// corner cases for shim batching. currnt shim batch size is 100
		// this query should return exactly 100 results (no call to Next())
		f = "query"
		args = util.ToChaincodeArgs(f, "{\"selector\":{\"color\":\"blue\"}}")

		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, _, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++

		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

		//unmarshal the results
		err = json.Unmarshal(retval, &keys)

		//check to see if there are 100 values
		if len(keys) != 100 {
			t.Fail()
			t.Logf("Error detected with the rich query, should have returned 100 but returned %v %s", len(keys), keys)
			return
		}
		f = "queryPrivate"
		args = util.ToChaincodeArgs(f, "c1", "{\"selector\":{\"color\":\"blue\"}}")

		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, _, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++
		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

		//unmarshal the results
		err = json.Unmarshal(retval, &keys)

		//check to see if there are 100 values
		if len(keys) != 100 {
			t.Fail()
			t.Logf("Error detected with the rich query, should have returned 100 but returned %v %s", len(keys), keys)
			return
		}
		//Reset the query limit to 5
		viper.Set("ledger.state.queryLimit", 5)

		//The following range query for "marble01" to "marble11" should return 5 marbles due to the queryLimit
		f = "keys"
		args = util.ToChaincodeArgs(f, "marble001", "marble011")

		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, retval, err := invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++
		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

		//unmarshal the results
		err = json.Unmarshal(retval, &keys)
		//check to see if there are 5 values
		if len(keys) != 5 {
			t.Fail()
			t.Logf("Error detected with the range query, should have returned 5 but returned %v", len(keys))
			return
		}

		//Reset the query limit to 10000
		viper.Set("ledger.state.queryLimit", 10000)

		//The following rich query for should return 50 marbles
		f = "query"
		args = util.ToChaincodeArgs(f, "{\"selector\":{\"owner\":\"jerry\"}}")

		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++

		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

		//unmarshal the results
		err = json.Unmarshal(retval, &keys)

		//check to see if there are 50 values
		//default query limit of 10000 is used, this query is effectively unlimited
		if len(keys) != 50 {
			t.Fail()
			t.Logf("Error detected with the rich query, should have returned 50 but returned %v", len(keys))
			return
		}

		//Reset the query limit to 5
		viper.Set("ledger.state.queryLimit", 5)

		//The following rich query should return 5 marbles due to the queryLimit
		f = "query"
		args = util.ToChaincodeArgs(f, "{\"selector\":{\"owner\":\"jerry\"}}")

		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++
		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

		//unmarshal the results
		err = json.Unmarshal(retval, &keys)

		//check to see if there are 5 values
		if len(keys) != 5 {
			t.Fail()
			t.Logf("Error detected with the rich query, should have returned 5 but returned %v", len(keys))
			return
		}

	}

	// modifications for history query
	f = "put"
	args = util.ToChaincodeArgs(f, "marble012", "{\"docType\":\"marble\",\"name\":\"marble012\",\"color\":\"red\",\"size\":30,\"owner\":\"jerry\"}")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	f = "put"
	args = util.ToChaincodeArgs(f, "marble012", "{\"docType\":\"marble\",\"name\":\"marble012\",\"color\":\"red\",\"size\":30,\"owner\":\"jerry\"}")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	//The following history query for "marble12" should return 3 records
	f = "history"
	args = util.ToChaincodeArgs(f, "marble012")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	var history []interface{}
	err = json.Unmarshal(retval, &history)
	if len(history) != 3 {
		t.Fail()
		t.Logf("Error detected with the history query, should have returned 3 but returned %v", len(history))
		return
	}
}

func constructCollectionConfigPkg(staticCollectionConfigs []*common.StaticCollectionConfig) *common.CollectionConfigPackage {
	var cc []*common.CollectionConfig
	for _, sc := range staticCollectionConfigs {
		cc = append(cc, &common.CollectionConfig{
			Payload: &common.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: sc}})
	}
	return &common.CollectionConfigPackage{Config: cc}
}
