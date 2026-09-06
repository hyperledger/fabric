# Security Policy

This document defines how security vulnerability reporting is handled for Hyperledger Fabric. The
approach aligns with the
[LF Decentralized Trust security policy](https://lf-decentralized-trust.github.io/governance/governing-documents/security).
Please review that document to understand the basis of security reporting for this project. Details
specific to this repository are documented below.

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues, discussions, or pull
requests.**

Suspected security vulnerabilities in this project can be reported privately using the repository's
[security advisories page](https://github.com/hyperledger/fabric/security/advisories/new). Guidance
can be found in the GitHub documentation on
[privately reporting a security vulnerability](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing/privately-reporting-a-security-vulnerability).

Reports are also always accepted by email to the LF Decentralized Trust security list,
[security@lists.lfdecentralizedtrust.org](mailto:security@lists.lfdecentralizedtrust.org). Include
the name of the project or repository along with the details listed below. If triage determines the
issue is a security vulnerability, the security team will open a GitHub security advisory for it.

### What to include

The more detail you can give us, the faster we can confirm and fix the issue. Where possible,
please include:

- The affected version, commit, or release.
- A description of the vulnerability and its potential impact.
- Step-by-step instructions to reproduce it, including any proof-of-concept code.
- Your assessment of the severity, and any suggested mitigation or fix.
- How you would like to be credited in the advisory, if the issue is confirmed.

## What to expect

- We will acknowledge receipt of your report within 2 business days.
- The maintainers will work with you to confirm the vulnerability and keep you updated on progress.
- We will coordinate a fix and a public security bulletin with you, and credit you as the
  discoverer unless you ask us not to.

Please keep the details of the issue confidential until an advisory has been published, so that
users have a chance to update.

## Vulnerabilities in dependencies

Dependencies are regularly scanned for published security vulnerabilities, and these are addressed
as soon as practical. In general it should not be necessary to report vulnerabilities in project
dependencies.
