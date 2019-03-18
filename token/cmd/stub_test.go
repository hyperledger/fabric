/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token_test

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	common2 "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	. "github.com/hyperledger/fabric/token/cmd"
	"github.com/stretchr/testify/assert"
)

func TestTokenClientStub_Setup(t *testing.T) {
	stub := &TokenClientStub{}
	err := stub.Setup("./testdata/config.json", "test_channel", "./testdata/no_msp_dir", "MSPID")
	assert.Error(t, err)

	// Failure
	stub = &TokenClientStub{}
	err = stub.Setup("./testdata/no_config_file.json", "test_channel", "./testdata/mspid", "MSPID")
	assert.Error(t, err)
	_, err = stub.Issue(nil, 20)
	assert.Equal(t, "stub not initialised!!!", err.Error())
	_, err = stub.ListTokens()
	assert.Equal(t, "stub not initialised!!!", err.Error())
	_, err = stub.Redeem(nil, ToHex(0), 10)
	assert.Equal(t, "stub not initialised!!!", err.Error())
	_, err = stub.Transfer(nil, nil, 10)
	assert.Equal(t, "stub not initialised!!!", err.Error())
}

func TestUnspentTokenResponseParser_ParseResponse(t *testing.T) {
	buffer := &bytes.Buffer{}
	parser := &UnspentTokenResponseParser{Writer: buffer}

	unspentTokens := []*token.UnspentToken{
		{
			Type: "token_type",
			Id:   &token.TokenId{TxId: "0", Index: 1},
		},
	}

	resp := &UnspentTokenResponse{Tokens: unspentTokens}
	err := parser.ParseResponse(resp)
	assert.NoError(t, err)
	assert.Equal(t, "{\"tx_id\":\"0\",\"index\":1}\n[token_type,]\n", buffer.String())

	// Parse back unspent tokens
	unspentTokens2, err := ExtractUnspentTokensFromOutput(buffer.String())
	assert.NoError(t, err)
	assert.Equal(t, len(unspentTokens), len(unspentTokens2))
	for i, ut := range unspentTokens {
		assert.True(t, proto.Equal(ut, unspentTokens2[i]))
	}
}

func TestOperationResponseParser_ParseResponse(t *testing.T) {
	buffer := &bytes.Buffer{}
	parser := &OperationResponseParser{Writer: buffer}

	resp := &OperationResponse{
		Envelope: nil,
	}
	err := parser.ParseResponse(resp)
	assert.Error(t, err)

	resp = &OperationResponse{
		Envelope: &common2.Envelope{Payload: []byte{1, 2, 3, 4}},
	}
	err = parser.ParseResponse(resp)
	assert.Error(t, err)
}
