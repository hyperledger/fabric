/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/hyperledger/fabric/protos/token"
	. "github.com/hyperledger/fabric/token/cmd"
	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	config, err := LoadConfig("./testdata/config.json")
	assert.NoError(t, err)

	config2, err := LoadConfig("{\"ChannelID\":\"\",\"MSPInfo\":{\"MSPConfigPath\":\"\",\"MSPID\":\"\",\"MSPType\":\"\"},\"Orderer\":{\"Address\":\"127.0.0.1:7050\",\"ConnectionTimeout\":0,\"TLSEnabled\":false,\"TLSRootCertFile\":\"\",\"ServerNameOverride\":\"\"},\"CommitterPeer\":{\"Address\":\"127.0.0.1:7051\",\"ConnectionTimeout\":0,\"TLSEnabled\":false,\"TLSRootCertFile\":\"\",\"ServerNameOverride\":\"\"},\"ProverPeer\":{\"Address\":\"127.0.0.1:7051\",\"ConnectionTimeout\":0,\"TLSEnabled\":false,\"TLSRootCertFile\":\"\",\"ServerNameOverride\":\"\"}}")
	assert.NoError(t, err)

	assert.Equal(t, config, config2)
}

func TestLoadTokenIDs(t *testing.T) {
	tokenIDs := []*token.TokenId{{TxId: "1", Index: 1}, {TxId: "2", Index: 1}}
	jsonBytes, err := json.Marshal(tokenIDs)
	assert.NoError(t, err)

	tokenIDs, err = LoadTokenIDs(string(jsonBytes))
	assert.NoError(t, err)
	jsonBytes2, err := json.Marshal(tokenIDs)
	assert.NoError(t, err)

	assert.Equal(t, jsonBytes, jsonBytes2)

	_, err = LoadTokenIDsFromFile("./testdata/no_file.json")
	assert.Error(t, err)

	tokenIDs, err = LoadTokenIDsFromFile("./testdata/tokenids.json")
	assert.NoError(t, err)
	jsonBytes2, err = json.Marshal(tokenIDs)
	assert.NoError(t, err)

	assert.Equal(t, jsonBytes, jsonBytes2)

	jsonLoader := &JsonLoader{}
	tokenIDs, err = jsonLoader.TokenIDs("./testdata/tokenids.json")
	assert.NoError(t, err)
	jsonBytes2, err = json.Marshal(tokenIDs)
	assert.NoError(t, err)

	assert.Equal(t, jsonBytes, jsonBytes2)
}

func TestShares(t *testing.T) {
	shares := []*token.RecipientShare{
		{
			Quantity:  ToHex(10),
			Recipient: &token.TokenOwner{Raw: []byte{0, 1, 2, 3, 4}},
		},
		{
			Quantity:  ToHex(20),
			Recipient: &token.TokenOwner{Raw: []byte{5, 6, 7, 8, 9}},
		},
	}
	jsonBytes, err := json.Marshal(shares)
	assert.NoError(t, err)

	shares, err = LoadShares(string(jsonBytes))
	assert.NoError(t, err)
	jsonBytes2, err := json.Marshal(shares)
	assert.NoError(t, err)
	assert.Equal(t, jsonBytes, jsonBytes2)

	_, err = LoadSharesFromFile("./testdata/not_a_file.json")
	assert.Error(t, err)

	shares, err = LoadSharesFromFile("./testdata/shares.json")
	assert.NoError(t, err)
	jsonBytes2, err = json.Marshal(shares)
	assert.NoError(t, err)
	assert.Equal(t, jsonBytes, jsonBytes2)

	jsonLoader := &JsonLoader{}
	shares, err = jsonLoader.Shares("./testdata/shares.json")
	assert.NoError(t, err)
	jsonBytes2, err = json.Marshal(shares)
	assert.NoError(t, err)

	assert.Equal(t, jsonBytes, jsonBytes2)
}

func TestGetSigningIdentity(t *testing.T) {
	_, err := GetSigningIdentity("", "", "invalid")
	assert.Error(t, err)

	_, err = GetSigningIdentity("./testdata/mspid", "MSP_ID", "bccsp")
	assert.NoError(t, err)
}

func TestLoadLocalMspRecipient(t *testing.T) {
	owner, err := LoadLocalMspRecipient("./testdata/mspid", "MSP_ID")
	assert.NoError(t, err)

	assert.Equal(t, token.TokenOwner_MSP_IDENTIFIER, owner.Type)

	jsonLoader := &JsonLoader{}
	owner2, err := jsonLoader.TokenOwner("./testdata/mspid")
	assert.NoError(t, err)
	assert.Equal(t, owner, owner2)
}

func ToHex(q uint64) string {
	return "0x" + strconv.FormatUint(q, 16)
}
