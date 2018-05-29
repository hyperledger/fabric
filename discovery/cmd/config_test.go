/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/cmd/common"
	"github.com/hyperledger/fabric/discovery/cmd"
	"github.com/hyperledger/fabric/discovery/cmd/mocks"
	. "github.com/hyperledger/fabric/protos/discovery"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestConfigCmd(t *testing.T) {
	server := "peer0"
	channel := "mychannel"
	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	cmd := discovery.NewConfigCmd(stub, parser)

	t.Run("no server supplied", func(t *testing.T) {
		cmd.SetChannel(&channel)
		cmd.SetServer(nil)

		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no server specified")
	})

	t.Run("no channel supplied", func(t *testing.T) {
		cmd.SetChannel(nil)
		cmd.SetServer(&server)

		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no channel specified")
	})

	t.Run("Server return error", func(t *testing.T) {
		cmd.SetChannel(&channel)
		cmd.SetServer(&server)

		stub.On("Send", server, mock.Anything, mock.Anything).Return(nil, errors.New("deadline exceeded")).Once()
		err := cmd.Execute(common.Config{})
		assert.Contains(t, err.Error(), "deadline exceeded")
	})

	t.Run("Config query", func(t *testing.T) {
		cmd.SetServer(&server)
		cmd.SetChannel(&channel)
		stub.On("Send", server, mock.Anything, mock.Anything).Return(nil, nil).Once()
		cmd.SetServer(&server)
		parser.On("ParseResponse", channel, mock.Anything).Return(nil)

		err := cmd.Execute(common.Config{})
		assert.NoError(t, err)
	})
}

func TestParseConfigResponse(t *testing.T) {
	buff := &bytes.Buffer{}
	parser := &discovery.ConfigResponseParser{Writer: buff}
	res := &mocks.ServiceResponse{}
	chanRes := &mocks.ChannelResponse{}

	t.Run("Failure", func(t *testing.T) {

		chanRes.On("Config").Return(nil, errors.New("not found")).Once()
		res.On("ForChannel", "mychannel").Return(chanRes)
		err := parser.ParseResponse("mychannel", res)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Success", func(t *testing.T) {
		chanRes.On("Config").Return(&ConfigResult{
			Msps: map[string]*msp.FabricMSPConfig{
				"Org1MSP": nil,
				"Org2MSP": nil,
			},
			Orderers: map[string]*Endpoints{
				"OrdererMSP": {Endpoint: []*Endpoint{
					{Host: "orderer1", Port: 7050},
				}},
			},
		}, nil).Once()
		res.On("ForChannel", "mychannel").Return(chanRes)

		err := parser.ParseResponse("mychannel", res)
		assert.NoError(t, err)
		expected := "{\n\t\"msps\": {\n\t\t\"Org1MSP\": null,\n\t\t\"Org2MSP\": null\n\t},\n\t\"orderers\": {\n\t\t\"OrdererMSP\": {\n\t\t\t\"endpoint\": [\n\t\t\t\t{\n\t\t\t\t\t\"host\": \"orderer1\",\n\t\t\t\t\t\"port\": 7050\n\t\t\t\t}\n\t\t\t]\n\t\t}\n\t}\n}"
		assert.Equal(t, fmt.Sprintf("%s\n", expected), buff.String())
	})
}
