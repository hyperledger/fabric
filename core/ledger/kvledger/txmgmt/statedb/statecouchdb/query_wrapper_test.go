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

// TestSimpleQuery tests a simple
func TestSimpleQuery(t *testing.T) {

	rawQuery := []byte(`{"selector":{"owner":{"$eq":"jerry"}},"limit": 10,"skip": 0}`)

	wrappedQuery := []byte(ApplyQueryWrapper("ns1", string(rawQuery)))

	//There should be one wrapped field
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.owner\""), 1)

	//$eg operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$eq\""), 1)

	//value should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"jerry\""), 1)

}

// TestSimpleQuery tests a simple
func TestQueryWithOperator(t *testing.T) {

	rawQuery := []byte(`{"selector":{"$or":[{"owner":{"$eq":"jerry"}},{"owner": {"$eq": "frank"}}]},"limit": 10,"skip": 0}`)

	wrappedQuery := []byte(ApplyQueryWrapper("ns1", string(rawQuery)))

	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.owner\""), 2)

	//$or operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$or\""), 1)

	//$eq operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$eq\""), 2)

	//value should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"jerry\""), 1)

}

// TestQueryWithImplicitOperatorAndExplicitOperator tests a simple
func TestQueryWithImplicitOperatorAndExplicitOperator(t *testing.T) {

	rawQuery := []byte(`{"selector":{"color":"green","$or":[{"owner":"fred"},{"owner":"mary"}]}}`)

	wrappedQuery := []byte(ApplyQueryWrapper("ns1", string(rawQuery)))

	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.owner\""), 2)

	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.color\""), 1)

	//$or operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$or\""), 1)

	//value should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"fred\""), 1)

	//value should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"mary\""), 1)

}

// TestQueryWithFields tests a simple
func TestQueryWithFields(t *testing.T) {

	rawQuery := []byte(`{"selector":{"owner": {"$eq": "tom"}},"fields": ["owner", "asset_name", "color", "size"], "limit": 10, "skip": 0}`)

	wrappedQuery := []byte(ApplyQueryWrapper("ns1", string(rawQuery)))

	//$eq operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$eq\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.owner\""), 2)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.asset_name\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.color\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.size\""), 1)

}

//TestQueryWithSortFields tests a simple
func TestQueryWithSortFields(t *testing.T) {

	rawQuery := []byte(`{"selector":{"owner": {"$eq": "tom"}},"fields": ["owner", "asset_name", "color", "size"], "sort": ["size", "color"], "limit": 10, "skip": 0}`)

	wrappedQuery := []byte(ApplyQueryWrapper("ns1", string(rawQuery)))

	//$eq operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$eq\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.owner\""), 2)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.asset_name\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.color\""), 2)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.size\""), 2)

}

//TestQueryWithSortObjects tests a simple
func TestQueryWithSortObjects(t *testing.T) {

	rawQuery := []byte(`{"selector":{"owner": {"$eq": "tom"}},"fields": ["owner", "asset_name", "color", "size"], "sort": [{"size": "desc"}, {"color": "desc"}], "limit": 10, "skip": 0}`)

	wrappedQuery := []byte(ApplyQueryWrapper("ns1", string(rawQuery)))

	//$eq operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$eq\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.owner\""), 2)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.asset_name\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.color\""), 2)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.size\""), 2)

	//sort descriptor should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"desc\""), 2)

}

//TestQueryLeadingOperator tests a simple
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

	wrappedQuery := []byte(ApplyQueryWrapper("ns1", string(rawQuery)))

	//$and operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$and\""), 1)

	//$gte operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$gte\""), 1)

	//$lte operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$lte\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.size\""), 2)

}

//TestQueryLeadingOperator tests a simple
func TestQueryLeadingAndEmbeddedOperator(t *testing.T) {

	rawQuery := []byte(`{"selector":{
					 "$and": [
							 { "size": {"$gte": 10}},
							 { "size": {"$lte": 20}},
							 { "$not": {"size": 15}}
					 ]
			 }}`)

	wrappedQuery := []byte(ApplyQueryWrapper("ns1", string(rawQuery)))

	//$and operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$and\""), 1)

	//$gte operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$gte\""), 1)

	//$lte operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$lte\""), 1)

	//$not operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$not\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.size\""), 3)

}

//TestQueryEmbeddedOperatorAndArrayOfObjects tests a simple
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

	wrappedQuery := []byte(ApplyQueryWrapper("ns1", string(rawQuery)))

	//$and operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$and\""), 1)

	//$gte operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$gte\""), 1)

	//$lte operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$lte\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.size\""), 5)

}

//TestQueryEmbeddedOperatorAndArrayOfValues tests a simple
func TestQueryEmbeddedOperatorAndArrayOfValues(t *testing.T) {

	rawQuery := []byte(`{
	  "selector": {
	    "size": {"$gt": 10},
	    "owner": {
	      "$all": ["bob","fred"]
	    }
	  }
	}`)

	wrappedQuery := []byte(ApplyQueryWrapper("ns1", string(rawQuery)))

	//$gt operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$gt\""), 1)

	//$all operator should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"$all\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.size\""), 1)

	//field value should be wrapped
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"data.owner\""), 1)

	//value should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"bob\""), 1)

	//value should be unchanged
	testutil.AssertEquals(t, strings.Count(string(wrappedQuery), "\"fred\""), 1)

}
