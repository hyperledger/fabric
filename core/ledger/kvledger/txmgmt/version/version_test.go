/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionSerialization(t *testing.T) {
	h1 := NewHeight(10, 100)
	b := h1.ToBytes()
	h2, n, err := NewHeightFromBytes(b)
	assert.NoError(t, err)
	assert.Equal(t, h1, h2)
	assert.Len(t, b, n)
}

func TestVersionComparison(t *testing.T) {
	assert.Equal(t, 1, NewHeight(10, 100).Compare(NewHeight(9, 1000)))
	assert.Equal(t, 1, NewHeight(10, 100).Compare(NewHeight(10, 90)))
	assert.Equal(t, -1, NewHeight(10, 100).Compare(NewHeight(11, 1)))
	assert.Equal(t, 0, NewHeight(10, 100).Compare(NewHeight(10, 100)))

	assert.True(t, AreSame(NewHeight(10, 100), NewHeight(10, 100)))
	assert.True(t, AreSame(nil, nil))
	assert.False(t, AreSame(NewHeight(10, 100), nil))
}

func TestVersionExtraBytes(t *testing.T) {
	extraBytes := []byte("junk")
	h1 := NewHeight(10, 100)
	b := h1.ToBytes()
	b1 := append(b, extraBytes...)
	h2, n, err := NewHeightFromBytes(b1)
	assert.NoError(t, err)
	assert.Equal(t, h1, h2)
	assert.Len(t, b, n)
	assert.Equal(t, extraBytes, b1[n:])
}
