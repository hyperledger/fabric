// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bft

import (
	"github.com/hyperledger/SmartBFT/pkg/api"
)

// Generate mocks for a collection of interfaces that are defined in api/dependencies.go

// VerifierMock mock for the Verifier interface
type VerifierMock interface {
	api.Verifier
}

// AssemblerMock mock for the Assembler interface
type AssemblerMock interface {
	api.Assembler
}

// ApplicationMock mock for the Application interface
type ApplicationMock interface {
	api.Application
}

// CommMock mock for the Comm interface
type CommMock = Comm

// SynchronizerMock mock for the Synchronizer interface
type SynchronizerMock interface {
	api.Synchronizer
}

// SignerMock mock for the Signer interface
type SignerMock interface {
	api.Signer
}

// MembershipNotifierMock mock for the MembershipNotifier interface
type MembershipNotifierMock interface {
	api.MembershipNotifier
}

// Synchronizer mock for the Synchronizer interface (no return value)
type Synchronizer interface {
	Sync()
}
