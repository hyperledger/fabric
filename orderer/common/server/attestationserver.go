/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"runtime/debug"
	"time"

	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/policies"
	localconfig "github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

type attestationserver struct {
	dh    *deliver.Handler
	debug *localconfig.Debug
	*multichannel.Registrar
}

// TODO This is preparation work for the BFT block puller. Right now it is used only in unit tests.
// We need to revisit this code and redesign this concept.
// We need to update the related unit tests.

// NewAttestationService creates an ab.AtomicBroadcastServer based on the broadcast target and ledger Reader
func NewAttestationService(
	r *multichannel.Registrar,
	metricsProvider metrics.Provider,
	debug *localconfig.Debug,
	timeWindow time.Duration,
	mutualTLS bool,
	expirationCheckDisabled bool,
) ab.BlockAttestationsServer {
	s := &attestationserver{
		dh:        deliver.NewHandler(deliverSupport{Registrar: r}, timeWindow, mutualTLS, deliver.NewMetrics(metricsProvider), expirationCheckDisabled),
		debug:     debug,
		Registrar: r,
	}
	return s
}

func (s *attestationserver) BlockAttestations(env *cb.Envelope, strm ab.BlockAttestations_BlockAttestationsServer) error {
	logger.Debugf("Starting new handler for block attestation")
	defer func() {
		if r := recover(); r != nil {
			logger.Criticalf("block attestation client triggered panic: %s\n%s", r, debug.Stack())
		}
		logger.Debugf("Closing attestation server stream")
	}()

	policyChecker := func(env *cb.Envelope, channelID string) error {
		chain := s.GetChain(channelID)
		if chain == nil {
			return errors.Errorf("channel %s not found", channelID)
		}
		// In maintenance mode, we typically require the signature of /Channel/Orderer/Readers.
		// This will block Deliver requests from peers (which normally satisfy /Channel/Readers).
		sf := msgprocessor.NewSigFilter(policies.ChannelReaders, policies.ChannelOrdererReaders, chain)
		return sf.Apply(env)
	}
	attestationServer := &deliver.Server{
		PolicyChecker: deliver.PolicyCheckerFunc(policyChecker),
		ResponseSender: &attestationSender{
			BlockAttestations_BlockAttestationsServer: strm,
		},
	}
	return s.dh.HandleAttestation(strm.Context(), attestationServer, env)
}

type attestationSender struct {
	ab.BlockAttestations_BlockAttestationsServer
}

func (rs *attestationSender) SendStatusResponse(status cb.Status) error {
	reply := &ab.BlockAttestationResponse{
		Type: &ab.BlockAttestationResponse_Status{Status: status},
	}
	return rs.Send(reply)
}

func (rs *attestationSender) SendBlockResponse(
	block *cb.Block,
	channelID string,
	chain deliver.Chain,
	signedData *protoutil.SignedData,
) error {
	blockAttestation := ab.BlockAttestation{Header: block.Header, Metadata: block.Metadata}
	response := &ab.BlockAttestationResponse{
		Type: &ab.BlockAttestationResponse_BlockAttestation{BlockAttestation: &blockAttestation},
	}
	return rs.Send(response)
}

func (rs *attestationSender) DataType() string {
	return "block_attestation"
}
