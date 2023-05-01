// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bft

import (
	"github.com/SmartBFT-Go/consensus/pkg/api"
	"github.com/SmartBFT-Go/consensus/smartbftprotos"
)

// Generate mocks for a collection of interfaces that are defined in api/dependencies.go

// VerifierMock mock for the Verifier interface
//
//go:generate mockery -dir . -name VerifierMock -case underscore -output ./mocks/
type VerifierMock interface {
	api.Verifier
}

// AssemblerMock mock for the Assembler interface
//
//go:generate mockery -dir . -name AssemblerMock -case underscore -output ./mocks/
type AssemblerMock interface {
	api.Assembler
}

// ApplicationMock mock for the Application interface
//
//go:generate mockery -dir . -name ApplicationMock -case underscore -output ./mocks/
type ApplicationMock interface {
	api.Application
}

// CommMock mock for the Comm interface
//
//go:generate mockery -dir . -name CommMock -case underscore -output ./mocks/
type CommMock interface {
	api.Comm
	BroadcastConsensus(m *smartbftprotos.Message)
}

// SynchronizerMock mock for the Synchronizer interface
//
//go:generate mockery -dir . -name SynchronizerMock -case underscore -output ./mocks/
type SynchronizerMock interface {
	api.Synchronizer
}

// SignerMock mock for the Signer interface
//
//go:generate mockery -dir . -name SignerMock -case underscore -output ./mocks/
type SignerMock interface {
	api.Signer
}

// MembershipNotifierMock mock for the MembershipNotifier interface
//
//go:generate mockery -dir . -name MembershipNotifierMock -case underscore -output ./mocks/
type MembershipNotifierMock interface {
	api.MembershipNotifier
}

// Synchronizer mock for the Synchronizer interface (no return value)
//
//go:generate mockery -dir . -name Synchronizer -case underscore -output ./mocks/
type Synchronizer interface {
	Sync()
}
