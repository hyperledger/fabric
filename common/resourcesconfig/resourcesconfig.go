/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/policies"
)

// PolicyMapper is an interface for
type PolicyMapper interface {
	// PolicyRefForAPI takes the name of an API, and returns the policy name
	// or the empty string if the API is not found
	PolicyRefForAPI(apiName string) string
}

// ChaincodeRegistry allows for querying chaincode info by name
type ChaincodeRegistry interface {
	// ChaincodeByName returns the chaincode definition if it is exists, and whether it exists
	ChaincodeByName(name string) (ChaincodeDefinition, bool)
}

// ChaincodeDefinition describes all of the necessary information for a peer to decide whether to endorse
// a proposal and whether to validate a transaction, for a particular chaincode.
type ChaincodeDefinition interface {
	// CCName returns the name of this chaincode (the name it was put in the ChaincodeRegistry with).
	CCName() string

	// Hash returns the hash of the chaincode.
	Hash() []byte

	// CCVersion returns the version of the chaincode.
	CCVersion() string

	// Validation returns how to validate transactions for this chaincode.
	// The string returned is the name of the validation method (usually 'vscc')
	// and the bytes returned are the argument to the validation (in the case of
	// 'vscc', this is a marshaled pb.VSCCArgs message).
	Validation() (string, []byte)

	// Endorsement returns how to endorse proposals for this chaincode.
	// The string returns is the name of the endorsement method (usually 'escc').
	Endorsement() string
}

// Resources defines a way to query peer resources associated with a channel.
type Resources interface {
	// ConfigtxValidator returns a reference to a configtx.Validator which can process updates to this config.
	ConfigtxValidator() configtx.Validator

	// PolicyManager returns a policy manager which can resolve names both in the /Channel and /Resources namespaces.
	// Note, the result of this method is almost definitely the one you want.  Calling ChannelConfig().PolicyManager()
	// will return a policy manager which can only resolve policies in the /Channel namespace.
	PolicyManager() policies.Manager

	// APIPolicyMapper returns a way to map API names to policies governing their invocation.
	APIPolicyMapper() PolicyMapper

	// ChaincodeRegistery returns a way to query for chaincodes defined in this channel.
	ChaincodeRegistry() ChaincodeRegistry

	// ChannelConfig returns the channelconfig.Resources which this config depends on.
	ChannelConfig() channelconfig.Resources
}
