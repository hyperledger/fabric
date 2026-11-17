# ADR: Pactway Hybrid Certificates and MSP Policy

## Status

Accepted for isolated PQ2 development and testing. The production certificate
profile and production OIDs are not approved or frozen. The supported Fabric
CI baseline and focused hybrid tests are passing; see the
[development acceptance report](development-and-validation.md). The closeout
recommendation is maintainer review for a development-only merge, not automatic
merge authorization, production approval, or permission to begin PQ3.

## Context

PQ1 adds ML-DSA-65 application signatures but does not bind the alternate key
to the complete X.509 certificate or require its use through authenticated
channel policy. The former alternate-key certificate construction signed only
key material and could leave its signature reusable after changes to
certificate fields. Merely finding a PQ extension is not an identity policy.

The project selected Thinkstratum Ltd as the intended IANA Private Enterprise
Number (PEN) owner. The PEN has not yet been allocated. The proposed production
subtree and final ASN.1 profile must remain unfrozen until allocation and
profile approval.

## Decision

Implement a new, explicitly versioned Pactway hybrid-certificate profile using
the project's own allocated namespace when available. Do not adopt or claim
RFC 10073 composite compliance. During authorized development only, use the
RFC 5612 example enterprise number `32473`:

- `1.3.6.1.4.1.32473.1.1`: temporary profile/key extension.
- `1.3.6.1.4.1.32473.1.2`: temporary PQ-signature extension.

These are test fixtures, not identifiers assigned to Thinkstratum Ltd. The
temporary identifiers are centralized in the profile registry and must not be
used in released or externally deployed networks. Runtime selection must name
one profile explicitly; there is no acceptance of multiple namespaces as a
fallback. The release workflow blocks while the registry contains any
development-only profile.

The profile keeps the ordinary classical X.509 signature and adds exactly one
profile/key extension and one PQ signature extension. The key extension
contains version 1, an ML-DSA-65 SubjectPublicKeyInfo using RFC 9881 OID
`2.16.840.1.101.3.4.3.18` with absent parameters, and the PQ algorithm
identifier. The signature extension contains a byte-aligned DER BIT STRING.
Both Pactway extensions are noncritical; this preserves ordinary X.509
parsing, but is not a security guarantee for validators that ignore them.

The PQ signature input is the domain label `Pactway-Hybrid-Certificate-v1`,
one zero byte, and the exact DER TBSCertificate with only the PQ-signature
extension removed. Removal preserves every other original TLV and extension
ordering, and recalculates only enclosing DER lengths. The classical issuer
signature covers the final TBSCertificate including the PQ signature
extension. Signing uses the issuer's PQ key; certificate creation and
validation reject key/profile mismatches and malformed DER.

Validation first applies standard Fabric and Go X.509 path, validity, usage,
constraint, revocation and trust-anchor checks. For required enrollment MSPs,
every certificate in the selected unique leaf-to-root chain must use the same
supported profile. Each child is verified using the immediate issuer's PQ key.
Configured self-signed roots have their self-issuance, classical
self-signature, and PQ self-signature checked explicitly. A trust anchor is
trusted because it is configured, not because it self-signs.

## MSP policy and migration

- The authenticated channel capability `PactwayHybridCertificateV1` requires
  every channel enrollment MSP to use valid hybrid certificate chains and
  hybrid application signatures. The capability is included from fresh
  genesis, cannot be removed by a config update, and required channels reject
  Idemix MSPs.
- Local signing MSPs use the `PQ_REQUIRED` sidecar and must prove possession of
  matching classical and ML-DSA private keys at setup. A local marker cannot
  override channel policy. Channel/verifying MSP configs receive policy from
  authenticated channel capabilities and do not inherit local sidecars.
- `CLASSICAL_PERMITTED` remains the default where the capability is absent.
  Classical credentials remain subject to normal Fabric rules there. A
  recognized but malformed, partial, unsupported, or invalid Pactway profile
  never falls back to classical verification.
- Enrollment roots, intermediates, clients, administrators, peers and orderers
  use fresh, independently generated hybrid keys in the demonstration network.
  Classical TLS certificates, TLS trust roots, revocation transport and
  consensus/gossip transport are outside the hybrid profile and remain
  classical.
- The migration target is a fresh genesis and new identities. Do not add
  legacy alternate-key certificate verification or reuse historical demo
  private keys without separate authorization.

## Time and authorization trust model

Hybrid certificate validity checks require a time source; a valid signature
does not independently establish when the message was created. PQ2 separates
live authorization from historical verification:

- Live envelope authorization and orderer endorsement admission use the
  receiving node's clock. A hybrid local signing identity must be valid when
  signing. SmartBFT live commit votes must pass both current-clock and signed
  timestamp checks; an expired consenter cannot regain live authorization
  merely by backdating its nonce.
- Transaction validation/replay uses the submitter-signed channel-header
  timestamp; block signatures and historical SmartBFT signatures use the
  timestamp authenticated by the orderer signature. These are replay evidence,
  not independent trusted-time proofs. Missing or malformed required evidence
  must not cause a classical or current-time fallback.
- The isolated demo assumes uncompromised, accurately maintained receiver
  clocks and, for Raft, non-Byzantine orderers enforcing admission. SmartBFT
  adds current-time vote admission but does not turn a Byzantine signer's
  timestamp into proof of actual signing time. No maximum clock-skew or
  admission-to-commit-delay bound is enforced.
- A request admitted before expiry may commit afterwards and remain valid on
  replay. Receiver clock skew or certificate expiry during consensus can
  produce different live admission decisions and impair liveness. Maintain
  clocks, rotate credentials with overlapping trust before expiry, and do not
  claim distributed clock-skew or expiry-during-consensus liveness guarantees.

The focused real-MSP SmartBFT regression checks historical validity at
`NotAfter - 1ns`, `NotAfter`, and `NotAfter + 1ns`, and rejection of live votes
from an already expired hybrid consenter despite a historically valid
timestamp. The separate consensus dispatch regression checks that rejected
live votes are not counted. These are boundary/composition tests, not a live
multi-node clock-skew experiment.

Production expiry assurance requires a separately approved enforceable time
design and threat model, including malicious timestamp assertions, receiver
clock compromise and distributed liveness. The development closeout does not
accept those risks on behalf of a production operator or silently defer their
solution to an authorized PQ3 implementation.

## Consequences

The custom profile can use Go's conventional classical X.509 path validation
and preserve PQ1 application-signing semantics, but requires bespoke ASN.1,
certificate, MSP, tooling and interoperability maintenance. It does not
interoperate as RFC 10073 composite certificates and must not be described as
RFC-compliant. Upstream validators that ignore noncritical extensions must be
prevented from joining by the authenticated channel capability.

The profile supplies post-quantum enrollment signing, not complete
quantum-resistant network security. TLS, key exchange, revocation and
unmodified external client transport remain classical. This ADR does not
authorize PQ3 or production deployment of temporary identifiers.

## References

- [RFC 5280](https://www.rfc-editor.org/rfc/rfc5280.html)
- [RFC 9881](https://www.rfc-editor.org/rfc/rfc9881.html)
- [RFC 10073](https://www.rfc-editor.org/rfc/rfc10073.html)
- [RFC 5612](https://www.rfc-editor.org/rfc/rfc5612.html)
- [PQ1 migration and validation evidence](../../pq-crypto/PQ1-MIGRATION.md)
