# Isolated PQ3 Fabric Network

This is an isolated development demonstration, not a production or payment
network. It uses only the `rfc5612-test-only-v1` certificate profile and
classical TLS. It makes no claim of quantum-resistant transport, high
availability, or Byzantine fault tolerance.

Run the commands below from WSL/Linux on an access-controlled Linux filesystem.
The default runtime directory is under `$HOME/.local/share/pactway-pq3`; do not
redirect it to a Windows-mounted filesystem. Docker Desktop, Docker Compose v2,
GNU make, and Go 1.27.4 are required. The Docker network is internal and no
container ports are published to the host by default.

Set the development-only environment explicitly in the shell that runs the
network:

```bash
export PACTWAY_HYBRID_PROFILE=rfc5612-test-only-v1
export PACTWAY_ISOLATED_DEVELOPMENT=true
```

## Lifecycle

```bash
integration/pq3/pq3.sh fresh
integration/pq3/pq3.sh health
integration/pq3/pq3.sh join
integration/pq3/pq3.sh deploy
integration/pq3/pq3.sh invoke
integration/pq3/pq3.sh verify
integration/pq3/pq3.sh negative-endorsement
integration/pq3/pq3.sh recovery
```

`fresh` builds the actual peer and orderer images from the current Pactway
source tree, records the source commit/tree digest and image IDs, generates
independent hybrid keys and genesis material, and starts the three services
with Fabric's channel participation mode. It refuses to replace an active run.
The explicit `join` command uses `osnadmin` to join the orderer from the
generated application-channel block, then joins both peers.

```bash
integration/pq3/pq3.sh stop
integration/pq3/pq3.sh start
integration/pq3/pq3.sh restart
integration/pq3/pq3.sh resume
integration/pq3/pq3.sh status
```

These commands preserve the generated credentials, channel configuration, and
ledger volumes. `reset` is separate and destructive: it removes only the
active PQ3 Compose project, that project's named volumes, and that run's
generated material.

```bash
integration/pq3/pq3.sh reset
```

## Topology and custody

- `Org1MSP` and `Org2MSP` each operate one peer; `OrdererOrgMSP` operates one
  Raft orderer.
- Peer and orderer images are locally built from this repository; Compose uses
  `pull_policy: never` and a private local image namespace.
- Node services receive only their own local MSP and TLS material. The CLI
  service receives only the selected organization’s client/admin MSPs.
- Runtime private keys, ledgers, temporary-profile artifacts, and evidence
  stay under the Linux runtime directory and are ignored by Git and Docker build
  contexts. Do not copy or publish that directory.
- The Compose network is internal. If a future presentation requires host
  access, any port publication must be loopback-only and separately reviewed.

The isolated workflow runs a peer-level invalid-PQ-signature rejection check,
expiry/replay acceptance, and checks that the verified upstream Fabric v3.1.5
peer rejects a channel requiring the fork-only hybrid capability. It also
checks channel activation, chaincode lifecycle, two-organization endorsement,
cryptographic evidence, negative endorsement, and state-preserving recovery.
Starting three containers alone is not PQ3 completion. The upstream check
demonstrates rejection of this unsupported capability, not interoperability.

`deploy` installs the external CCAAS package for both organizations, approves
and commits sequence 1 with
`AND('Org1MSP.peer','Org2MSP.peer')`, and starts the chaincode server.
`invoke` submits an asset transaction to both peers, checks the committed value
at each peer, fetches the latest block, and runs `pq3-verify` against the
generated channel config. The verifier parses the actual block and transaction
bytes, checks the block data hash and orderer block signature, verifies client
and endorsement signatures using the channel MSPs, requires the authenticated
hybrid channel capability, and confirms both peer MSPs endorsed the requested
chaincode transaction. Orderer-delivered blocks do not carry the peer's
transaction validation filter, so the verifier reports
`transaction_status_available: false` when that metadata is absent; the dual
peer state queries separately confirm that the asset write committed.

`negative-endorsement` submits a write with only Org1's endorsement. The peer
reports `ENDORSEMENT_POLICY_FAILURE`; both peers' state remains unchanged, and
the evidence verifier rejects the resulting transaction bytes because the
Org2 endorsement is absent. `recovery` checks both peer memberships, the active
orderer channel, and the ledger state after stop/start, restart, and resume.
The `PQ3 Isolated Network` workflow repeats these checks on a source-built,
internal Docker network and does not upload generated credentials or runtime
evidence.

The verifier output and fetched block are stored under the run's private
Linux-side evidence directory. They are demonstration evidence, not an
independent production audit or a claim that the classical TLS transport is
post-quantum secure.
Elapsed whole-second measurements for successful fresh, join, deploy, invoke,
and recovery commands are appended to `evidence/timings.txt`; they are
environment-specific observations, not performance guarantees.

The demo has one Raft orderer and makes no availability or fault-tolerance
claim. It does not implement settlement tokens or PQ4; settlement integration
must remain a separate change and must not treat this demo's classical TLS as
post-quantum protection.
