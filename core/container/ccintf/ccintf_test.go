/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccintf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetName(t *testing.T) {
	t.Run("Complete", func(t *testing.T) {
		ccid := &CCID{Name: "ccname", Version: "ver"}
		name := ccid.GetName()
		assert.Equal(t, "ccname-ver", name, "unexpected name")
	})

	t.Run("MissingVersion", func(t *testing.T) {
		ccid := &CCID{Name: "ccname"}
		name := ccid.GetName()
		assert.Equal(t, "ccname", name)
	})
}
