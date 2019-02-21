/*
Copyright 2017 - Greg Haskins <gregory.haskins@gmail.com>

SPDX-License-Identifier: Apache-2.0
*/

package golang

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_listDeps(t *testing.T) {
	_, err := listDeps(nil, "github.com/hyperledger/fabric/cmd/peer")
	if err != nil {
		t.Errorf("list failed: %s", err)
	}
}

func Test_runProgram(t *testing.T) {
	_, err := runProgram(
		getEnv(),
		10*time.Millisecond,
		"go",
		"build",
		"github.com/hyperledger/fabric/cmd/peer",
	)
	assert.Contains(t, err.Error(), "timed out")

	_, err = runProgram(
		getEnv(),
		1*time.Second,
		"go",
		"cmddoesnotexist",
	)
	assert.Contains(t, err.Error(), "unknown command")
}
