package statecouchdb

import (
	"encoding/json"
	"fmt"
	"reflect"
)

var dataWrapper = "data"
var jsonQueryFields = "fields"

var validOperators = []string{"$and", "$or", "$not", "$nor", "$all", "$elemMatch",
	"$lt", "$lte", "$eq", "$ne", "$gte", "$gt", "$exits", "$type", "$in", "$nin",
	"$size", "$mod", "$regex"}

/*
ApplyQueryWrapper parses the query string passed to CouchDB
the wrapper prepends the wrapper "data." to all fields specified in the query
All fields in the selector must have "data." prepended to the field names
Fields listed in fields key will have "data." prepended
Fields in the sort key will have "data." prepended

Example:

Source Query:
{"selector":{"owner": {"$eq": "tom"}},
"fields": ["owner", "asset_name", "color", "size"],
"sort": ["size", "color"], "limit": 10, "skip": 0}

Result Wrapped Query:
{"selector":{"data.owner":{"$eq":"tom"}},
"fields": ["data.owner","data.asset_name","data.color","data.size","_id","version"],
"sort":["data.size","data.color"],"limit":10,"skip":0}


*/
func ApplyQueryWrapper(namespace, queryString string) string {

	//TODO - namespace is being added to support scoping queries to the correct chaincode context
	// A followup change will add the implementation for enabling the namespace filter

	//create a generic map for the query json
	jsonQueryMap := make(map[string]interface{})

	//unmarshal the selected json into the generic map
	json.Unmarshal([]byte(queryString), &jsonQueryMap)

	//traverse through the json query and wrap any field names
	processAndWrapQuery(jsonQueryMap)

	//process the query and add the version and fields if fields are specified
	for jsonKey, jsonValue := range jsonQueryMap {

		//Add the "_id" and "version" fields,  these are needed by default
		if jsonKey == jsonQueryFields {

			//check to see if this is an interface map
			if reflect.TypeOf(jsonValue).String() == "[]interface {}" {

				//Add the "_id" and "version" fields,  these are needed by default
				//Overwrite the query fields if the "_id" field has been added
				jsonQueryMap[jsonQueryFields] = append(jsonValue.([]interface{}), "_id", "version")
			}

		}
	}

	//Marshal the updated json query
	editedQuery, _ := json.Marshal(jsonQueryMap)

	logger.Debugf("Rewritten query with data wrapper: %s", editedQuery)

	return string(editedQuery)

}

func processAndWrapQuery(jsonQueryMap map[string]interface{}) {

	//iterate through the JSON query
	for _, jsonValue := range jsonQueryMap {

		//create a case for the data types found in the JSON query
		switch jsonValueType := jsonValue.(type) {

		case string:
			//intercept the string case and prevent the []interface{} case from
			//incorrectly processing the string

		case float64:
			//intercept the float64 case and prevent the []interface{} case from
			//incorrectly processing the float64

		//if the type is an array, then iterate through the items
		case []interface{}:

			//iterate the the items in the array
			for itemKey, itemValue := range jsonValueType {

				switch itemValue.(type) {

				case string:

					//This is a simple string, so wrap the field and replace in the array
					jsonValueType[itemKey] = fmt.Sprintf("%v.%v", dataWrapper, itemValue)

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

//processInterfaceMap processes an interface map and wraps field names or traverses the next level of the json query
func processInterfaceMap(jsonFragment map[string]interface{}) {

	//iterate the the item in the map
	for keyVal, itemVal := range jsonFragment {

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
