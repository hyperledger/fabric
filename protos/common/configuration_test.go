/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
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

	h = nil
	assert.Equal(t, "", h.GetName())
	h = &HashingAlgorithm{Name: "SHA256"}
	assert.Equal(t, "SHA256", h.GetName())
	h.Reset()
	_ = h.String()
	_, _ = h.Descriptor()
	h.ProtoMessage()
	assert.Equal(t, "", h.GetName())

	b = nil
	assert.Equal(t, uint32(0), b.GetWidth())
	b = &BlockDataHashingStructure{Width: uint32(1)}
	assert.Equal(t, uint32(1), b.GetWidth())
	b.Reset()
	_ = b.String()
	_, _ = b.Descriptor()
	b.ProtoMessage()
	assert.Equal(t, uint32(0), b.GetWidth())

	o = nil
	assert.Nil(t, o.GetAddresses())
	o = &OrdererAddresses{Addresses: []string{"address"}}
	assert.Equal(t, "address", o.GetAddresses()[0])
	o.Reset()
	_ = o.String()
	_, _ = o.Descriptor()
	o.ProtoMessage()
	assert.Nil(t, o.GetAddresses())

	c = nil
	assert.Equal(t, "", c.GetName())
	c = &Consortium{Name: "consortium"}
	assert.Equal(t, "consortium", c.GetName())
	c.Reset()
	_ = c.String()
	_, _ = c.Descriptor()
	c.ProtoMessage()
	assert.Equal(t, "", c.GetName())

}
