/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver_test

import (
	"github.com/hyperledger/fabric/common/ledger/blockledger"
)

//go:generate counterfeiter -o mock/block_reader.go -fake-name BlockReader . blockledgerReader
type blockledgerReader interface {
	blockledger.Reader
}

//go:generate counterfeiter -o mock/block_iterator.go -fake-name BlockIterator . blockledgerIterator
type blockledgerIterator interface {
	blockledger.Iterator
}
