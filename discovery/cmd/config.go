/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hyperledger/fabric/cmd/common"
	discovery "github.com/hyperledger/fabric/discovery/client"
	"github.com/pkg/errors"
)

// NewConfigCmd creates a new ConfigCmd
func NewConfigCmd(stub Stub, parser ResponseParser) *ConfigCmd {
	return &ConfigCmd{
		stub:   stub,
		parser: parser,
	}
}

// ConfigCmd executes a command that retrieves config
type ConfigCmd struct {
	stub    Stub
	server  *string
	channel *string
	parser  ResponseParser
}

// SetServer sets the server of the ConfigCmd
func (pc *ConfigCmd) SetServer(server *string) {
	pc.server = server
}

// SetChannel sets the channel of the ConfigCmd
func (pc *ConfigCmd) SetChannel(channel *string) {
	pc.channel = channel
}

// Execute executes the command
func (pc *ConfigCmd) Execute(conf common.Config) error {
	if pc.server == nil || *pc.server == "" {
		return errors.New("no server specified")
	}
	if pc.channel == nil || *pc.channel == "" {
		return errors.New("no channel specified")
	}

	server := *pc.server
	channel := *pc.channel

	req := discovery.NewRequest().OfChannel(channel).AddConfigQuery()
	res, err := pc.stub.Send(server, conf, req)
	if err != nil {
		return err
	}
	return pc.parser.ParseResponse(channel, res)
}

// ConfigResponseParser parses config responses
type ConfigResponseParser struct {
	io.Writer
}

// ParseResponse parses the given response for the given channel
func (parser *ConfigResponseParser) ParseResponse(channel string, res ServiceResponse) error {
	chanConf, err := res.ForChannel(channel).Config()
	if err != nil {
		return err
	}
	jsonBytes, _ := json.MarshalIndent(chanConf, "", "\t")
	fmt.Fprintln(parser.Writer, string(jsonBytes))
	return nil
}
