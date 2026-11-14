# PQ1: ML-DSA-65 migration and compatibility

## Status

The implementation migrates the PQ signing wrapper to the Go standard-library
`crypto/mldsa` API and pins the root module toolchain to Go 1.27.4. This report
records the final PQ1 merge validation and its remaining evidence limits. PQ1
was merged with no code changes from its reviewed PR head. Do not interpret the
CI results below as evidence for subsequent PQ2 changes or as coverage of
standalone nested-module tests.

## Baseline

- Baseline commit: `19a9bbb3684e636a430e181535ecf421e46b077d`.
- Feature branch: `feature/pq1-ml-dsa65`.
- Environment: WSL Ubuntu 24.04, Go 1.27.4, GCC 13.3.0. Docker was unavailable.
- Baseline commands and outcomes:
  - `go test -short ./pq-crypto` — passed (`7.608s`).
  - `go test ./msp` — passed (`5.708s`).
  - `go test -mod=readonly ./bccsp/sw` in a temporary Go workspace joining the
    root and local `fabric-lib-go` modules — passed (`0.635s`).
- `make unit-test` was not run because it requires Docker.
- The temporary BCCSP workspace was removed; module hashes were unchanged.

Final merged PR evidence:

- PR #3 final head: `7a17b4d0932f4df9ae3ed5829eb082748fed95a9`; it was merged
  into the pinned base with no code changes.
- CI run 43332454263 reported success for Basic Checks, `make unit-test`, and
  all nine `make integration-test INTEGRATION_TEST_SUITE="..."` matrix jobs:
  `raft`, `pvtdata`, `pvtdatapurge e2e`, `ledger`, `lifecycle`, `smartbft`,
  `discovery gossip devmode pluggable`, `gateway idemix pkcs11 configtx
  configtxlator`, and `sbe nwo msp`.
- The CI unit-test script covers root-module `github.com/hyperledger/fabric/...`
  packages and excludes integration packages; it does not independently
  enumerate the nested replacement module's `bccsp/sw` tests.
- The workflow selects Go using `go-version-file: go.mod`, which declares
  `go 1.27.3` and `toolchain go1.27.4`. The exact runtime version was not
  independently verified because job-log retrieval returned HTTP 403.
- The standalone post-change BCCSP test remains unconfirmed:
  `go test -mod=readonly ./bccsp/sw` in a temporary workspace joining the root
  and local `fabric-lib-go` modules was passed on the baseline, but not rerun
  after PQ1. `make unit-test` did pass in CI; this does not establish that
  separate nested-module command.
- These results are the strongest available final PQ1 evidence. They do not
  remove the standalone BCCSP and exact-toolchain evidence gaps above.

## Backend assessment

The implementation selects Go's standard-library `crypto/mldsa` on the pinned
Go 1.27.4 toolchain instead of liboqs 0.16.0. This provides FIPS 204 ML-DSA-65
key generation, seed import, signing, verification, and standard PKIX/PKCS#8
key encoding without cgo, a separately built native library, or bundled
platform binaries. liboqs 0.16.0 is a viable maintained alternative and
includes `mldsa-native`, but would retain native toolchain, distribution, and
platform-specific build requirements not needed by this repository's current
Go implementation.

The standard-library backend has no separate Go module checksum to pin: its
implementation is fixed by the Go toolchain. The root module declares
`toolchain go1.27.4`. Reproducibility still requires using that exact Go
toolchain and the repository's normal dependency-locking/build process.

## Versioned signing profile

`MLDSA65ProfileV1` names the following fixed profile:

- Algorithm: FIPS 204 ML-DSA-65, **pure ML-DSA**.
- Context: the empty string.
- Input: the exact byte slice received by the PQ signer. This profile does not
  reinterpret a supplied digest as HashML-DSA or hash it a second time.
- Legacy names such as `Dilithium3` are rejected. `DEFAULT` is retained only as
  a source-compatibility alias for this one ML-DSA-65 profile; algorithm
  selection does not depend on liboqs ordering.

Existing Fabric call semantics are preserved. For non-Ed25519 MSP identities,
MSP hashes the message according to its configured signature hash family before
passing those digest bytes to the PQ signer. For Ed25519 identities it passes
the message bytes. In the existing hybrid identity signature, the PQ signer
signs those bytes first; the classical signer signs SHA-384 over the
concatenation of those same bytes and the PQ signature. The alternate-key
certificate helper continues to sign its existing DER-encoded key-material
message directly. Context and domain-separation behavior have not been
silently changed.

## Identifiers and encodings

