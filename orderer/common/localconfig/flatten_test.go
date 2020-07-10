/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package localconfig

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
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
	require.Equal(t, 4, len(x), "expect 3 items")
	require.Equal(t, x[0], "B.A = I'm 'foo'")
	require.Equal(t, x[1], "B.i = 42")
	require.Equal(t, x[2], "B.X = \"bar \"")
	require.Equal(t, x[3], "c =")
}
