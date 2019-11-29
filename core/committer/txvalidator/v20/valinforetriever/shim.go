/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package valinforetriever

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/plugindispatcher"
	"github.com/hyperledger/fabric/core/ledger"
)

//go:generate mockery -dir . -name LifecycleResources -case underscore -output mocks/

// LifecycleResources is the local interface that used to generate mocks for foreign interface.
type LifecycleResources interface {
	plugindispatcher.LifecycleResources
}

// ValidationInfoRetrieveShim implements plugindispatcher.LifecycleResource
// by attempting to retrieve validation information from the two
// supplied sources - a legacy source and a new source. The ValidationInfo
// function will return info from the new source (if available) or
// info from the legacy source
type ValidationInfoRetrieveShim struct {
	Legacy plugindispatcher.LifecycleResources
	New    plugindispatcher.LifecycleResources
}

// ValidationInfo implements the function of the LifecycleResources interface.
// It returns the name and arguments of the validation plugin for the supplied
// chaincode. The function merely acts as a pass-through - returning validation
// info from the new or the legacy lifecycle, unless the legacy lifecycle successfully
// returns validation info requiring the built-in validation plugin and arguments
// that can be successfully parsed as a SignaturePolicyEnvelope and that can be
// successfully converted into ApplicationPolicy. In that case, and in that case
// only does this function modify the return values from the underlying lifecycle.
func (v *ValidationInfoRetrieveShim) ValidationInfo(channelID, chaincodeName string, qe ledger.SimpleQueryExecutor) (plugin string, args []byte, unexpectedErr error, validationErr error) {
	plugin, args, unexpectedErr, validationErr = v.New.ValidationInfo(channelID, chaincodeName, qe)
	if unexpectedErr != nil || validationErr != nil || plugin != "" || len(args) != 0 {
		// in case of any errors or any actual information
		// returned by the new lifecycle, we don't perform
		// any translation
		return
	}

	plugin, args, unexpectedErr, validationErr = v.Legacy.ValidationInfo(channelID, chaincodeName, qe)
	if unexpectedErr != nil || validationErr != nil || plugin != "vscc" || len(args) == 0 {
		// in case of any errors or in case the plugin is
		// not the default one ("vscc") or we have an empty
		// policy, we don't perform any translation
		return
	}

	spe := &common.SignaturePolicyEnvelope{}
	err := proto.Unmarshal(args, spe)
	if err != nil {
		// if we can't interpret the policy, we don't attempt
		// any translation
		return
	}

	newArgs, err := proto.Marshal(
		&peer.ApplicationPolicy{
			Type: &peer.ApplicationPolicy_SignaturePolicy{
				SignaturePolicy: spe,
			},
		},
	)
	if err != nil {
		// if we can't marshal the translated policy, we don't attempt
		// any translation
		return
	}

	// if marshalling is successful, we return the translated policy
	args = newArgs
	return
}
