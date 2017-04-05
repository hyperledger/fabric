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

package statecouchdb

import (
	"strings"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
)

// TestSimpleQuery tests a simple query
func TestSimpleQuery(t *testing.T) {

	rawQuery := []byte(`{"selector":{"owner":{"$eq":"jerry"}},"limit": 10,"skip": 0}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	//There should be one wrapped field
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.owner\""), 1)

	//$eg operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$eq\""), 1)

	//value should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"jerry\""), 1)

}

// TestSimpleQuery tests a query with a leading operator
func TestQueryWithOperator(t *testing.T) {

	rawQuery := []byte(`{"selector":{"$or":[{"owner":{"$eq":"jerry"}},{"owner": {"$eq": "frank"}}]},"limit": 10,"skip": 0}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.owner\""), 2)

	//$or operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$or\""), 1)

	//$eq operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$eq\""), 2)

	//value should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"jerry\""), 1)

}

// TestQueryWithImplicitOperatorAndExplicitOperator tests an implicit operator and an explicit operator
func TestQueryWithImplicitOperatorAndExplicitOperator(t *testing.T) {

	rawQuery := []byte(`{"selector":{"color":"green","$or":[{"owner":"fred"},{"owner":"mary"}]}}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.owner\""), 2)

	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.color\""), 1)

	//$or operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$or\""), 1)

	//value should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"fred\""), 1)

	//value should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"mary\""), 1)

}

// TestQueryWithFields tests a query with specified fields
func TestQueryWithFields(t *testing.T) {

	rawQuery := []byte(`{"selector":{"owner": {"$eq": "tom"}},"fields": ["owner", "asset_name", "color", "size"], "limit": 10, "skip": 0}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	//$eq operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$eq\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.owner\""), 2)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.asset_name\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.color\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.size\""), 1)

}

//TestQueryWithSortFields tests sorting fields
func TestQueryWithSortFields(t *testing.T) {

	rawQuery := []byte(`{"selector":{"owner": {"$eq": "tom"}},"fields": ["owner", "asset_name", "color", "size"], "sort": ["size", "color"], "limit": 10, "skip": 0}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	//$eq operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$eq\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.owner\""), 2)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.asset_name\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.color\""), 2)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.size\""), 2)

}

//TestQueryWithSortObjects tests a sort objects
func TestQueryWithSortObjects(t *testing.T) {

	rawQuery := []byte(`{"selector":{"owner": {"$eq": "tom"}},"fields": ["owner", "asset_name", "color", "size"], "sort": [{"size": "desc"}, {"color": "desc"}], "limit": 10, "skip": 0}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	//$eq operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$eq\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.owner\""), 2)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.asset_name\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.color\""), 2)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.size\""), 2)

	//sort descriptor should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"desc\""), 2)

}

//TestQueryLeadingOperator tests a leading operator
func TestQueryLeadingOperator(t *testing.T) {

	rawQuery := []byte(`{"selector":
 {"$and":[
		 {"size": {
				 "$gte": 2
		 }},
		 {"size": {
				 "$lte": 5
		 }}
 ]
 }
 }`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	//$and operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$and\""), 2)

	//$gte operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$gte\""), 1)

	//$lte operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$lte\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.size\""), 2)

}

//TestQueryLeadingOperator tests a leading operator and embedded operator
func TestQueryLeadingAndEmbeddedOperator(t *testing.T) {

	rawQuery := []byte(`{"selector":{
					 "$and": [
							 { "size": {"$gte": 10}},
							 { "size": {"$lte": 20}},
							 { "$not": {"size": 15}}
					 ]
			 }}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	//$and operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$and\""), 2)

	//$gte operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$gte\""), 1)

	//$lte operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$lte\""), 1)

	//$not operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$not\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.size\""), 3)

}

//TestQueryEmbeddedOperatorAndArrayOfObjects an embedded operator and object array
func TestQueryEmbeddedOperatorAndArrayOfObjects(t *testing.T) {

	rawQuery := []byte(`{
	  "selector": {
	    "$and": [
	      {
	        "size": {"$gte": 10}
	      },
	      {
	        "size": {"$lte": 30}
	      },
	      {
	        "$nor": [
	          {"size": 15},
	          {"size": 18},
	          {"size": 22}
	        ]
	      }
	    ]
	  }
	}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	//$and operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$and\""), 2)

	//$gte operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$gte\""), 1)

	//$lte operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$lte\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.size\""), 5)

}

//TestQueryEmbeddedOperatorAndArrayOfValues tests an array of values
func TestQueryEmbeddedOperatorAndArrayOfValues(t *testing.T) {

	rawQuery := []byte(`{
	  "selector": {
	    "size": {"$gt": 10},
	    "owner": {
	      "$all": ["bob","fred"]
	    }
	  }
	}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	//$gt operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$gt\""), 1)

	//$all operator should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"$all\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.size\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"data.owner\""), 1)

	//value should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"bob\""), 1)

	//value should be unchanged
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"fred\""), 1)

}

//TestQueryNoSelector with no selector specified
func TestQueryNoSelector(t *testing.T) {

	rawQuery := []byte(`{"fields": ["owner", "asset_name", "color", "size"]}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	//check to make sure the default selector is added
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"selector\":{\"chaincodeid\":\"ns1\"}"), 1)

}

//TestQueryWithUseDesignDoc tests query with index design doc specified
func TestQueryWithUseDesignDoc(t *testing.T) {

	rawQuery := []byte(`{"selector":{"owner":{"$eq":"jerry"}},"use_index":"_design/testDoc","limit": 10,"skip": 0}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	//check to make sure the default selector is added
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"use_index\":\"_design/testDoc\""), 1)

}

//TestQueryWithUseDesignDocAndIndexName tests query with index design doc and index name specified
func TestQueryWithUseDesignDocAndIndexName(t *testing.T) {

	rawQuery := []byte(`{"selector":{"owner":{"$eq":"jerry"}},"use_index":["_design/testDoc","testIndexName"],"limit": 10,"skip": 0}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	//check to make sure the default selector is added
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "\"use_index\":[\"_design/testDoc\",\"testIndexName\"]"), 1)

}

//TestQueryWithLargeInteger tests query with large integer
func TestQueryWithLargeInteger(t *testing.T) {

	rawQuery := []byte(`{"selector":{"$and":[{"size":{"$eq": 1000007}}]}}`)

	wrappedQuery, err := ApplyQueryWrapper("ns1", string(rawQuery), 10000, 0)

	//Make sure the query did not throw an exception
	testutil.AssertNoError(t, err, "Unexpected error thrown when for query JSON")

	//check to make sure the default selector is added
	testutil.AssertEquals(t, strings.Count(wrappedQuery, "{\"$eq\":1000007}"), 1)

}
