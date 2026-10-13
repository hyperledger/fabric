# BiniBFT orderer consensus plugin

A Hyperledger Fabric ordering-service consenter backed by the BiniBFT protocol.
It **replaces SmartBFT**: it registers under Fabric's `"BFT"` consensus type
(`orderer/common/server/main.go`), so channels use `OrdererType: BFT`,
`V3_0` capabilities and the usual `ConsenterMapping`, and Fabric peers verify its
blocks with the same BFT block-validation policy they apply to SmartBFT.

Only one cluster-based BFT consenter can be registered per orderer process
(both would register the same `ClusterNodeService`), so this build does not
contain SmartBFT.

## Layout

| Path | What |
|---|---|
| `consenter.go` | `consensus.Consenter`/`ClusterConsenter`: `HandleChain`, `IsChannelMember`, `ReceiverByChain`, cluster service registration |
| `chain.go` | `consensus.Chain` + the engine's `ApplicationDelivery`; block signing and the ledger write path |
| `assembler.go` | builds a full Fabric block per proposal; isolates config transactions |
| `crypto.go` | engine signer/verifier on the orderer's MSP identity; request and proposal validation |
| `network.go`, `ingress.go`, `wire.go` | engine messages over Fabric's authenticated cluster transport |
| `identity.go` | consenter set from channel config; quorum math |
| `engine/` | the BiniBFT engine, vendored from `Hyperledger-BiniBFT/consensus` by `sync-engine.sh` |
| `testclient/` | `broadcast` and `verify` tool (verify uses Fabric's own BFT block verifier) |
| `testnet/` | 4-orderer localhost network: `net.sh gen`, `up`, `stop N`, `start N`, `down` |

The engine is vendored (not a `replace` directive) because Fabric vendors its
dependencies; `sync-engine.sh` copies it and rewrites the import path.

## How it works

```
Order/Configure -> engine pool -> primary assembles a Fabric block (proposal)
 -> engine pre-prepare / prepare / commit -> Deliver(proposal) on every node
 -> each node signs OrdererBlockMetadata||IdentifierHeader||BlockHeader and
    broadcasts the signature -> a node writes the block once it holds a quorum
    ceil((n+f+1)/2) of consenter signatures (identical to the peer policy)
```

Topology: every consenter is its own single-node shard, so the engine's
cross-shard quorum is a flat BFT group over all consenters. The primary is the
lowest consenter ID.

## Test it

```
cd testnet
bash net.sh gen         # crypto material + genesis block (once)
bash net.sh up          # 4 orderers, joined to "mychannel"
go build -o ~/bin/binibft-testclient ../testclient   # then: binibft-testclient broadcast|verify (see testclient/main.go)
```

`go test ./orderer/consensus/binibft/` runs the unit tests.

## Verified (4 orderers, localhost)

* all 4 orderers join as active consenters; IDs, primary and quorum (3) as expected
* transactions via the primary and via a non-primary (forwarded) are ordered
* 315 transactions in -> 315 committed, no loss or duplication
* identical block hashes on all 4 orderers; every block carries 3 signatures and
  passes `deliverclient.NewBlockVerificationAssistant` (the peer's verifier)
* one non-primary orderer down (f=1): the other three keep ordering

## Known gaps (not done)

1. **No leader failover.** The engine does not implement view change
   (`MsgViewChange` is a stub), so if the primary fails ordering stops.
2. **No catch-up.** An orderer that was down restarts with its ledger but does
   not fetch the blocks it missed, and so stops taking part in consensus.
3. **No live membership change.** A config update that changes the consenter
   set needs the chain to be re-created (restart); the plugin does not do it.
4. **Batch limit not enforced.** The engine's batcher ignores
   `BatchSize.MaxMessageCount` (a block of 280 txs was produced with a limit of 20).
5. **Engine safety.** Followers accept the shard leader's finalized block
   without a quorum certificate; block-signature collection in this plugin
   compensates for what is written to the ledger, but the engine itself should
   verify certificates. VRF leader election exists in the engine but is not used.
6. Single-machine test only; no peer was joined to the network.
