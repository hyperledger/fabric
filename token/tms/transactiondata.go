/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tms

import "github.com/hyperledger/fabric/protos/token"

// TransactionData struct contains the token transaction, and the Fabric transaction ID.
// The TokenTransaction is created by the prover peer, but the transaction ID is only created
// later by the client (using the TokenTransaction and a nonce). At validation and commit
// time, both the TokenTransaction, and the transaction ID are needed by the committing peer.
// Storing them together in a single struct facilitates the handling of them.
type TransactionData struct {
	Tx *token.TokenTransaction
	// The fabric transaction ID
	TxID string
}
