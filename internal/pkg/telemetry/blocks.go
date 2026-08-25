/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/protoutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// EnvBlockTxLinks controls whether block commit spans are linked back to the
// traces of the transactions they carry.
//
// This is off by default, and that default is a deliberate cost decision rather
// than caution. Building the links means unmarshalling the header of every
// transaction in every block, on the commit path, on every peer. Validation
// already does that work, but it does not hand the result back here, so turning
// this on pays for it a second time. The block-level spans are what answer "why
// is commit slow"; the links answer the different question of "which client
// request ended up in this block", and only that second question costs extra.
const (
	EnvBlockTxLinks    = "FABRIC_TRACE_BLOCK_TX_LINKS"
	EnvBlockTxLinksMax = "FABRIC_TRACE_BLOCK_TX_LINKS_MAX"

	// defaultMaxBlockTxLinks matches the OpenTelemetry default limit on links
	// per span. Blocks can hold far more transactions than this, so links stop
	// being added past the limit rather than being silently dropped later by
	// the SDK.
	defaultMaxBlockTxLinks = 128
)

// Resolved once at startup rather than read per block, for the same reason as
// the chaincode switch: these answers cannot change while the node runs, and the
// commit path should not pay to look them up.
var (
	blockTxLinks    atomic.Bool
	blockTxLinksMax atomic.Int64
)

// resolveBlockTxLinks reads the switches from the environment. Called from
// Initialize, and from tests that need to change them.
func resolveBlockTxLinks() {
	enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(EnvBlockTxLinks)))
	blockTxLinks.Store(err == nil && enabled)

	blockTxLinksMax.Store(defaultMaxBlockTxLinks)
	if raw := strings.TrimSpace(os.Getenv(EnvBlockTxLinksMax)); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 0 {
			logger.Warnw("Invalid "+EnvBlockTxLinksMax+", using default",
				"value", raw, "default", defaultMaxBlockTxLinks)
		} else {
			blockTxLinksMax.Store(int64(limit))
		}
	}
}

// BlockTxLinksEnabled reports whether commit spans should be linked to the
// traces of the transactions in the block.
func BlockTxLinksEnabled() bool {
	return Enabled() && blockTxLinks.Load()
}

// maxBlockTxLinks reports how many links a single commit span may carry.
func maxBlockTxLinks() int {
	return int(blockTxLinksMax.Load())
}

// BlockTxLinks returns links from a block commit span to the traces of the
// transactions the block carries.
//
// A block is not part of any one transaction's trace: it aggregates
// transactions from many unrelated clients and is committed asynchronously on
// every peer. Links, rather than a parent, are the honest way to express that
// relationship, and they let a backend get from a client's request to the commit
// that finalized it.
//
// The trace context for a transaction comes from one of two places. If this peer
// endorsed it, the context was recorded at endorsement time. Otherwise it can
// only come from the transaction itself, which requires the client to have
// embedded it. Transactions that supply neither are skipped: their commit is
// still visible on the block span and still findable by fabric.tx_id.
func BlockTxLinks(block *cb.Block) []trace.Link {
	if block == nil || block.Data == nil {
		return nil
	}

	limit := maxBlockTxLinks()
	if limit == 0 {
		return nil
	}

	links := make([]trace.Link, 0, min(len(block.Data.Data), limit))
	for _, envelopeBytes := range block.Data.Data {
		if len(links) >= limit {
			logger.Debugw("Reached the per-block link limit, remaining transactions are not linked",
				"limit", limit, "block", block.Header.GetNumber())
			break
		}

		txID, sc := transactionTrace(envelopeBytes)
		if !sc.IsValid() {
			continue
		}
		links = append(links, trace.Link{
			SpanContext: sc,
			Attributes:  []attribute.KeyValue{AttrTxID.String(txID)},
		})
	}
	return links
}

// transactionTrace returns a transaction's id and the trace context it should be
// linked to, preferring what this peer recorded when it endorsed the
// transaction over what the client embedded in it.
func transactionTrace(envelopeBytes []byte) (string, trace.SpanContext) {
	envelope, err := protoutil.UnmarshalEnvelope(envelopeBytes)
	if err != nil {
		return "", trace.SpanContext{}
	}
	payload, err := protoutil.UnmarshalPayload(envelope.Payload)
	if err != nil || payload.Header == nil {
		return "", trace.SpanContext{}
	}
	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil || chdr.TxId == "" {
		return "", trace.SpanContext{}
	}

	if sc, ok := LookupEndorsement(chdr.TxId); ok {
		ForgetEndorsement(chdr.TxId)
		return chdr.TxId, sc
	}
	return chdr.TxId, TraceContextFromHeaderExtension(chdr.Extension)
}
