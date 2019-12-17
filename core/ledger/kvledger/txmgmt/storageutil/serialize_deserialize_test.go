/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package storageutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSerializeDeSerialize(t *testing.T) {
	sampleMetadata := map[string][]byte{
		"metadata_1": []byte("metadata_value_1"),
		"metadata_2": []byte("metadata_value_2"),
		"metadata_3": []byte("metadata_value_3"),
	}

	serializedMetadata, err := SerializeMetadataByMap(sampleMetadata)
	assert.NoError(t, err)
	metadataMap, err := DeserializeMetadata(serializedMetadata)
	assert.NoError(t, err)
	assert.Equal(t, sampleMetadata, metadataMap)
}
