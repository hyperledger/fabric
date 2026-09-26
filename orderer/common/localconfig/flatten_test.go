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
	require.Len(t, x, 4, "expect 3 items")
	require.Equal(t, "B.A = I'm 'foo'", x[0])
	require.Equal(t, "B.i = 42", x[1])
	require.Equal(t, "B.X = \"bar \"", x[2])
	require.Equal(t, "c =", x[3])
}
