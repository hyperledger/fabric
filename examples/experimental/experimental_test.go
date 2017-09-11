// +build experimental

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package experimental

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExperimental(t *testing.T) {
	m := MyType{}

	something := "did something"
	somethingExperimental := "did something experimental"

	assert.Equal(t, something, m.DoSomething())
	assert.Equal(t, somethingExperimental, m.DoSomethingExperimental())
}
