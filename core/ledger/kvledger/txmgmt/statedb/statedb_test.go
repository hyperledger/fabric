/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statedb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPaginatedRangeValidation tests queries with pagination
func TestPaginatedRangeValidation(t *testing.T) {
	queryOptions := make(map[string]interface{})
	queryOptions["limit"] = int32(10)

	err := ValidateRangeMetadata(queryOptions)
	assert.NoError(t, err, "An error was thrown for a valid option")

	queryOptions = make(map[string]interface{})
	queryOptions["limit"] = float32(10.2)

	err = ValidateRangeMetadata(queryOptions)
	assert.Error(t, err, "An should have been thrown for an invalid option")

	queryOptions = make(map[string]interface{})
	queryOptions["limit"] = "10"

	err = ValidateRangeMetadata(queryOptions)
	assert.Error(t, err, "An should have been thrown for an invalid option")

	queryOptions = make(map[string]interface{})
	queryOptions["limit1"] = int32(10)

	err = ValidateRangeMetadata(queryOptions)
	assert.Error(t, err, "An should have been thrown for an invalid option")
}
