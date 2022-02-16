/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgerutil

import (
	"testing"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/stretchr/testify/require"
)

func TestTimestampToStr(t *testing.T) {
	ts := &timestamp.Timestamp{
		Seconds: 7891,
		Nanos:   123,
	}

	require.Equal(t, timestampToStr(ts), "7891.000000123")
}