ML-DSA-65 uses RFC 9881's NIST OID
`2.16.840.1.101.3.4.3.18`. RFC 9881 requires absent parameters; encoders omit
them and the alternate public-key parser rejects a present parameter value.
The public-key bit string contains the raw FIPS 204 ML-DSA-65 public-key bytes.
Private keys are encoded as PKCS#8 using the RFC 9881 seed representation.

This does **not** make Fabric's existing hybrid certificate format RFC 9881
compliant. The alternate public key and signature still use the existing
custom extensions (`2.5.29.71`, `.72`, `.73`) associated with a draft format.
The extension currently embeds an RFC 9881 ML-DSA `SubjectPublicKeyInfo`, but
the hybrid certificate construction and validation remain custom and are not a
standardized hybrid profile.

## Compatibility and rollout

- Existing Dilithium3 keys, certificates, and signatures are not compatible
  with ML-DSA and are rejected; there is no classical fallback.
- This migration targets a fresh demonstration network only: generate a new
  genesis block and regenerate demonstration identities and keys before use.
- Targeted searches of `sampleconfig` and `integration` found no committed PQ
  demonstration keys or Dilithium/OQS references to regenerate in those trees.
  Any locally generated network artifacts must still be recreated.
- Historical signature verification is not claimed. Any future legacy
  verify-only support needs a separate compatibility decision, implementation,
  and tests; legacy signing must remain disabled.
- Standard RFC 9881 ML-DSA certificate and private-key encodings do not
  standardize Fabric's hybrid extensions or its existing nested signature
  structure.

## Tests and vectors

The focused tests include key/signature size checks, successful signing and
verification, wrong message/key and modified signature rejection, malformed
keys, unsupported profile rejection, RFC 9881 OID/absent-parameter checks, and
PKIX/PKCS#8 round trips. PKCS#8 import now rejects ML-DSA-44 and ML-DSA-87
instead of labeling them ML-DSA-65. A hybrid alternate-signature input fixture
pins the DER field order and exact bytes passed to the signer.

The ML-DSA-65 deterministic signing test uses C2SP/Wycheproof
`testvectors_v1/mldsa_65_sign_seed_test.json`, tcId 1 (`baseline`), pinned at
commit `20b256949ea8f21a11375619b1825027ca0668bd` and Git blob
`ff302e20c0b847e66fdcc7444cd63fc1cd7bf634`. The vector's omitted context means
the empty/default context. The checked seed is
`2a` repeated 32 times; the expected derived public-key SHA-256 is
`b7acce2ddb11f8cc1aa46e2bafac6eacfa2b732ef192bd636ad8d3a56d649c66`, and the
expected deterministic signature SHA-256 is
`39fbbb0d97a52c79844213b325af823a7f16a174e00a5b3daeb3e6e6d1c89681`.
The source repository is Apache-2.0 licensed.

The standalone BCCSP command remains to be run:

```text
GOTOOLCHAIN=go1.27.4 go test -mod=readonly ./bccsp/sw
```

The BCCSP command requires a temporary Go workspace joining the root and local
`fabric-lib-go` modules, as it did for the baseline. Full Fabric unit tests,
race tests, and performance tests have not been run after the change.

## Security limitations and unresolved work

- This is not a FIPS 140-3 validated module claim. Go documents ML-DSA as
  unavailable in FIPS module v1.0 mode; errors must propagate, without fallback.
- The implementation has not received a dedicated cryptographic review or
  side-channel analysis. Go's key API exposes the seed bytes for serialization;
  zeroization guarantees are not established here.
- The vendored liboqs native binaries were removed. The now-unused headers under
  `pq-crypto/include/oqs` remain pending authorization to remove them; they are
  not used by the new Go implementation.
- Existing hybrid signing retains no explicit protocol/domain tag and uses the
  current nested-signature construction. This behavior is deliberately
  preserved for PQ1; changing it requires a separately specified and tested
  migration.
- ML-DSA support is pinned to Go 1.27.4. Consumers must use a toolchain
  containing `crypto/mldsa`; older toolchains cannot build this package.

## PQ2: certificate and MSP modernization

PQ2 uses a custom, explicitly versioned Pactway hybrid-certificate profile;
this is not RFC 10073 composite and does not claim RFC compliance. The current
development profile uses only isolated RFC 5612 test OIDs while Thinkstratum
Ltd awaits its official IANA PEN. See
[`docs/pq2/adr-hybrid-identities.md`](../docs/pq2/adr-hybrid-identities.md)
for the accepted design, policy, compatibility boundary and remaining gates.
