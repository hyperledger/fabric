/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	tclient "github.com/hyperledger/fabric/token/client"
	"github.com/pkg/errors"
)

type TokenClientStub struct {
	client *tclient.Client
}

func (stub *TokenClientStub) Setup(configPath, channel, mspPath, mspID string) error {
	config, err := LoadConfig(configPath)
	if err != nil {
		return err
	}

	if len(channel) != 0 {
		config.ChannelID = channel
	}
	if len(mspPath) != 0 {
		config.MSPInfo.MSPConfigPath = mspPath
	}
	if len(mspID) != 0 {
		config.MSPInfo.MSPID = mspID
	}

	sId, err := GetSigningIdentity(config.MSPInfo.MSPConfigPath, config.MSPInfo.MSPID, "bccsp")
	if err != nil {
		return err
	}

	stub.client, err = tclient.NewClient(*config, sId)
	return err
}

func (stub *TokenClientStub) Issue(tokensToIssue []*token.Token, waitTimeout time.Duration) (StubResponse, error) {
	if stub.client == nil {
		return nil, errors.New("stub not initialised!!!")
	}

	envelope, txid, ordererStatus, committed, err := stub.client.Issue(tokensToIssue, waitTimeout)
	return &OperationResponse{Envelope: envelope, TxID: txid, Status: ordererStatus, Committed: committed}, err
}

func (stub *TokenClientStub) Transfer(tokenIDs []*token.TokenId, shares []*token.RecipientShare, waitTimeout time.Duration) (StubResponse, error) {
	if stub.client == nil {
		return nil, errors.New("stub not initialised!!!")
	}

	envelope, txid, ordererStatus, committed, err := stub.client.Transfer(tokenIDs, shares, waitTimeout)
	return &OperationResponse{Envelope: envelope, TxID: txid, Status: ordererStatus, Committed: committed}, err
}

func (stub *TokenClientStub) Redeem(tokenIDs []*token.TokenId, quantity string, waitTimeout time.Duration) (StubResponse, error) {
	if stub.client == nil {
		return nil, errors.New("stub not initialised!!!")
	}

	envelope, txid, ordererStatus, committed, err := stub.client.Redeem(tokenIDs, quantity, waitTimeout)
	return &OperationResponse{Envelope: envelope, TxID: txid, Status: ordererStatus, Committed: committed}, err
}

func (stub *TokenClientStub) ListTokens() (StubResponse, error) {
	if stub.client == nil {
		return nil, errors.New("stub not initialised!!!")
	}

	outputs, err := stub.client.ListTokens()
	return &UnspentTokenResponse{Tokens: outputs}, err
}

type OperationResponse struct {
	Envelope  *common.Envelope
	TxID      string
	Status    *common.Status
	Committed bool
}

type UnspentTokenResponse struct {
	Tokens []*token.UnspentToken
}

// OperationResponseParser parses operation responses
type OperationResponseParser struct {
	io.Writer
}

// ParseResponse parses the given response for the given channel
func (parser *OperationResponseParser) ParseResponse(response StubResponse) error {
	resp := response.(*OperationResponse)

	if resp.Envelope == nil {
		return errors.New("nil envelope")
	}

	payload := common.Payload{}
	err := proto.Unmarshal(resp.Envelope.Payload, &payload)
	if err != nil {
		return err
	}
	tokenTx := &token.TokenTransaction{}
	err = proto.Unmarshal(payload.Data, tokenTx)
	if err != nil {
		return err
	}
	tokenTxid, err := tclient.GetTransactionID(resp.Envelope)
	if err != nil {
		return err
	}
	if resp.TxID != tokenTxid {
		return errors.Errorf("got different transaction ids [%s], [%s]", resp.TxID, tokenTxid)
	}

	fmt.Fprintf(parser.Writer, "Orderer Status [%s]", resp.Status)
	fmt.Fprintf(parser.Writer, "Committed [%v]", resp.Committed)

	return nil
}

// UnspentTokenResponseParser parses import responses
type UnspentTokenResponseParser struct {
	io.Writer
}

// ParseResponse parses the given response for the given channel
func (parser *UnspentTokenResponseParser) ParseResponse(response StubResponse) error {
	resp := response.(*UnspentTokenResponse)

	for _, token := range resp.Tokens {
		out, _ := json.Marshal(token)
		fmt.Fprintf(parser.Writer, "Token = %s", string(out))
	}
	return nil
}
