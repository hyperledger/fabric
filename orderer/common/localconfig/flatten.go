/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package localconfig

import (
	"fmt"
	"reflect"
)

// Flatten performs a depth-first serialization of a struct to a slice of
// strings. Each string will be formatted at 'path.to.leaf = value'.
func Flatten(i interface{}) []string {
	var res []string
	flatten("", &res, reflect.ValueOf(i))
	return res
}

// flatten recursively retrieves every leaf node in a struct in depth-first fashion
// and aggregate the results into given string slice with format: "path.to.leaf = value"
// in the order of definition. Root name is ignored in the path. This helper function is
// useful to pretty-print a struct, such as configs.
// for example, given data structure:
//
//	A{
//	  B{
//	    C: "foo",
//	    D: 42,
//	  },
//	  E: nil,
//	}
//
// it should yield a slice of string containing following items:
// [
//
//	"B.C = \"foo\"",
//	"B.D = 42",
//	"E =",
//
// ]
func flatten(k string, m *[]string, v reflect.Value) {
	delimiter := "."
	if k == "" {
		delimiter = ""
	}

	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			*m = append(*m, fmt.Sprintf("%s =", k))
			return
		}
		flatten(k, m, v.Elem())
	case reflect.Struct:
		if x, ok := v.Interface().(fmt.Stringer); ok {
			*m = append(*m, fmt.Sprintf("%s = %v", k, x))
			return
		}

		for i := 0; i < v.NumField(); i++ {
			flatten(k+delimiter+v.Type().Field(i).Name, m, v.Field(i))
		}
	case reflect.String:
		// It is useful to quote string values
		*m = append(*m, fmt.Sprintf("%s = \"%s\"", k, v))
	default:
		*m = append(*m, fmt.Sprintf("%s = %v", k, v))
	}
}
