/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/common/metrics"
	config "github.com/hyperledger/fabric/orderer/common/localconfig"
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

// validateBootstrapBlock returns whether this block can be used as a bootstrap block.
// A bootstrap block is a block of a system channel, and needs to have a ConsortiumsConfig.
func validateBootstrapBlock(block *common.Block, bccsp bccsp.BCCSP) error {
	if block == nil {
		return errors.New("nil block")
	}

	if block.Data == nil || len(block.Data.Data) == 0 {
		return errors.New("empty block data")
	}

	firstTransaction := &common.Envelope{}
	if err := proto.Unmarshal(block.Data.Data[0], firstTransaction); err != nil {
		return errors.Wrap(err, "failed extracting envelope from block")
	}

	bundle, err := channelconfig.NewBundleFromEnvelope(firstTransaction, bccsp)
	if err != nil {
		return err
	}

	_, exists := bundle.ConsortiumsConfig()
	if !exists {
		return errors.New("the block isn't a system channel block because it lacks ConsortiumsConfig")
	}
	return nil
}
