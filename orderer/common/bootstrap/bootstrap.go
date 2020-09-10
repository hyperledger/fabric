// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"github.com/hyperledger/fabric-protos-go/common"
)

// Helper defines the functions a bootstrapping implementation should provide.
type Helper interface {
	// GenesisBlock should return the genesis block required to bootstrap
	// the ledger (be it reading from the filesystem, generating it, etc.)
	GenesisBlock() *common.Block

	// SaveBlock persists the block to the file path specified by the Helper.
	// This is used to save a system channel join-block when the system channel is created using the channel
	// participation API.
	// It will fail if: the block cannot be marshaled, the file exists, or cannot be written.
	SaveBlock(block *common.Block) error
}
