/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package localconfig

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type A struct {
	s string
}

type B struct {
	A A
	i int
	X string
}

type C struct{}

type D struct {
	B B
	c *C
}

func (a A) String() string {
	return fmt.Sprintf("I'm '%s'", a.s)
}

func TestFlattenStruct(t *testing.T) {
	d := &D{
		B: B{
			A: A{
				s: "foo",
			},
			i: 42,
			X: "bar ",
		},
		c: nil,
	}

	var x []string
	flatten("", &x, reflect.ValueOf(d))
	assert.Equal(t, 4, len(x), "expect 3 items")
	assert.Equal(t, x[0], "B.A = I'm 'foo'")
	assert.Equal(t, x[1], "B.i = 42")
	assert.Equal(t, x[2], "B.X = \"bar \"")
	assert.Equal(t, x[3], "c =")
}
