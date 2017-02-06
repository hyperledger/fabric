[![Build Status](https://jenkins.hyperledger.org/buildStatus/icon?job=fabric-merge-x86_64)](https://jenkins.hyperledger.org/view/fabric/job/fabric-merge-x86_64/)
[![Go Report Card](https://goreportcard.com/badge/github.com/hyperledger/fabric)](https://goreportcard.com/report/github.com/hyperledger/fabric)
[![GoDoc](https://godoc.org/github.com/hyperledger/fabric?status.svg)](https://godoc.org/github.com/hyperledger/fabric)
[![Documentation Status](https://readthedocs.org/projects/hyperledger-fabric/badge/?version=latest)](http://hyperledger-fabric.readthedocs.io/en/latest/?badge=latest)

# Hyperledger Fabric

The Hyperledger Fabric is an implementation of blockchain technology,
leveraging familiar and proven technologies. It offers a modular
[architecture](architecture.md) allowing pluggable implementations of
various capabilities, such as smart contracts (chaincode), cryptography,
consensus, ledger datastore and more. It leverages powerful container technology
for its deployment and for providing the execution environment for the smart
contracts (chaincode) such that the network is protected from malicious code.

## Get Hyperledger Fabric

The Fabric releases are documented [here](releases.md). We
published our second release under the governance of the Hyperledger Project -
[v0.6-preview](http://hyperledger-fabric.readthedocs.io/en/v0.6/) in October,
2016.

If you are seeking a **stable** version of the Hyperledger Fabric on which to
develop applications or explore the technology, we **strongly recommend** that
you use the
[v0.6 Starter Kit](http://hyperledger-fabric.readthedocs.io/en/v0.6/starter/fabric-starter-kit/)
while the architectural refactoring for our upcoming v1.0 release is still in
development.

However, if you'd like a taste of what the Hyperledger Fabric architecture has
in store, we invite you to read the preview [here](abstract_v1.md).

If you are *adventurous*, we invite you to explore the current state of
development using the [v1.0 Starter Kit](gettingstarted.md), with the caveat
that it is not yet completely stable. We hope to have a stable v1.0-alpha
release in the very near term.

# Hyperledger Fabric Documentation

If you are new to the project, you might want to familiarize yourself with some
of the basics before diving into either developing applications using the
Hyperledger Fabric, or collaborating with us on our journey to continuously
extend and improve its capabilities.

- [Use cases](biz/usecases.md)
- [Glossary](glossary.md): to understand the terminology that we use throughout
the Hyperledger Fabric project's documentation.
- [Hyperledger Fabric FAQ](https://github.com/hyperledger/fabric/tree/master/docs/FAQ)

## Application developer guide

- [CLI](API/CLI.md): working with the command-line interface.
- [Node.js SDK](http://fabric-sdk-node.readthedocs.io/en/latest/node-sdk-guide):
working with the Node.js SDK.
- Java SDK (coming soon).
- [Python SDK](https://wiki.hyperledger.org/projects/fabric-sdk-py.md): working with the Python SDK.
- [Writing, Building, and Running Chaincode](Setup/Chaincode-setup.md):
a step-by-step guide to developing and testing Chaincode.
- [Chaincode FAQ](FAQ/chaincode_FAQ.md): a FAQ for all of your burning questions
relating to Chaincode.

# Operations guide

(coming soon)

**Note:** if you are looking for instructions to operate the fabric for a POC,
we recommend that you use the more stable [v0.6 Starter Kit](http://hyperledger-fabric.readthedocs.io/en/v0.6/starter/fabric-starter-kit/).
However, the [v1.0 Starter Kit](gettingstarted.md) is also available, though
subject to change as we stabilize the code for the upcoming v1.0-alpha release.

## Contributing to the project

We welcome contributions to the Hyperledger Project in many forms. There's
always plenty to do! Full details of how to contribute to this project are
documented in the [Contribution Guidelines](CONTRIBUTING.md).

## Still Have Questions?
We try to maintain a comprehensive set of documentation (see below) for various
audiences. However, we realize that often there are questions that remain
unanswered. For any technical questions relating to the Hyperledger Fabric
project not answered in this documentation, please use
[StackOverflow](http://stackoverflow.com/questions/tagged/hyperledger). If you
need help finding things, please don't hesitate to send a note to the
[mailing list](http://lists.hyperledger.org/mailman/listinfo/hyperledger-fabric),
or ask on [RocketChat]((https://chat.hyperledger.org/)) (an alternative to
Slack).

# Incubation Notice

This project is a Hyperledger project in _Incubation_. It was proposed to the
community and documented [here](https://goo.gl/RYQZ5N). Information on what
_Incubation_ entails can be found in the [Hyperledger Project Lifecycle
document](https://goo.gl/4edNRc).

# License <a name="license"></a>
The Hyperledger Project uses the [Apache License Version 2.0](LICENSE) software
license.
