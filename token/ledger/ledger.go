/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

// LedgerReader interface, used to read from a ledger.
type LedgerReader interface {
	// GetState gets the value for given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	GetState(namespace string, key string) ([]byte, error)
}

//go:generate counterfeiter -o mock/ledger_writer.go -fake-name LedgerWriter . LedgerWriter

// LedgerWriter interface, used to read from, and write to, a ledger.
type LedgerWriter interface {
	LedgerReader
	// SetState sets the given value for the given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	SetState(namespace string, key string, value []byte) error
}
