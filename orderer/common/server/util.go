/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"encoding/hex"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/metrics"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/common/util"
	config "github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/throttle"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
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

type clock struct{}

func (c *clock) Now() time.Time {
	return time.Now()
}

func (c *clock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

func newRateLimiter(rate int, timeout time.Duration) *throttle.SharedRateLimiter {
	return &throttle.SharedRateLimiter{
		Limit:             rate,
		Time:              &clock{},
		InactivityTimeout: timeout,
	}
}

//go:generate counterfeiter -o mock/ratelimiter.go --fake-name RateLimiter . RateLimiter

// RateLimiter limits the rate of transactions processed
type RateLimiter interface {
	// LimitRate limits the processing rate of transactions from this client
	// across all invocations
	LimitRate(client string)
}

//go:generate counterfeiter -o mock/broadcastservice.go --fake-name BroadcastService . BroadcastService
type BroadcastService interface {
	orderer.AtomicBroadcastServer
}

type ThrottlingAtomicBroadcast struct {
	ThrottlingEnabled             bool
	PerClientRateLimiter          RateLimiter
	PerOrgRateLimiter             RateLimiter
	orderer.AtomicBroadcastServer // The backend
}

func (tab *ThrottlingAtomicBroadcast) Broadcast(stream orderer.AtomicBroadcast_BroadcastServer) error {
	cert := util.ExtractCertificateFromContext(stream.Context())

	throttlingDisabled := !tab.ThrottlingEnabled

	if throttlingDisabled || cert == nil {
		return tab.AtomicBroadcastServer.Broadcast(stream)
	}

	logger.Debugf("Throttling %s", cert.Subject.CommonName)

	aki := hex.EncodeToString(cert.AuthorityKeyId)
	ski := hex.EncodeToString(cert.SubjectKeyId)

	throttleStream := &throttleStream{
		perClientRateLimiter:            tab.PerClientRateLimiter,
		perOrgRateLimiter:               tab.PerOrgRateLimiter,
		AtomicBroadcast_BroadcastServer: stream,
		client:                          ski,
		org:                             aki,
	}
	return tab.AtomicBroadcastServer.Broadcast(throttleStream)
}

type throttleStream struct {
	orderer.AtomicBroadcast_BroadcastServer // The backend
	perClientRateLimiter                    RateLimiter
	perOrgRateLimiter                       RateLimiter
	client                                  string
	org                                     string
}

func (t *throttleStream) Recv() (*common.Envelope, error) {
	defer func() {
		t.perClientRateLimiter.LimitRate(t.client)
		t.perOrgRateLimiter.LimitRate(t.org)
	}()
	return t.AtomicBroadcast_BroadcastServer.Recv()
}
