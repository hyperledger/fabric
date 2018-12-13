// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	ab "github.com/hyperledger/fabric/protos/common"
)

// Helper defines the functions a bootstrapping implementation should provide.
type Helper interface {
	// GenesisBlock should return the genesis block required to bootstrap
	// the ledger (be it reading from the filesystem, generating it, etc.)
	GenesisBlock() *ab.Block
}

// Replacer provides the ability to to replace the current genesis block used
// for bootstrapping with the supplied block. It is used during consensus-type
// migration in order to replace the original genesis block used for
// bootstrapping with the latest config block of the system channel, which
// contains the new consensus-type. This will ensure the instantiation of the
// correct consenter type when the server restarts.
type Replacer interface {
	// ReplaceGenesisBlockFile should first copy the current file to a backup
	// file: <genesis-file-name> => <genesis-file-name>.bak
	// and then overwrite the original file with the content of the given block.
	// If something goes wrong during migration, the original file could be
	// restored from the backup.
	// An error is returned if the operation was not completed successfully.
	ReplaceGenesisBlockFile(block *ab.Block) error

	// CheckReadWrite checks whether the current file is readable and writable,
	// because if it is not, there is no point in attempting to replace. This
	// check is performed at the beginning of the consensus-type migration
	// process.
	// An error is returned if the file is not readable and writable.
	CheckReadWrite() error
}
