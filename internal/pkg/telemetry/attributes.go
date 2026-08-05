/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import "go.opentelemetry.io/otel/attribute"

// Semantic conventions for Fabric spans.
//
// OpenTelemetry has no Fabric-specific conventions, so these are defined once
// here rather than being spelled out at each call site, which keeps a query like
// "slowest chaincodes on channel X" working across peer and orderer spans.
//
// Note the difference from the Prometheus metrics in common/metrics: a metric
// label must be low cardinality because it multiplies time series, but a span
// attribute is recorded per span and is exactly where high-cardinality
// identifiers belong. Transaction and block identifiers are therefore both
// present and useful here, while they are deliberately absent from the metrics.
const (
	// AttrChannelID is the channel a transaction or block belongs to.
	AttrChannelID = attribute.Key("fabric.channel_id")

	// AttrTxID is the Fabric transaction id. This is the join key between the
	// endorsement trace and the block commit spans it eventually lands in.
	AttrTxID = attribute.Key("fabric.tx_id")

	// AttrTxType is the common.HeaderType name, e.g. ENDORSER_TRANSACTION or
	// CONFIG.
	AttrTxType = attribute.Key("fabric.tx_type")

	// AttrChaincodeName identifies the invoked chaincode.
	AttrChaincodeName = attribute.Key("fabric.chaincode.name")

	// AttrChaincodeFunction is the function within the chaincode being invoked,
	// taken from the first argument by the convention every Fabric contract API
	// follows. It is what separates "this chaincode is slow" from "this one
	// function of it is slow".
	AttrChaincodeFunction = attribute.Key("fabric.chaincode.function")

	// AttrChaincodeArgsCount is how many arguments the invocation carried. The
	// arguments themselves are never recorded: they are business data, routinely
	// contain identifiers or personal information, and would end up in a
	// telemetry backend with weaker access controls than the ledger.
	AttrChaincodeArgsCount = attribute.Key("fabric.chaincode.args_count")

	// AttrShimRequest is the type of callback a chaincode made back into the
	// peer, such as GET_STATE or PUT_STATE.
	AttrShimRequest = attribute.Key("fabric.shim.request")

	// AttrMSPID is the MSP of the submitting or endorsing identity.
	AttrMSPID = attribute.Key("fabric.msp_id")

	// AttrBlockNumber is the ledger height of the block being processed.
	AttrBlockNumber = attribute.Key("fabric.block.number")

	// AttrBlockTxCount is how many transactions the block carries, which is the
	// main thing that explains a slow commit.
	AttrBlockTxCount = attribute.Key("fabric.block.tx_count")

	// AttrValidationCode is the peer.TxValidationCode name assigned during
	// validation. Anything other than VALID means the transaction was written to
	// the ledger but had no effect on world state.
	AttrValidationCode = attribute.Key("fabric.validation_code")

	// AttrResponseStatus is the status of an endorsement or broadcast response.
	AttrResponseStatus = attribute.Key("fabric.response.status")

	// AttrEndorsementFailed marks a proposal that was simulated but returned a
	// non-success status, so failed endorsements can be filtered out without
	// parsing messages.
	AttrEndorsementFailed = attribute.Key("fabric.endorsement.failed")

	// AttrPeerID and AttrOrdererID identify the process emitting the span. They
	// are set once as resource attributes at startup, not per span.
	AttrPeerID    = attribute.Key("fabric.peer.id")
	AttrOrdererID = attribute.Key("fabric.orderer.id")

	// AttrConsensusType is the ordering service consensus mechanism.
	AttrConsensusType = attribute.Key("fabric.consensus.type")
)

// Tracer names, used to scope spans to the subsystem that produced them.
const (
	TracerEndorser  = "github.com/hyperledger/fabric/core/endorser"
	TracerChaincode = "github.com/hyperledger/fabric/core/chaincode"
	TracerCommitter = "github.com/hyperledger/fabric/core/committer"
	TracerBroadcast = "github.com/hyperledger/fabric/orderer/common/broadcast"
	TracerConsensus = "github.com/hyperledger/fabric/orderer/consensus"
)
