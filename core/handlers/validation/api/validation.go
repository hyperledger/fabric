/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import "github.com/hyperledger/fabric/protos/common"

// Argument defines the argument for validation
type Argument interface {
	Dependency
	// Arg returns the bytes of the argument
	Arg() []byte
}

// Dependency marks a dependency passed to the Init() method
type Dependency interface {
}

// Plugin validates transactions
type Plugin interface {
	// Validate returns nil if the action at the given position inside the transaction
	// at the given position in the given block is valid, or an error if not.
	Validate(block *common.Block, namespace string, txPosition int, actionPosition int) error

	// Init injects dependencies into the instance of the Plugin
	Init(dependencies ...Dependency) error
}

// PluginFactory creates a new instance of a Plugin
type PluginFactory interface {
	New() Plugin
}
