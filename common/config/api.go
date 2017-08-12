/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

type ValueProposer interface {
	// BeginValueProposals called when a config proposal is begun
	BeginValueProposals(tx interface{}, groups []string) (ValueDeserializer, []ValueProposer, error)

	// RollbackProposals called when a config proposal is abandoned
	RollbackProposals(tx interface{})

	// PreCommit is invoked before committing the config to catch
	// any errors which cannot be caught on a per proposal basis
	// TODO, rename other methods to remove Value/Proposal references
	PreCommit(tx interface{}) error

	// CommitProposals called when a config proposal is committed
	CommitProposals(tx interface{})
}
