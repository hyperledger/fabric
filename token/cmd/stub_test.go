/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token

import (
	"bytes"
	"testing"

	common2 "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
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
	_, err = stub.Redeem(nil, 0, 10)
	assert.Equal(t, "stub not initialised!!!", err.Error())
	_, err = stub.Transfer(nil, nil, 10)
	assert.Equal(t, "stub not initialised!!!", err.Error())
}

func TestTokenOutputResponseParser_ParseResponse(t *testing.T) {
	buffer := &bytes.Buffer{}
	parser := &TokenOutputResponseParser{Writer: buffer}

	resp := &TokenOutputResponse{[]*token.TokenOutput{
		{Type: "token_type", Id: &token.TokenId{TxId: "0", Index: 1}},
	}}
	err := parser.ParseResponse(resp)
	assert.NoError(t, err)
	assert.Equal(t, "Token[0] = [{\n    \"id\": {\n        \"tx_id\": \"0\",\n        \"index\": 1\n    },\n    \"type\": \"token_type\"\n}]", buffer.String())
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
