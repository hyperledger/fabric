package statecouchdb

import (
	"encoding/json"
	"fmt"
	"strings"
)

//ApplyQueryWrapper parses the query string passed to CouchDB
//the wrapper prepends the wrapper "data." to all fields specified in the query
//All fields in the selector must have "data." prepended to the field names
//Fields listed in fields key will have "data." prepended
//Fields in the sort key will have "data." prepended
func ApplyQueryWrapper(queryString string) []byte {

	//create a generic map for the query json
	jsonQuery := make(map[string]interface{})

	//unmarshal the selected json into the generic map
	json.Unmarshal([]byte(queryString), &jsonQuery)

	//iterate through the JSON query
	for jsonKey, jsonValue := range jsonQuery {

		//create a case for the data types found in the JSON query
		switch jsonQueryPart := jsonValue.(type) {

		//if the type is an array, then this is either the "fields" or "sort" part of the query
		case []interface{}:

			//check to see if this is a "fields" or "sort" array
			//if jsonKey == jsonQueryFields || jsonKey == jsonQuerySort {
			if jsonKey == jsonQueryFields {

				//iterate through the names and add the data wrapper for each field
				for itemKey, fieldName := range jsonQueryPart {

					//add "data" wrapper to each field definition
					jsonQueryPart[itemKey] = fmt.Sprintf("%v.%v", dataWrapper, fieldName)
				}

				//Add the "_id" and "version" fields,  these are needed by default
				if jsonKey == jsonQueryFields {

					jsonQueryPart = append(jsonQueryPart, "_id")
					jsonQueryPart = append(jsonQueryPart, "version")

					//Overwrite the query fields if the "_id" field has been added
					jsonQuery[jsonQueryFields] = jsonQueryPart
				}

			}

			if jsonKey == jsonQuerySort {

				//iterate through the names and add the data wrapper for each field
				for sortItemKey, sortField := range jsonQueryPart {

					//create a case for the data types found in the JSON query
					switch sortFieldType := sortField.(type) {

					//if the type is string, then this is a simple array of field names.
					//Add the datawrapper to the field name
					case string:

						//simple case, update the existing array item with the updated name
						jsonQueryPart[sortItemKey] = fmt.Sprintf("%v.%v", dataWrapper, sortField)

					case interface{}:

						//this case is a little more complicated.  Here we need to
						//iterate over the mapped field names since this is an array of objects
						//example:  {"fieldname":"desc"}
						for key, itemValue := range sortField.(map[string]interface{}) {
							//delete the mapping for the field definition, since we have to change the
							//value of the key
							delete(sortField.(map[string]interface{}), key)

							//add the key back into the map with the field name wrapped with then "data" wrapper
							sortField.(map[string]interface{})[fmt.Sprintf("%v.%v", dataWrapper, key)] = itemValue
						}

					default:

						logger.Debugf("The type %v was not recognized as a valid sort field type.", sortFieldType)

					}

				}
			}

		case interface{}:

			//if this is the "selector", the field names need to be mapped with the
			//data wrapper
			if jsonKey == jsonQuerySelector {

				processSelector(jsonQueryPart.(map[string]interface{}))

			}

		default:

			logger.Debugf("The value %v was not recognized as a valid selector field.", jsonKey)

		}
	}

	//Marshal the updated json query
	editedQuery, _ := json.Marshal(jsonQuery)

	logger.Debugf("Rewritten query with data wrapper: %s", editedQuery)

	return editedQuery

}

//processSelector is a recursion function for traversing the selector part of the query
func processSelector(selectorFragment map[string]interface{}) {

	//iterate through the top level definitions
	for itemKey, itemValue := range selectorFragment {

		//check to see if the itemKey starts with a $.  If so, this indicates an operator
		if strings.HasPrefix(fmt.Sprintf("%s", itemKey), "$") {

			processSelector(itemValue.(map[string]interface{}))

		} else {

			//delete the mapping for the field definition, since we have to change the
			//value of the key
			delete(selectorFragment, itemKey)
			//add the key back into the map with the field name wrapped with then "data" wrapper
			selectorFragment[fmt.Sprintf("%v.%v", dataWrapper, itemKey)] = itemValue

		}
	}
}
