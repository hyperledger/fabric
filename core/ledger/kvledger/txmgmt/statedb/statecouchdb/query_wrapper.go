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
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

const dataWrapper = "data"
const jsonQueryFields = "fields"
const jsonQuerySelector = "selector"
const jsonQueryUseIndex = "use_index"
const jsonQueryLimit = "limit"
const jsonQuerySkip = "skip"

var validOperators = []string{"$and", "$or", "$not", "$nor", "$all", "$elemMatch",
	"$lt", "$lte", "$eq", "$ne", "$gte", "$gt", "$exits", "$type", "$in", "$nin",
	"$size", "$mod", "$regex"}

/*
ApplyQueryWrapper parses the query string passed to CouchDB
the wrapper prepends the wrapper "data." to all fields specified in the query
All fields in the selector must have "data." prepended to the field names
Fields listed in fields key will have "data." prepended
Fields in the sort key will have "data." prepended

- The query will be scoped to the chaincodeid

- limit be added to the query and is based on config
- skip is defaulted to 0 and is currently not used, this is for future paging implementation

In the example a contextID of "marble" is assumed.

Example:

Source Query:
{"selector":{"owner": {"$eq": "tom"}},
"fields": ["owner", "asset_name", "color", "size"],
"sort": ["size", "color"]}

Result Wrapped Query:
{"selector":{"$and":[{"chaincodeid":"marble"},{"data.owner":{"$eq":"tom"}}]},
"fields": ["data.owner","data.asset_name","data.color","data.size","_id","version"],
"sort":["data.size","data.color"],"limit":10,"skip":0}

*/
func ApplyQueryWrapper(namespace, queryString string, queryLimit, querySkip int) (string, error) {

	//create a generic map for the query json
	jsonQueryMap := make(map[string]interface{})

	//unmarshal the selected json into the generic map
	decoder := json.NewDecoder(bytes.NewBuffer([]byte(queryString)))
	decoder.UseNumber()
	err := decoder.Decode(&jsonQueryMap)
	if err != nil {
		return "", err
	}

	//traverse through the json query and wrap any field names
	processAndWrapQuery(jsonQueryMap)

	//if "fields" are specified in the query, then add the "_id", "version" and "chaincodeid" fields
	if jsonValue, ok := jsonQueryMap[jsonQueryFields]; ok {
		//check to see if this is an interface map
		if reflect.TypeOf(jsonValue).String() == "[]interface {}" {

			//Add the "_id", "version" and "chaincodeid" fields,  these are needed by default
			jsonQueryMap[jsonQueryFields] = append(jsonValue.([]interface{}),
				"_id", "version", "chaincodeid")
		}
	}

	//Check to see if the "selector" is specified in the query
	if jsonValue, ok := jsonQueryMap[jsonQuerySelector]; ok {
		//if the "selector" is found, then add the "$and" clause and the namespace filter
		setNamespaceInSelector(namespace, jsonValue, jsonQueryMap)
	} else {
		//if the "selector" is not found, then add a default namespace filter
		setDefaultNamespaceInSelector(namespace, jsonQueryMap)
	}

	//Add limit
	jsonQueryMap[jsonQueryLimit] = queryLimit

	//Add skip
	jsonQueryMap[jsonQuerySkip] = querySkip

	//Marshal the updated json query
	editedQuery, _ := json.Marshal(jsonQueryMap)

	logger.Debugf("Rewritten query with data wrapper: %s", editedQuery)

	return string(editedQuery), nil

}

//setNamespaceInSelector adds an additional hierarchy in the "selector"
//{"owner": {"$eq": "tom"}}
//would be mapped as (assuming a namespace of "marble"):
//{"$and":[{"chaincodeid":"marble"},{"data.owner":{"$eq":"tom"}}]}
func setNamespaceInSelector(namespace, jsonValue interface{},
	jsonQueryMap map[string]interface{}) {

	//create a array to store the parts of the query
	var queryParts = make([]interface{}, 0)

	//Add the namespace filter to filter on the chaincodeid
	namespaceFilter := make(map[string]interface{})
	namespaceFilter["chaincodeid"] = namespace

	//Add the context filter and the existing selector value
	queryParts = append(queryParts, namespaceFilter, jsonValue)

	//Create a new mapping for the new query structure
	mappedSelector := make(map[string]interface{})

	//Specify the "$and" operator for the parts of the query
	mappedSelector["$and"] = queryParts

	//Set the new mapped selector to the query selector
	jsonQueryMap[jsonQuerySelector] = mappedSelector

}

