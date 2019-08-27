/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package golang

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_dependencyPackageInfo(t *testing.T) {
	deps, err := dependencyPackageInfo("github.com/hyperledger/fabric/cmd/peer")
	assert.NoError(t, err, "failed to get dependencyPackageInfo")

	var found bool
	for _, pi := range deps {
		if pi.ImportPath == "github.com/hyperledger/fabric/cmd/peer" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected to find the peer package")
}
