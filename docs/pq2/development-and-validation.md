# PQ2 Development and Validation

## Verified development baseline

The tested implementation head is
`f6ff21ca78bce024214f156714f057250cb56db5` on
`feature/pq2-hybrid-certificates`, based on audited main commit
`87b7b5855d47089830c03b05246fd4258492a8aa`.
[PR run 44068010218](https://github.com/vignatov/fabric-pq/actions/runs/44068010218)
and [push run 44068002211](https://github.com/vignatov/fabric-pq/actions/runs/44068002211)
both passed at that implementation head. CI logs confirm
`go version go1.27.4 linux/amd64`.

The PR run provides the following complete, passing workflow evidence:

| Gate | Exact command | Result |
| --- | --- | --- |
| Basic Checks | `go mod tidy`; `go mod vendor`; `make basic-checks` | Passed, including generated-module/vendor consistency checks. |
| SmartBFT live-vote dispatch | `go test -mod=mod -race github.com/hyperledger/SmartBFT/internal/bft` | Passed. |
| Fabric unit baseline | `make unit-test` | Passed. |
| Raft integration | `make integration-test INTEGRATION_TEST_SUITE="raft"` | Passed. |
| Private-data integration | `make integration-test INTEGRATION_TEST_SUITE="pvtdata"` | Passed. |
| Purge and end-to-end integration | `make integration-test INTEGRATION_TEST_SUITE="pvtdatapurge e2e"` | Passed. |
| Ledger integration | `make integration-test INTEGRATION_TEST_SUITE="ledger"` | Passed. |
| Lifecycle integration | `make integration-test INTEGRATION_TEST_SUITE="lifecycle"` | Passed. |
| SmartBFT integration | `make integration-test INTEGRATION_TEST_SUITE="smartbft"` | Passed. |
| Discovery and gossip integration | `make integration-test INTEGRATION_TEST_SUITE="discovery gossip devmode pluggable"` | Passed. |
| Gateway and configuration integration | `make integration-test INTEGRATION_TEST_SUITE="gateway idemix pkcs11 configtx configtxlator"` | Passed. |
| State-based endorsement, network and MSP integration | `make integration-test INTEGRATION_TEST_SUITE="sbe nwo msp"` | Passed; the MSP suite ran all six specs, including hybrid rotation and client/endorser expiry replay. |

These results supersede the earlier local baseline/integration failures below
for this implementation head; the failed attempts remain historical evidence,
not passing runs. Passing CI does not approve the production profile or
establish independent trusted time, and is not a human security sign-off.

## Temporary-profile restriction

The only currently registered profile is `rfc5612-test-only-v1`. Its
`1.3.6.1.4.1.32473.1.1` and `.1.2` identifiers are temporary development/test
fixtures. They are not Thinkstratum Ltd identifiers and must not be used in a
released or externally deployed network. Production OIDs, final ASN.1, and
production golden DER vectors remain pending the official PEN and profile
approval.

The release workflow runs `ci/scripts/check_release_hybrid_profile.sh` before
building binaries or container images. It fails while any development-only
profile is registered. Do not bypass this guard or manually publish artifacts
built with temporary identifiers.

## Generate a fresh isolated demo

Run from the repository root in WSL/bash. Choose a new output directory for
each run; the generator refuses to overwrite an existing one. Use Go 1.27.4
and an access-controlled Linux filesystem for private material, rather than
assuming POSIX permission bits protect a Windows-mounted directory.

```bash
PACTWAY_HYBRID_PROFILE=rfc5612-test-only-v1 \
PACTWAY_ISOLATED_DEVELOPMENT=true go run ./cmd/pactway-pq-gen \
  -output=pq2-demo \
  -configtx-dir=sampleconfig \
  -fabric-profile=SampleAppChannelEtcdRaft \
  -channel-id=pactway-pq2-demo \
  -hybrid-profile=rfc5612-test-only-v1
```

The generator creates a fresh root and intermediate CA, administrator and
client credentials for each organization, a peer identity for Org1 and Org2,
an orderer identity for OrdererOrg, classical TLS material, and a genesis
block with the Pactway channel capability. `public/` contains certificates,
MSP trust material, the genesis block and a manifest; `private/` contains
signing keys and local MSPs. Protect `private/` as secret material and do not
commit or reuse it. Random secret bytes intentionally differ across runs; the
command, layout, profile, and verification procedure are reproducible.

Before using the output:

- Check `public/manifest.json`: `development_only` must be `true`, the selected
  profile must be `rfc5612-test-only-v1`, and the channel capability must be
  `PactwayHybridCertificateV1`. Compare `genesis_sha256` with the generated
  `public/genesis.block` before distributing public channel material.
- Distribute only `public/` for remote MSP verification. Provision each node
  with only its own local MSP and TLS keys; do not copy all organizations'
  private material to every node. Issuer keystores belong with their respective
  organization, not in peer/orderer distribution bundles.
- On Linux the generator requests mode `0700` for the private root and
  keystore directories and `0600` for secret key files; public certificates
  and configuration within local MSPs can have less restrictive modes.
  Verify effective permissions and access controls.
  Protect backups and keep all output outside source control. Generation
  failure can leave partial output: treat it as secret material, discard that
  specific failed output directory and choose a new destination.
- The default orderer hostname is
  `orderer0.ordererorg.pactway.local`, port `7050`. Select `-orderer-host` and
  `-orderer-port` for the isolated network before generation; the host is bound
  into the classical TLS certificate and genesis consenter endpoint.
- Stop the associated nodes before retiring an output directory. Remove only
  that explicitly identified demo's credentials and ledgers according to local
  key-disposal policy. Never reuse its keys for another genesis or assume file
  deletion securely erases backups or SSD contents.

TLS and transport remain classical. This demo is not complete
quantum-resistant network security.

Ordinary peer, orderer and client processes using these temporary-profile
identities must explicitly select the same profile and opt into isolated use:

```bash
export PACTWAY_HYBRID_PROFILE=rfc5612-test-only-v1
export PACTWAY_ISOLATED_DEVELOPMENT=true
```

Set these only in the isolated demo's process environment. They select and
permit the development profile; they do not replace authenticated channel
policy or the generated local `PQ_REQUIRED` sidecar. A test-binary exception
does not authorize ordinary binaries to omit these runtime guards.

## Security assumptions and remaining release limits

- Orderer and peer local-signer compatibility checks now run after the config
  block has been committed. A node whose old signer no longer chains to the
  committed MSP stops after persistence; a replacement signer can then restart
  against the new config. A focused Docker-backed integration test now
  demonstrates a single-orderer root rotation: the old orderer stops after
  persistence, a replacement signer restarts it, peers restart and replay the
  committed config, and a subsequent config update succeeds. A further test
  also verifies that restarting with the stale signer fails before recovery
  with the replacement signer. Replacement signer material must be prepared
  before the update. A three-orderer Raft test now covers an overlapping-root
  rollout with individually generated signers, rolling restarts, removal of
  old roots, and rejection and recovery of a stale-signer restart. Broader
  failure-injection coverage remains open.
- Hybrid identities are validated against the signed channel-header timestamp
  for transaction creators and endorsement signers, so validation is
  deterministic during block validation and replay. Direct hybrid identity
  validation uses the current time, and time-independent MSP identity and
  principal caches are bypassed for hybrid identities. Classical-only
  identities in classical-permitted configurations retain Fabric's existing
  certificate-time behavior. Focused tests cover expiry at the supplied
  timestamp and propagation of that timestamp into state-based endorsement
  checks. Separate end-to-end tests now cover transactions signed by an
  expiring hybrid client or endorsed by an expiring hybrid peer: an offline
  peer catches up after the respective certificate expires and applies the
  valid transaction. An earlier replay attempt using Docker chaincode failed
  because the WSL host-network path refused the chaincode callback; it is not
  passing evidence. Broader failure-injection coverage remains open.
- Security review identified that the signed channel-header timestamp is
  chosen by the submitter and is not proof of endorsement time. An attacker
  with sufficient expired but still trusted endorser keys and a currently
  authorized writer identity can backdate a transaction and forge endorsements
  that pass the historical-time checks. PQ-required channels now validate
  every endorsement identity against the receiving orderer's current time at
  admission; malformed or expired identities are rejected. SmartBFT also
  applies admission validation to requests and proposals, including requests
  submitted through the cluster path. Raft relies on trusted, non-Byzantine
  orderers; a compromised orderer can bypass normal ingress validation.
  Admission is not proof of endorsement or commit time: peers use the signed
  transaction timestamp for deterministic transaction validation and use the
  orderer-signed block timestamp for block-signature replay. The latter is an
  authenticated orderer clock assertion, not an independent time authority.
  Transactions admitted before expiry can commit afterwards. Differing orderer
  clocks or expiry during SmartBFT consensus may cause inconsistent admission or liveness
  failures; these scenarios have not been tested with a live cluster. An
  approved, enforceable time design and independent external security review
  remain necessary before claiming production expiry assurance. The
  [ADR time model](adr-hybrid-identities.md#time-and-authorization-trust-model)
  records the narrower isolated-development assumptions.

For PQ-required channels, peer and orderer signer checks now additionally
require membership in the respective channel organization group and the
corresponding MSP peer or orderer principal. This requires NodeOU role
classification; a valid client identity is not a node signer. Every PQ-required
channel MSP must enable NodeOUs, define non-empty peer and orderer roles, and
use distinct identifiers for configured NodeOU roles. Missing or ambiguous
roles are rejected while constructing the config bundle. The hybrid channel
capability also requires channel capability `V1_4_3`, `V2_0`, or `V3_0`, which
enables the MSP's orderer principal.

## PQ2 validation record

The targeted implementation packages have passed together on the feature
branch using Go 1.27.4:

```text
go test -count=1 ./pq-crypto ./msp ./common/capabilities ./common/channelconfig ./cmd/pactway-pq-gen
```

The standalone certificate-feasibility tests also passed:

```text
go test -count=1 -run 'Test(FullTBSCertificatePQFeasibility|FeasibilityDERLengthBoundaries)$' ./pq-crypto
```

They construct and byte-compare the full unsigned TBSCertificate, validate the
classical and ML-DSA signatures, and test covered-field mutations and DER
length boundaries. Additional profile tests cover root/intermediate/leaf
chains with distinct issuer keys, autogenerated serial numbers, configured
non-self-issued trust anchors, missing or invalid PQ signatures, altered
issuer keys, incorrect anchors, classical-only and partial chains, duplicate
extensions, unsupported algorithms, malformed ASN.1, and non-canonical or
trailing DER. This is a feasibility prototype, not proof of the unsupported
binary capability gate.

The generator's end-to-end test additionally validates the authenticated
channel capability by loading the generated genesis envelope through Fabric's
`channelconfig.NewBundleFromEnvelope`; all three generated organization MSPs
must configure successfully under that required capability. It creates nine
generated local MSPs (client,
administrator, peer, and orderer roles across three organizations), then
performs BCCSP-backed hybrid signing and verification. It checks generated
certificate paths, that Org2 rejects an Org1 certificate even when it claims
Org2MSP, that Org2 accepts its own peer certificate, and that private key files
are absent from `public/`. A public-only Org2 verifier also verifies the peer's
signature and checks positive and negative client/admin/peer principals; an
OrdererOrg verifier checks the orderer principal.

The focused orderer root-rotation/restart/replay integration test passed:

```text
PATH=/usr/local/go/bin:/usr/bin:/bin go test ./integration/msp -run '^TestMSP$' -ginkgo.focus='Pactway hybrid orderer root rotation' -timeout 10m
```

Result: `ok github.com/hyperledger/fabric/integration/msp 276.290s`. This
covers one orderer and two peers, including a post-rotation config update and
peer restart/replay; it does not cover multi-orderer rotation or failure
injection. A subsequent full MSP integration run did not pass:

```text
PATH=/usr/local/go/bin:/usr/bin:/bin go test ./integration/msp -timeout 20m
```

Result: 1 spec passed and 2 failed. The mutual-TLS test's chaincode containers
timed out connecting to the WSL-host callback (`10.255.255.254:25006` and
`:25011`); the RSA-CA test timed out waiting for an installed chaincode and
received an empty package list. Both failures also reproduced when run in
isolation. These were not passing integration evidence. The later successful Linux CI
baseline above closes the broader MSP-suite execution gap without treating
these local callback/chaincode failures as successes.

A later run from the Windows-mounted checkout timed out while packaging the
mutual-TLS test's chaincode. Its RSA-CA test failed installation with
`Unknown chaincodeType: BINARY`; the binary external-builder scripts in that
checkout had CRLF line endings. A fresh Git checkout on the WSL Linux
filesystem with LF executable scripts and the same three uncommitted changes
ran:

```text
PATH=/usr/local/go/bin:/usr/bin:/bin go test ./integration/msp -count=1 -timeout 20m
```

Two specs passed, including the RSA-CA and root-rotation tests, and the
mutual-TLS spec failed because its chaincode containers timed out connecting
to `10.255.255.254:25006` and `:25011`.
Advertising `host.docker.internal` in that disposable checkout did not make
the focused mutual-TLS test pass. These results do not close the full-suite
or expired-credential replay gates.

The root-rotation test was extended to inject a restart with the stale signer
before installing the replacement. The following focused run passed with
`ok github.com/hyperledger/fabric/integration/msp 218.587s`:

```text
PATH=/usr/local/go/bin:/usr/bin:/bin go test ./integration/msp -count=1 -run='^TestMSP$' -ginkgo.focus='Pactway hybrid orderer root rotation' -timeout=10m
```

The same focused command passed again after adding orderer admission filtering
(`ok github.com/hyperledger/fabric/integration/msp 295.416s`).

The three-orderer Raft rotation test passed:

```text
PATH=/usr/local/go/bin:/usr/bin:/bin go test ./integration/msp -count=1 -run '^TestMSP$' -ginkgo.focus='rotates a three-orderer cluster with overlapping roots' -ginkgo.v -timeout=18m -v
```

Result: one spec passed, `ok github.com/hyperledger/fabric/integration/msp
303.357s`; a later rerun with the final fixture assertions passed in
289.874s. The test generates distinct old and replacement identities for
each orderer, adds replacement roots before rolling restarts, removes old
roots, and verifies stale-signer rejection, replacement-signer recovery and
the recovered orderer's config replay. The test MSP trusts multiple issuers
and uses NodeOUs without issuer-specific certificate pins. This does not
demonstrate abrupt root removal, loss of quorum, SmartBFT rotation, or
expired-credential transaction replay.

Both the single-orderer and three-orderer scenarios subsequently passed
together (two specs, 321.656s):

```text
PATH=/usr/local/go/bin:/usr/bin:/bin go test ./integration/msp -count=1 -run '^TestMSP$' -ginkgo.focus='Pactway hybrid orderer root rotation' -ginkgo.v -timeout=20m -v
```

The expired-client replay test passed using the binary external chaincode
builder from a clean Linux filesystem checkout of commit `abe602c`, with the
new test file copied in and its SHA-256 verified against the worktree
(`8bed2bc3af89258557c770d6d826c25e27c57fc3336974851e3837ef9cd9a3c7`):

```text
PATH=/usr/local/go/bin:/usr/bin:/bin go test ./integration/msp -count=1 -run=TestMSP -ginkgo.focus='replays a valid transaction after its client certificate expires' -ginkgo.v -timeout=20m -v
```

Result: one spec passed, `ok github.com/hyperledger/fabric/integration/msp
147.122s`. The test generates a fresh short-lived hybrid client identity,
confirms a valid committed transaction while the identity is unexpired,
keeps the replay peer offline until after expiry, and verifies that the peer
catches up and applies the transaction's exact state change. It does not
prove trusted endorsement time.

All three hybrid integration scenarios passed together in the same Linux
checkout (three specs, 192.513s):

```text
PATH=/usr/local/go/bin:/usr/bin:/bin go test ./integration/msp -count=1 -run=TestMSP -ginkgo.focus='Pactway hybrid' -ginkgo.v -timeout=20m -v
```

The replay test was extended to cover an expiring endorser as well as an
expiring client. It checks that the committed transaction actually contains
the generated short-lived certificate as the relevant signer. Both cases
passed in the same Linux checkout, with the updated test file copied in and
SHA-256 verified against the worktree
(`4ebd37ad69fa16fe9d13f50cee5f302fe7f11ed18072a1cae04b817da8216482`):

```text
PATH=/usr/local/go/bin:/usr/bin:/bin go test ./integration/msp -count=1 -run=TestMSP -ginkgo.focus='Pactway hybrid transaction expiry replay' -ginkgo.v -timeout=20m -v
```

Result: two specs passed, `ok github.com/hyperledger/fabric/integration/msp
280.845s`. Replay uses the signed transaction timestamp; this does not prove
when the endorsement was made or close the trusted-time security blocker.

The orderer admission tests generate fresh hybrid certificates and use a real
BCCSP-backed MSP. They verify that an expired endorser whose backdated
timestamp passes historical identity validation is rejected at admission, as
are malformed identities and mixed valid/expired endorsements. SmartBFT tests
exercise rejection in both request and proposal verification and fail closed
when the admission callback is absent. The following checks passed:

```text
PATH=/usr/local/go/bin:/usr/bin:/bin go test -count=1 ./orderer/common/msgprocessor ./orderer/consensus/smartbft
PATH=/usr/local/go/bin:/home/cogdev/go/bin:/usr/bin:/bin golangci-lint run ./orderer/common/msgprocessor/... ./orderer/consensus/smartbft/... ./integration/msp/...
```

An automated follow-up security review found no additional reportable
vulnerabilities in this mitigation; it is not the independent external review
required before production acceptance. These package tests are not full
transaction-replay or multi-orderer integration evidence.

The PR's Raft integration job failed while onboarding a fourth orderer to
`testchannel2`: its broadcast returned `SERVICE_UNAVAILABLE` because that
orderer had not yet learned the Raft leader. The orderer learned the leader
immediately afterwards; the corresponding push workflow's Raft job passed
on the same commit. The test now waits for the channel-specific leader
notification before broadcasting. This focused Docker-backed run passed:

```text
PATH=/usr/local/go/bin:/usr/bin:/bin go test ./integration/raft -count=1 -run '^TestRaft$' -ginkgo.focus='is still possible to onboard new orderers' -timeout=15m
```

Result: `ok github.com/hyperledger/fabric/integration/raft 285.556s`. This
is a classical Raft TLS-certificate onboarding test, not evidence for
hybrid-MSP rotation.

Additional focused tests verify that a PQ-required channel MSP handler rejects
a classical-only MSP, that a required MSP cannot be weakened through the local
policy API, that a new independently rooted hybrid MSP accepts its new identity
but rejects the old identity after root rotation. The generator test also
builds new bundles from the generated hybrid genesis config, proving an
application capability update that retains the PQ channel capability is
accepted and an update removing it is rejected.

The standalone nested BCCSP command `go test -mod=readonly ./bccsp/sw` passed
from `third_party/fabric-lib-go` in a temporary Go workspace joining the root
and nested modules. The release guard was exercised and correctly rejected the
currently registered development-only profile.

Docker Desktop integration is now enabled for WSL Ubuntu-24.04; Docker Server
29.9.3 is reachable. The Fabric suite required CouchDB image `couchdb:3.4.2`,
which was pulled successfully (digest
`sha256:2817ad50b5c5cf7461f5963670466197426605a790a19a6208068c509a2d8a59`).

An earlier documented unit-suite run passed from a clean ext4 snapshot in WSL
Ubuntu-24.04:

```text
PATH=/usr/local/go/bin:/usr/bin:/bin make unit-test
```

The earlier run from the Windows-mounted worktree was invalidated by line-ending
and executable-mode differences in its temporary copy. One clean ext4 unit run
then failed two timing-sensitive three-node Raft cases in the untouched
`orderer/consensus/etcdraft` package; an immediate standalone rerun of that
package passed. To reduce package scheduling pressure, the complete unit suite
was rerun from a Git-backed ext4 clone with `GOFLAGS=-p=1`, retaining the
Makefile's race and coverage options. It passed, including the PQ2-touched
packages and the PKCS11-tagged peer package:

```text
cd /tmp/pactway-pq2-unit-clean && PATH=/usr/local/go/bin:/usr/bin:/bin GOFLAGS=-p=1 make unit-test
```

Result: exit code 0; `make unit-test` reported `real 36m13.060s`.

The full command `/usr/local/go/bin/go test -count=1 ./...` was also started
after fetching CouchDB. `integration/configtx` passed; multiple other
integration packages hit Go's 10-minute test timeout during the run, so the
run was stopped and is not reported as passing. Temporary Fabric test
containers were removed. That stopped run remains incomplete; the successful
CI baseline above separately establishes the complete supported integration
matrix at the recorded implementation head.

The unsupported-binary gate passed using actual unmodified official v3.1.5
Linux/amd64 binaries. Their hashes were verified:

```text
peer   46a760292a0622bebd8e593b391485f1aa281cb505b2488968ecc4d9e1d5cf89
orderer 276b60a703e6187dfcdde7a0d16751a1986ca8ee12e9dd8065b307438cff8f69
```

Both binaries reported v3.1.5, commit `86c1172`. A fresh generated genesis
block containing `PactwayHybridCertificateV1` had SHA-256
`bb3836b4c11d86c5c5cb29803c2287e935b646c71bc92d38fbc9c5e45233c515`.
Orderer was started with channel participation enabled, generated TLS and MSP
material, and no system channel. The upstream `osnadmin channel join` returned
HTTP 400:

```text
cannot join: failed creating ledger resources: error checking bundle for channel:
pactway-unsupported-gate: config requires unsupported channel capabilities:
Channel capability PactwayHybridCertificateV1 is required but not supported
```

The `osnadmin` process itself returned exit code 0 despite the HTTP 400, so the
server response and log were used as the rejection evidence. The upstream peer
started with fresh local MSP and writable temporary ledger paths. Its
`peer channel join -b <genesis.block>` returned exit code 1:

```text
JoinChain ... failed because of validation of configuration block,
because of Failed capabilities check:
[Channel capability PactwayHybridCertificateV1 is required but not supported]
```

Earlier failed startup attempts were corrected using temporary copies of
`orderer.yaml` and `core.yaml`; they are not counted as gate results. Temporary
binaries, identities, nodes and logs were removed after capturing the block
hash and responses.

## Review findings addressed

- MSP chain validation rejects legacy alternate-key extensions before choosing
  either the optional legacy validator or the versioned profile validator.
  Tests cover both policy modes, including a classical root without
  alternate-key material.
- Hybrid chains in either policy mode require a self-issued trust anchor and
  verify both its classical and PQ self-signatures. All-classical chains
  retain ordinary upstream trust-anchor behavior in classical-permitted mode.
- Bundle construction and config-update validation do not depend on the
  process-local signer. On channel startup and activation of a consensus-ordered
  PQ-required configuration, the peer and orderer validate their local signer
  against the channel MSP and verify a hybrid signing challenge. An incompatible
  node stops rather than activating the configuration or signing classically.
- Any hybrid local signer must have an available ML-DSA-65 private key and
  pass classical and PQ key-possession checks, even when its MSP is otherwise
  classical-permitted.
- Embedded ML-DSA-65 SubjectPublicKeyInfo parsing now rejects extra sequence
  fields and non-canonical DER.
- The identity generator emits IP SANs for IP-valued orderer hosts, DNS SANs
  for hostnames, and correctly brackets IPv6 orderer endpoints.
- Live envelope authorization continues to validate at current time;
  transaction-time validation is explicit for replay-sensitive paths. Config
  updates must pass both current-time and signed-timestamp authorization before
  replay validation uses the timestamp.
- Hybrid endorsement admission deduplicates decoded MSP ID/certificate-DER
  pairs before MSP deserialization and validation. This corrects the earlier
  claim that deduplication by the constructed identity identifier prevented
  repeated PQ chain verification: ECDSA certificate sanitization during
  `newIdentity` calls `getUniqueValidationChain`, which verifies the hybrid
  chain. Protobuf field ordering, unknown fields and PEM wrapper differences
  now reach that expensive path only once per pair per envelope. A real hybrid
  MSP regression counts deserialization calls, rather than only mocked
  `Validate` calls.
- Raft and SmartBFT block signatures carry a versioned signing timestamp inside
  their signed nonce fields. Block replay validates hybrid orderer identities
  at that timestamp; an unversioned nonce supplies zero time and cannot silently
  fall back to current-time validation for a hybrid identity. SmartBFT rejects
  malformed timestamp nonces during consensus verification, before accepting a
  signature that downstream block verifiers would reject. Local hybrid signers
  are validated before signing even when their MSP permits classical identities.
- SmartBFT live commit votes additionally require current-time authorization;
  the signer's timestamp cannot authorize an expired consenter to cast a new
  vote. Historical previous-commit and last-decision verification retain
  signed-time validation. Live votes must pass both checks so an out-of-validity
  timestamp cannot produce a committed signature rejected during replay.
  The SmartBFT verifier interface and live-vote call site carry this explicit
  distinction. The root module replaces SmartBFT v1.0.3 with the used-source
  snapshot in `third_party/SmartBFT`, retaining its upstream license and module
  metadata plus the live-vote patch and regression test. `go mod vendor`
  regenerates the same patched dependency; module-mode builds also use it.
  Preserve the patch when refreshing that snapshot. CI explicitly tests the
  local module's live-vote dispatch because vendoring omits dependency tests.
  This does not turn a signed timestamp into independent trusted-time evidence.
- The gateway timeout-leak regression waits for earlier mock Evaluate workers
  to exit before measuring its own worker. Comparing against a nonzero worker
  count was flaky when an earlier timeout worker exited during the assertion;
  the test now requires zero workers before and after the call.

The focused validation commands passed:

```text
go test -count=1 -p=1 ./pq-crypto ./msp/... ./common/channelconfig ./orderer/common/multichannel ./orderer/common/server ./internal/peer/common ./internal/peer/node ./cmd/pactway-pq-gen
go vet -p=1 ./pq-crypto ./msp/... ./common/channelconfig ./orderer/common/multichannel ./orderer/common/server ./internal/peer/common ./internal/peer/node ./cmd/pactway-pq-gen
```

For the subsequent activation, channel-MSP trust, anchor, and IPv6 fixes, the
following affected-package checks passed on Go 1.27.4:

```text
go test -count=1 -p=1 ./pq-crypto ./msp/... ./common/capabilities ./common/channelconfig ./core/peer ./orderer/common/multichannel ./orderer/common/server ./internal/peer/common ./internal/peer/node ./cmd/pactway-pq-gen
go vet -p=1 ./pq-crypto ./msp/... ./common/capabilities ./common/channelconfig ./core/peer ./orderer/common/multichannel ./orderer/common/server ./internal/peer/common ./internal/peer/node ./cmd/pactway-pq-gen
```

For the live-vote, admission-deduplication and gateway timeout-test corrections,
the following checks passed on WSL Linux/amd64 with Go 1.27.4:

```text
go test -count=1 ./orderer/consensus/smartbft ./orderer/common/msgprocessor ./internal/pkg/gateway ./msp ./protoutil
go test -mod=mod -race -count=10 -run 'Test(Evaluate.*|VerifyConsenterSig|VerifyLiveConsenterSig|LiveVoteUsesCurrentAuthorization|HybridEndorsementAdmission.*)$' ./internal/pkg/gateway ./orderer/consensus/smartbft ./orderer/common/msgprocessor github.com/hyperledger/SmartBFT/internal/bft
go test -mod=mod -race github.com/hyperledger/SmartBFT/internal/bft
go vet ./orderer/consensus/smartbft ./orderer/common/msgprocessor ./internal/pkg/gateway
go mod tidy
go mod vendor
git diff --check
```

The module and vendor commands left the hashes of `go.mod`, `go.sum` and every
vendored file unchanged; vendor regeneration is reproducible with the local
SmartBFT replacement. The diff check used Windows-native Git. Targeted
`gofmt -l` reported no formatting changes. The full Fabric unit and integration
suites were not rerun locally for these corrections. At the preceding PR head
`bfbf8e8`, [PR CI run 44002860818](https://github.com/vignatov/fabric-pq/actions/runs/44002860818)
passed all integration jobs but failed the gateway timeout-leak unit assertion;
that run is not passing evidence for the new changes.

The final development closeout adds only regression tests and documentation;
it does not change the production implementation tested at `f6ff21c`.
The following checks passed with Go 1.27.4 on WSL Linux/amd64:

```text
go build ./cmd/peer ./cmd/orderer ./cmd/pactway-pq-gen
go test -mod=mod -count=1 ./cmd/pactway-pq-gen ./pq-crypto ./msp ./common/channelconfig ./common/configtx ./common/capabilities github.com/hyperledger/fabric-lib-go/bccsp/sw
go test -mod=mod -count=1 ./orderer/consensus/smartbft github.com/hyperledger/SmartBFT/internal/bft
go test -mod=mod -race -count=20 -run 'Test(VerifyLiveConsenterSig.*|VerifyConsenterSig|LiveVoteUsesCurrentAuthorization)$' ./orderer/consensus/smartbft github.com/hyperledger/SmartBFT/internal/bft
go vet ./orderer/consensus/smartbft
git diff --check
```

The multi-package build is a compile check, not a release-artifact publication.
Module mode explicitly executes the local replacement modules' tests,
including the nested BCCSP suite omitted by normal root-module enumeration.
`bash ci/scripts/check_release_hybrid_profile.sh` also returned the expected
exit code `1` and `release blocked: hybrid profile registry contains a
development-only profile`; that refusal is a successful negative gate, not a
release-build pass. CI must remain green on the final closeout commit before
merge; the full matrix result above is tied to its exact implementation head.

## Development closeout and release gates

The development acceptance evidence now covers both PQ2.0 feasibility gates,
strict certificate/issuer-chain and fail-closed MSP policy tests, real BCCSP
signing, fresh three-organization generation, and the complete supported Fabric
unit and nine-job integration baseline. Hybrid integration covers authorized
updates, single-orderer and planned three-orderer Raft root rotation,
stale-signer rejection/recovery, peer/orderer config replay, and replay after
client or endorser expiry. These completed gates must not be relisted as
unexecuted merely because earlier local attempts failed.

An automated closeout security review of the profile, MSP enforcement and
live/historical authorization paths found no confirmed exploitable defect.
Its remaining focused coverage gap is addressed by
`TestVerifyLiveConsenterSigWithHybridMSP`: real ML-DSA/BCCSP signatures and an
orderer-principal policy check signed times immediately before, exactly at,
and immediately after `NotAfter`, for both currently valid and expired
consenters. Historical signatures remain verifiable through the boundary;
live votes from expired consenters are rejected. The separate SmartBFT
dispatch regression proves that a rejected live vote is not counted. This
composed coverage is not a multi-node clock-skew/liveness test or independent
external cryptographic review.

The recommendation is final maintainer review for a **development-only merge
for isolated use**, preserving the guards and limitations above. This report
is not the maintainer's sign-off; no merge or PQ3 authorization is implied. The
following gates remain open before production acceptance or external use:

- Obtain the official Thinkstratum Ltd PEN, approve the permanent ASN.1/profile
  and OIDs, regenerate all certificates/MSPs/genesis and production fixtures,
  and rerun the complete acceptance suite and unsupported-binary gate.
- Independently review the custom certificate profile and approve an enforceable
  time threat model. Submitter/orderer-controlled timestamps are not independent
  proof of signing or endorsement time; accurate receiver clocks and the
  non-Byzantine Raft admission assumption remain essential. Do not claim secure
  production expiry enforcement based only on the current signatures.
- Validate distributed clock-skew and expiry-during-consensus liveness against
  the approved operational limits. Abrupt root removal, quorum loss, SmartBFT
  hybrid rotation and broader failure injection are not demonstrated by the
  current planned Raft demo tests.
- Preserve classical TLS/transport and CRL-authentication limitations, and
  establish production key custody, backup, rotation and disposal procedures.

Production-profile PQ2 acceptance remains open. Do not publish or externally
deploy temporary-profile artifacts, bypass the release guard, or begin PQ3.

## PQ3 handoff

PQ3 must not start until the maintainer explicitly reviews and accepts the PQ2
handoff and authorizes the next phase. Remaining production prerequisites
include obtaining the official Thinkstratum Ltd PEN; reviewing and approving
the production ASN.1/profile and permanent OIDs; regenerating all certificate,
MSP and genesis material; rerunning the complete PQ2 tests and unsupported
binary gate; and independent security review. The handoff must also carry
forward the time-design and distributed-liveness gates above, classical TLS
and CRL limitations, and production key-custody requirements. Recording these
items does not approve their design or move them silently into an authorized
PQ3 implementation. PQ2 does not include settlement application development
or a claim of fully quantum-resistant networking.
