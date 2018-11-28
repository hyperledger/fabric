/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	tk "github.com/hyperledger/fabric/token"
)

//go:generate counterfeiter -o mock/prover.go -fake-name Prover . Prover

type Prover interface {

	// RequestImport allows the client to submit an issue request to a prover peer service;
	// the function takes as parameters tokensToIssue and the signing identity of the client;
	// it returns a response in bytes and an error message in the case the request fails.
	// The response corresponds to a serialized TokenTransaction protobuf message.
	RequestImport(tokensToIssue []*token.TokenToIssue, signingIdentity tk.SigningIdentity) ([]byte, error)

	// RequestTransfer allows the client to submit a transfer request to a prover peer service;
	// the function takes as parameters a fabtoken application credential, the identifiers of the tokens
	// to be transfererd and the shares describing how they are going to be distributed
	// among recipients; it returns a response in bytes and an error message in the case the
	// request fails
	RequestTransfer(tokenIDs [][]byte, shares []*token.RecipientTransferShare, signingIdentity tk.SigningIdentity) ([]byte, error)
}

//go:generate counterfeiter -o mock/fabric_tx_submitter.go -fake-name FabricTxSubmitter . FabricTxSubmitter

type FabricTxSubmitter interface {

	// Submit allows the client to build and submit a fabric transaction for fabtoken that has as
	// payload a serialized tx; it takes as input an array of bytes
	// and returns an error indicating the success or the failure of the tx submission and an error
	// explaining why.
	Submit(tx []byte) error
}

// Client represents the client struct that calls Prover and TxSubmitter
type Client struct {
	SigningIdentity tk.SigningIdentity
	Prover          Prover
	TxSubmitter     FabricTxSubmitter
}

// Issue is the function that the client calls to introduce tokens into the system.
// Issue takes as parameter an array of token.TokenToIssue that define what tokens
// are going to be introduced.

func (c *Client) Issue(tokensToIssue []*token.TokenToIssue) ([]byte, error) {
	serializedTokenTx, err := c.Prover.RequestImport(tokensToIssue, c.SigningIdentity)
	if err != nil {
		return nil, err
	}

	tx, err := c.createTx(serializedTokenTx)
	if err != nil {
		return nil, err
	}

	return tx, c.TxSubmitter.Submit(tx)
}

// Transfer is the function that the client calls to transfer his tokens.
// Transfer takes as parameter an array of token.RecipientTransferShare that
// identifies who receives the tokens and describes how the tokens are distributed.
func (c *Client) Transfer(tokenIDs [][]byte, shares []*token.RecipientTransferShare) ([]byte, error) {
	serializedTokenTx, err := c.Prover.RequestTransfer(tokenIDs, shares, c.SigningIdentity)
	if err != nil {
		return nil, err
	}
	tx, err := c.createTx(serializedTokenTx)
	if err != nil {
		return nil, err
	}

	return tx, c.TxSubmitter.Submit(tx)
}

// TODO to be updated later to have a proper fabric header
// createTx is a function that creates a fabric tx form an array of bytes.
func (c *Client) createTx(tokenTx []byte) ([]byte, error) {
	payload := &common.Payload{Data: tokenTx}
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, err
	}
	signature, err := c.SigningIdentity.Sign(payloadBytes)
	if err != nil {
		return nil, err
	}
	envelope := &common.Envelope{Payload: payloadBytes, Signature: signature}
	return proto.Marshal(envelope)
}
