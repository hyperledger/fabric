/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rangequery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPaginatedQueryOptionsValidation tests queries with pagination
func TestPaginatedQueryOptionValidation(t *testing.T) {
	queryOptions := make(map[string]interface{})
	queryOptions["limit"] = int32(10)

	err := ValidateOptions(queryOptions)
	assert.NoError(t, err, "an error was thrown for a valid option")

	queryOptions = make(map[string]interface{})
	queryOptions["limit"] = float32(10.2)

	err = ValidateOptions(queryOptions)
	assert.Error(t, err, "an error should have been thrown for an invalid option")

	queryOptions = make(map[string]interface{})
	queryOptions["limit"] = "10"

	err = ValidateOptions(queryOptions)
	assert.Error(t, err, "an error should have been thrown for an invalid option")

	queryOptions = make(map[string]interface{})
	queryOptions["limit1"] = int32(10)

	err = ValidateOptions(queryOptions)
	assert.Error(t, err, "an error should have been thrown for an invalid option")
}
