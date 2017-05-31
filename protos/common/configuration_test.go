/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfiguration(t *testing.T) {
	var h *HashingAlgorithm
	var b *BlockDataHashingStructure
	var o *OrdererAddresses
	var c *Consortium

	h = &HashingAlgorithm{Name: "SHA256"}
	h.Reset()
	_ = h.String()
	_, _ = h.Descriptor()
	h.ProtoMessage()
	assert.Equal(t, "", h.Name)

	b = &BlockDataHashingStructure{Width: uint32(1)}
	b.Reset()
	_ = b.String()
	_, _ = b.Descriptor()
	b.ProtoMessage()
	assert.Equal(t, uint32(0), b.Width)

	o = &OrdererAddresses{Addresses: []string{"address"}}
	o.Reset()
	_ = o.String()
	_, _ = o.Descriptor()
	o.ProtoMessage()
	assert.Nil(t, o.Addresses)

	c = &Consortium{Name: "consortium"}
	c.Reset()
	_ = c.String()
	_, _ = c.Descriptor()
	c.ProtoMessage()
	assert.Equal(t, "", c.Name)

}
