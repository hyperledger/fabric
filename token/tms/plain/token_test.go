/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"encoding/asn1"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToken(t *testing.T) {
	// set token values
	name := "alice"
	credBytes := []byte(name)
	tokenType := "wild_pineapple"
	quantity := uint64(100)
	// create a token
	aliceToken := &Token{Owner: credBytes, Type: tokenType, Quantity: quantity}
	assert.NotNil(t, aliceToken)
	// serialize the token
	tokenBytes, err := aliceToken.MarshalBinary()
	assert.NoError(t, err)
	assert.NotNil(t, tokenBytes)
	// deserialize the serialized token
	parsedToken := &Token{}
	err = parsedToken.UnmarshalBinary(tokenBytes)
	assert.NoError(t, err)
	assert.Equal(t, aliceToken, parsedToken)
	// serialize the deserialized token
	parsedTokenBytes, err := parsedToken.MarshalBinary()
	assert.NoError(t, err)
	assert.NotNil(t, parsedTokenBytes)
	assert.Equal(t, tokenBytes, parsedTokenBytes)
}

func TestParsingEmptyToken(t *testing.T) {
	// serialized token is slice of zero length
	tokenBytes := make([]byte, 0)
	parsedToken := &Token{}
	err := parsedToken.UnmarshalBinary(tokenBytes)
	assert.Error(t, err)
}

func TestParsingBadToken(t *testing.T) {
	// invalid asn1 encoding
	tokenBytes, err := hex.DecodeString("deadbeef")
	assert.NoError(t, err)
	parsedToken := &Token{}
	err = parsedToken.UnmarshalBinary(tokenBytes)
	assert.Error(t, err)
}

func TestParsingTokenWithIncompatibleAsn1Struct(t *testing.T) {
	// serialized incompatible struct
	name := "wild_pineapple"
	otherStruct := struct {
		Name string
	}{
		Name: name,
	}
	tokenBytes, err := asn1.Marshal(otherStruct)
	assert.NoError(t, err)
	assert.NotNil(t, tokenBytes)
	parsedToken := &Token{}
	err = parsedToken.UnmarshalBinary(tokenBytes)
	assert.Error(t, err)
}

func TestParsingTokenWithIncorrectNumberOfEntries(t *testing.T) {
	// serialized struct with invalid number of entries
	data := raw{}
	data.Entries = make([][]byte, 4)
	tokenBytes, err := asn1.Marshal(data)
	assert.NoError(t, err)
	assert.NotNil(t, tokenBytes)
	parsedToken := &Token{}
	err = parsedToken.UnmarshalBinary(tokenBytes)
	assert.Error(t, err)
}

func TestSerializeNilToken(t *testing.T) {
	var nilToken *Token
	nilToken = nil
	bytes, err := nilToken.MarshalBinary()
	assert.Error(t, err)
	assert.Nil(t, bytes)
}