//setDefaultNamespaceInSelector adds an default namespace filter in "selector"
//If no selector is specified, the following is mapped to the "selector"
//assuming a namespace of "marble"
//{"chaincodeid":"marble"}
func setDefaultNamespaceInSelector(namespace string, jsonQueryMap map[string]interface{}) {

	//Add the context filter to filter on the chaincodeid
	namespaceFilter := make(map[string]interface{})
	namespaceFilter["chaincodeid"] = namespace

	//Set the new mapped selector to the query selector
	jsonQueryMap[jsonQuerySelector] = namespaceFilter
}

func processAndWrapQuery(jsonQueryMap map[string]interface{}) {

	//iterate through the JSON query
	for jsonKey, jsonValue := range jsonQueryMap {

		//create a case for the data types found in the JSON query
		switch jsonValueType := jsonValue.(type) {

		case string:
			//intercept the string case and prevent the []interface{} case from
			//incorrectly processing the string

		case float64:
			//intercept the float64 case and prevent the []interface{} case from
			//incorrectly processing the float64

		case json.Number:
			//intercept the Number case and prevent the []interface{} case from
			//incorrectly processing the float64

		//if the type is an array, then iterate through the items
		case []interface{}:

			//iterate the items in the array
			for itemKey, itemValue := range jsonValueType {

				switch itemValue.(type) {

				case string:

					//if this is not "use_index", the wrap the string
					if jsonKey != jsonQueryUseIndex {
						//This is a simple string, so wrap the field and replace in the array
						jsonValueType[itemKey] = fmt.Sprintf("%v.%v", dataWrapper, itemValue)
					}

				case []interface{}:

					//This is a array, so traverse to the next level
					processAndWrapQuery(itemValue.(map[string]interface{}))

				case interface{}:

					//process this part as a map
					processInterfaceMap(itemValue.(map[string]interface{}))

				}
			}

		case interface{}:

			//process this part as a map
			processInterfaceMap(jsonValue.(map[string]interface{}))

		}
	}
}

//processInterfaceMap processes an interface map and wraps field names or traverses
//the next level of the json query
func processInterfaceMap(jsonFragment map[string]interface{}) {

	//create a copy of the jsonFragment for iterating
	var bufferFragment = make(map[string]interface{})
	for keyVal, itemVal := range jsonFragment {
		bufferFragment[keyVal] = itemVal
	}

	//iterate the item in the map
	for keyVal, itemVal := range bufferFragment {

		//check to see if the key is an operator
		if arrayContains(validOperators, keyVal) {

			//if this is an operator, traverse the next level of the json query
			processAndWrapQuery(jsonFragment)

		} else {

			//if this is not an operator, this is a field name and needs to be wrapped
			wrapFieldName(jsonFragment, keyVal, itemVal)

		}
	}
}

//wrapFieldName "wraps" the field name with the data wrapper, and replaces the key in the json fragment
func wrapFieldName(jsonFragment map[string]interface{}, key string, value interface{}) {

	//delete the mapping for the field definition, since we have to change the
	//value of the key
	delete(jsonFragment, key)

	//add the key back into the map with the field name wrapped with then "data" wrapper
	jsonFragment[fmt.Sprintf("%v.%v", dataWrapper, key)] = value

}

//arrayContains is a function to detect if a soure array of strings contains the selected string
//for this application, it is used to determine if a string is a valid CouchDB operator
func arrayContains(sourceArray []string, selectItem string) bool {
	set := make(map[string]struct{}, len(sourceArray))
	for _, s := range sourceArray {
		set[s] = struct{}{}
	}
	_, ok := set[selectItem]
	return ok
}
