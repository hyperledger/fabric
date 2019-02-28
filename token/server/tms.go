/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package server

import (
	"github.com/hyperledger/fabric/protos/token"
)

//go:generate counterfeiter -o mock/issuer.go -fake-name Issuer . Issuer

// An Issuer creates token import requests.
type Issuer interface {
	// RequestIssue creates an issue request transaction.
	RequestIssue(tokensToIssue []*token.Token) (*token.TokenTransaction, error)

	// RequestTokenOperation returns a token transaction matching the requested issue operation
	RequestTokenOperation(op *token.TokenOperation) (*token.TokenTransaction, error)
}

//go:generate counterfeiter -o mock/transactor.go -fake-name Transactor . Transactor

// Transactor allows to operate on issued tokens
type Transactor interface {
	// RequestTransfer Create data associated to the transfer of a token assuming
	// an application-level identity. The inTokens bytes are the identifiers
	// of the outputs, the details of which need to be looked up from the ledger.
	RequestTransfer(request *token.TransferRequest) (*token.TokenTransaction, error)

	// RequestRedeem allows the redemption of the tokens in the input tokenIds
	// It queries the ledger to read detail for each token id.
	// It creates a token transaction with an output for redeemed tokens and
	// possibly another output to transfer the remaining tokens, if any, to the creator
	RequestRedeem(request *token.RedeemRequest) (*token.TokenTransaction, error)

	// ListTokens returns a slice of unspent tokens owned by this transactor
	ListTokens() (*token.UnspentTokens, error)

	// RequestTokenOperation returns a token transaction matching the requested transfer operation
	RequestTokenOperation(tokenIDs []*token.TokenId, op *token.TokenOperation) (*token.TokenTransaction, int, error)

	// Done releases any resources held by this transactor
	Done()
}

//go:generate counterfeiter -o mock/tms_manager.go -fake-name TMSManager . TMSManager

type TMSManager interface {
	// GetIssuer returns an Issuer bound to the passed channel and whose credential
	// is the tuple (privateCredential, publicCredential).
	GetIssuer(channel string, privateCredential, publicCredential []byte) (Issuer, error)

	// GetTransactor returns a Transactor bound to the passed channel and whose credential
	// is the tuple (privateCredential, publicCredential).
	GetTransactor(channel string, privateCredential, publicCredential []byte) (Transactor, error)
}
