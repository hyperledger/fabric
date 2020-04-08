/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rangequery

import (
	"fmt"

	"github.com/pkg/errors"
)

// LimitOpt holds the name of the result limit key
const LimitOpt = "limit"

// ValidateOptions validates the JSON containing options for the range query
func ValidateOptions(options map[string]interface{}) error {
	for key, keyVal := range options {
		switch key {

		case LimitOpt:
			if _, ok := keyVal.(int32); ok {
				continue
			}
			return errors.New("invalid entry, \"limit\" must be a int32")

		default:
			return errors.New(fmt.Sprintf("invalid entry, option %s not recognized", key))
		}
	}
	return nil
}
