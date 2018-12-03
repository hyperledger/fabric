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
	// Issue creates an import request transaction.
	RequestImport(tokensToIssue []*token.TokenToIssue) (*token.TokenTransaction, error)

	// RequestExpectation allows indirect import based on the expectation.
	// It creates a token transaction with the outputs as specified in the expectation.
	RequestExpectation(request *token.ExpectationRequest) (*token.TokenTransaction, error)
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

	// RequestApprove creates a token transaction that includes the data necessary
	// for approve
	RequestApprove(request *token.ApproveRequest) (*token.TokenTransaction, error)

	// RequestTransferFrom creates a token transaction that includes the data necessary
	// for transferring the tokens of a third party that previsouly delegated the transfer
	// via an approve request
	RequestTransferFrom(request *token.TransferRequest) (*token.TokenTransaction, error)

	// RequestExpectation allows indirect transfer based on the expectation.
	// It creates a token transaction with the outputs as specified in the expectation.
	RequestExpectation(request *token.ExpectationRequest) (*token.TokenTransaction, error)

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
