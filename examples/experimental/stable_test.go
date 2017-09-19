// +build !experimental

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package experimental

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testIntf interface {
	DoSomethingExperimental() string
}

func TestStable(t *testing.T) {
	var m MyInterface

	m = &MyType{}

	something := "did something"

	assert.Equal(t, something, m.DoSomething())

	// make sure that MyType does not implement testIntf
	_, ok := m.(testIntf)
	if ok {
		t.Error("MyType (stable) should not have DoSomethingExperimental method")
	}
}
