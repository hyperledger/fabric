/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package valinforetriever

import (
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

func (v *ValidationInfoRetrieveShim) ValidationInfo(channelID, chaincodeName string, qe ledger.SimpleQueryExecutor) (plugin string, args []byte, unexpectedErr error, validationErr error) {
	plugin, args, unexpectedErr, validationErr = v.New.ValidationInfo(channelID, chaincodeName, qe)
	if unexpectedErr != nil || validationErr != nil || plugin != "" || args != nil {
		return
	}

	return v.Legacy.ValidationInfo(channelID, chaincodeName, qe)
}
