/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"github.com/hyperledger/fabric/v2/common/ledger/blockledger"
	"github.com/hyperledger/fabric/v2/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/v2/common/metrics"
	config "github.com/hyperledger/fabric/v2/orderer/common/localconfig"
	"github.com/pkg/errors"
)

func createLedgerFactory(conf *config.TopLevel, metricsProvider metrics.Provider) (blockledger.Factory, error) {
	ld := conf.FileLedger.Location
	if ld == "" {
		logger.Panic("Orderer.FileLedger.Location must be set")
	}

	logger.Debug("Ledger dir:", ld)
	lf, err := fileledger.New(ld, metricsProvider)
	if err != nil {
		return nil, errors.WithMessage(err, "Error in opening ledger factory")
	}
	return lf, nil
}
