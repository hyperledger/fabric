# OpenTelemetry traces and metrics

This fork adds OpenTelemetry to the peer and the orderer so that a slow
transaction can be attributed to a specific stage rather than inferred from
aggregate metrics.

Everything here is off unless an OTLP endpoint is configured. With no `OTEL_*`
variables set, no provider is installed, every instrumentation point falls
through to the OpenTelemetry no-op implementation, and the nodes behave exactly
as stock Fabric does.

## Why tracing, when Fabric already has metrics

The existing Prometheus metrics answer *how long endorsement takes on this peer*.
They cannot answer *where the 900ms went for this particular transaction*,
because that time is spread across a client, one or more endorsing peers, the
ordering service, and finally a commit on every peer in the channel. Tracing
records that one transaction's path; the metrics remain the right tool for rates
and percentiles.

## Configuration

Configuration is entirely through standard `OTEL_*` environment variables rather
than `core.yaml` or `orderer.yaml`, so the same deployment tooling used for
surrounding services applies unchanged.

| Variable | Effect |
| --- | --- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Enables telemetry. Nothing is exported until this is set. |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | Signal-specific override of the above. |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` (default) or `grpc`. |
| `OTEL_EXPORTER_OTLP_HEADERS` | Authentication headers for the collector. |
| `OTEL_SERVICE_NAME` | Defaults to `fabric-peer` or `fabric-orderer`. |
| `OTEL_TRACES_SAMPLER` / `_ARG` | Sampling strategy. Default `parentbased_always_on`. |
| `FABRIC_TRACE_BLOCK_TX_LINKS` | Links block commit spans to the transactions they carry. Off by default; see below. |
| `FABRIC_TRACE_CHAINCODE_SHIM` | Emits a span per chaincode callback into the peer. On by default; see below. |

**Set a sampler before running this under real load.** The default records every
trace, which for a peer under load means a span per gRPC call plus several per
transaction. In production set
`OTEL_TRACES_SAMPLER=parentbased_traceidratio` with a small
`OTEL_TRACES_SAMPLER_ARG` and let the client at the edge decide which
transactions are worth recording.

For metrics, additionally set `metrics.provider: otel` in `core.yaml` (or
`Metrics.Provider: otel` in `orderer.yaml`).

## What gets traced

Endorsement, on the peer:

| Span | What a slow one means |
| --- | --- |
| `Endorser.ProcessProposal` | Total endorsement time. |
| `Endorser.preProcess` | Signature and ACL checks. |
| `Endorser.GetTxSimulator` | Contention with block commit — the simulator takes a shared lock on the state database. |
| `Endorser.ExecuteChaincode` | The chaincode itself, including its state access. Usually the answer. |
| `Chaincode.GET_STATE`, `Chaincode.PUT_STATE`, … | One per callback the chaincode makes back into the peer. |
| `Endorser.GetTxSimulationResults` | Collecting the read-write set; scales with state touched. |
| `Endorser.DistributePrivateData` | Pushing private data to collection members over gossip. |
| `Endorser.EndorseWithPlugin` | Signing; scales with the BCCSP or HSM in use. |

Ordering, on the orderer:

| Span | What a slow one means |
| --- | --- |
| `Broadcast.ProcessMessage` | Total time to accept a transaction. |
| `Broadcast.ProcessNormalMsg` | Validation and config-sequence checks. |
| `Broadcast.WaitReady` | Consenter backpressure — the ordering service is the bottleneck. |
| `Broadcast.Order` | Enqueueing to the consenter. Does not cover consensus, which is asynchronous. |

Commit, on every peer:

| Span | What a slow one means |
| --- | --- |
| `Committer.StoreBlock` | Total time to finalize a block. |
| `Committer.Validate` | VSCC and endorsement policy evaluation for every transaction in the block. |
| `Committer.RetrievePvtdata` | Pulling private data from other peers. |
| `Committer.CommitLegacy` | Writing to the state database and block store. |

## Chaincode invocations and state access

The endorsement spans identify *which* chaincode and *which function* was
invoked — `fabric.chaincode.name` and `fabric.chaincode.function`, the latter
taken from the first argument by the convention every contract API follows.
Nothing in the protocol enforces that convention, so the value is validated
before being recorded and dropped if it does not look like a function name.
Only the first argument is ever read; the rest are business data and are
deliberately never recorded.

Underneath, each callback the chaincode makes back into the peer gets its own
span: `Chaincode.GET_STATE`, `Chaincode.PUT_STATE`, `Chaincode.GET_STATE_BY_RANGE`,
`Chaincode.INVOKE_CHAINCODE` and the rest. This is what separates "this chaincode
is slow" from "this chaincode issues four hundred state reads", which look
identical from the endorsement span alone.

Every callback is handled on its own goroutine, keyed only by transaction id, so
there is no ambient context to inherit. The invocation's span context is instead
carried on `TransactionParams` into the transaction context, which is what those
goroutines read. That also means no shared lock on the callback path. The same
value is propagated across chaincode-to-chaincode calls, so a callee's state
access stays in the caller's trace — as siblings of the invocation rather than
children of it, told apart by their `fabric.chaincode.name`.

These spans are on by default, because chaincode state access is usually the
answer. **The sampler is the volume control**: shim spans inherit the sampling
decision of the transaction that caused them, so a ratio-based sampler bounds
them exactly as it bounds everything else. Set
`FABRIC_TRACE_CHAINCODE_SHIM=false` for the case where a single sampled
transaction fans out to thousands of reads and the spans stop being readable.
The aggregate view survives either way, since Fabric already meters shim requests
by type, channel and chaincode.

## How trace context travels, and where it stops

This is the part worth understanding before reading a trace, because Fabric's
shape does not match the request/response model tracing assumes.

**Endorsement and ordering propagate natively.** Both are gRPC calls from a
client we control, so the W3C `traceparent` in the request metadata is picked up
by the gRPC stats handler and every span below it joins the client's trace. This
works with no changes to transactions.

**Commit does not, and cannot without help.** A block is cut asynchronously and
contains transactions from many unrelated clients, then commits independently on
every peer in the channel. There is no ambient context to continue and no single
parent a block could belong to. Block commit spans are therefore separate traces,
related to transactions through span *links* rather than parentage.

Links are resolved from two sources:

1. **What the peer remembers.** A peer that endorsed a transaction records the
   trace context against the transaction id and looks it up at commit. This is
   bounded in both time and size and is lossy by design — a peer that did not
   endorse a transaction never had the context.
2. **What the transaction carries.** For a link that resolves on *every* peer,
   the trace context has to travel inside the signed transaction. The peer side
   of this is implemented: `TraceContextFromHeaderExtension` reads a
   `traceparent` from the `ChaincodeHeaderExtension` as protobuf fields 1000 and
   1001. These are unknown fields to stock Fabric, which preserves and ignores
   them, and because the client signs the header after adding them the signature
   verifies normally.

   **The client side is not implemented here.** Emitting these fields requires a
   change to whichever SDK builds transactions.
   `MarshalTraceContextExtension` exists as the reference implementation for that
   change, and `TestTraceContextSurvivesProtoRoundTrip` demonstrates the
   round trip.

When neither source resolves, the commit is still traced and still carries
`fabric.tx_id`, so the two traces can be joined at query time.

### The cost of links

`FABRIC_TRACE_BLOCK_TX_LINKS` is off by default because building links means
unmarshalling the header of every transaction in every block, on the commit path,
on every peer. Validation already does that work but does not hand the result
back, so enabling this pays for it twice. The block-level spans answer "why is
commit slow" without it; links answer the different question of "which client
request ended up in this block". `FABRIC_TRACE_BLOCK_TX_LINKS_MAX` caps links per
block, defaulting to 128 to match the OpenTelemetry limit.

## What is deliberately not traced

Gossip, the Raft cluster service and gRPC health checks are excluded. Their
volume scales with cluster size rather than transaction load, and consensus
timing is better served by the existing Prometheus metrics.

Long-lived streams — `Deliver`, and `AtomicBroadcast` at the stream level — are
also excluded from automatic instrumentation. A span lasts as long as its RPC, so
wrapping a stream that stays open for hours would produce one span held in memory
that whole time and covering everything the stream ever carried. Ordering is
traced per message instead.

## Metrics

Setting `metrics.provider: otel` swaps Fabric's `metrics.Provider` for an
OpenTelemetry-backed implementation. Every metric Fabric already defines is then
exported over OTLP — there are no new metric definitions and no new
instrumentation call sites, which is what keeps the OTLP and Prometheus views
from drifting apart.

Two behaviours are worth knowing:

- Fabric's `Gauge` supports both `Set` and `Add`, but OTLP carries only absolute
  values, so `Add` is resolved against the previous reading per label set.
- The Prometheus provider is scraped, so unbounded labels cost memory here; the
  OTLP provider pushes, so they also cost ingest on the collector. Fabric's own
  labels are bounded, but anything added downstream should be checked.

## Implementation notes for rebasing

The instrumentation is confined to `internal/pkg/telemetry` plus a small number
of call sites, to keep conflicts manageable when rebasing onto upstream:

- `internal/peer/node/start.go` and `orderer/common/server/main.go` — startup and
  the gRPC stats handler
- `internal/pkg/comm/{config,server}.go` — a `StatsHandlers` field, since gRPC
  supports multiple stats handlers and Fabric only exposed one
- `core/endorser/endorser.go` — a `context.Context` threaded through
  `ProcessProposalSuccessfullyOrError`, `simulateProposal` and `callChaincode`
- `orderer/common/broadcast/broadcast.go` — a `context.Context` on `ProcessMessage`
- `gossip/privdata/coordinator.go` — commit spans
- `core/operations/system.go` — the `otel` metrics provider case

The endorsement registry is process-global rather than threaded through
constructors, deliberately: the alternative touches a long chain of upstream
constructors on both the endorsement and commit paths, and every one of those is
a rebase conflict for a value that is genuinely process-scoped.
