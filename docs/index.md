# Incubation Notice

This project is a Hyperledger project in _Incubation_. It was proposed to the
community and documented [here](https://goo.gl/RYQZ5N). Information on what
_Incubation_ entails can be found in the [Hyperledger Project Lifecycle
document](https://goo.gl/4edNRc).

[![Build Status](https://jenkins.hyperledger.org/buildStatus/icon?job=fabric-merge-x86_64)](https://jenkins.hyperledger.org/view/fabric/job/fabric-merge-x86_64/)
[![Go Report Card](https://goreportcard.com/badge/github.com/hyperledger/fabric)](https://goreportcard.com/report/github.com/hyperledger/fabric)
[![GoDoc](https://godoc.org/github.com/hyperledger/fabric?status.svg)](https://godoc.org/github.com/hyperledger/fabric)
[![Documentation Status](https://readthedocs.org/projects/hyperledger-fabric/badge/?version=latest)](http://hyperledger-fabric.readthedocs.io/en/latest/?badge=latest)

# Hyperledger Fabric

The fabric is an implementation of blockchain technology, leveraging familiar
and proven technologies. It is a modular architecture allowing pluggable
implementations of various function. It features powerful container technology
to host any mainstream language for smart contracts development.

## Releases

The fabric releases are documented
[here](https://github.com/hyperledger/fabric/wiki/Fabric-Releases). We have just
released our first release under the governance of the Hyperledger Project -
v0.5-developer-preview.

## Contributing to the project

We welcome contributions to the Hyperledger Project in many forms. There's
always plenty to do! Full details of how to contribute to this project are
documented in the [Fabric developer's guide](#fabric-developer-guide) below.

To contribute to this documentation, create an issue for any requests for
clarification or to highlight any errors, or you may clone and update the
[source](https://gerrit.hyperledger.org/r/#/admin/projects/fabric), and submit a
Gerrit review (essentially the same process as for fabric development).

## Maintainers

The project's [maintainers](MAINTAINERS.md): are responsible for reviewing and
merging all patches submitted for review and they guide the over-all technical
direction of the project within the guidelines established by the Hyperledger
Project's Technical Steering Committee (TSC).

## Communication <a name="communication"></a>

We use [Hyperledger Slack](https://slack.hyperledger.org/) for communication and
Google Hangouts&trade; for screen sharing between developers.

# Hyperledger Fabric Documentation

The Hyperledger
[fabric](https://gerrit.hyperledger.org/r/#/admin/projects/fabric) is an
implementation of blockchain technology, that has been collaboratively developed
under the Linux Foundation's [Hyperledger Project](http://hyperledger.org). It
leverages familiar and proven technologies, and offers a modular architecture
that allows pluggable implementations of various function including membership
services, consensus, and smart contracts (Chaincode) execution. It features
powerful container technology to host any mainstream language for smart
contracts development.

## Still Have Questions?
We try to maintain a comprehensive set of documentation for various audiences.
However, we realize that often there are questions that remain unanswered. For
any technical questions relating to the Hyperledger Fabric project not answered
in this documentation, please use
[StackOverflow](http://stackoverflow.com/questions/tagged/hyperledger).

## TOC

Below, you'll find the following sections:

- [Getting started](#getting-started)
- [Quickstart](#quickstart-documentation)
- [Developer guides](#developer-guides)

  - [Fabric developer's guide](#fabric-developer-guide)
  - [Chaincode developer's guide](#chaincode-developer-guide)
  - [API developer's guide](#api-developer-guide)

- [Operations guide](#operations-guide)

# Getting started

If you are new to the project, you can begin by reviewing the following links.
If you'd prefer to dive right in, see the
[Quickstart](#quickstart-documentation) section, below.

- [Whitepaper WG](https://github.com/hyperledger/hyperledger/wiki/Whitepaper-WG):
where the community is developing a whitepaper to explain the motivation and
goals for the project.
- [Requirements WG](https://github.com/hyperledger/hyperledger/wiki/Requirements-WG):
where the community is developing use cases and requirements.
- [Canonical use cases](biz/usecases.md)
- [Glossary](glossary.md): to understand the terminology that we use throughout
the Fabric project's documentation.
- [Fabric FAQs](https://github.com/hyperledger/fabric/tree/master/docs/FAQ)

# Quickstart documentation

- [Development environment set-up](dev-setup/devenv.md): if you are considering
helping with development of the Hyperledger Fabric or Fabric-API projects
themselves, this guide will help you install and configure all you'll need. The
development environment is also useful (but, not necessary) for developing
blockchain applications and/or Chaincode.
- [Network setup](Setup/Network-setup.md): This document covers setting up a
network on your local machine for development.
- [Chaincode development environment](Setup/Chaincode-setup.md): Chaincode
developers need a way to test and debug their Chaincode without having to set up
a complete peer network. This document describes how to write, build, and test
Chaincode in a local development environment.
- [APIs](API/CoreAPI.md): This document covers the available APIs for
interacting with a peer node.

# Developer guides

## Fabric developer guide

When you are ready to start contributing to the Hyperledger fabric project, we
strongly recommend that you read the [protocol specification](protocol-spec.md)
for the technical details so that you have a better understanding of how the
code fits together.

- [Making code contributions](CONTRIBUTING.md): First, you'll want to familiarize
yourself with the project's contribution guidelines.
- [Setting up the development environment](dev-setup/devenv.md): after that, you
will want to set up your development environment.
- [Building the fabric core](dev-setup/build.md): next, try building the project
in your local development environment to ensure that everything is set up
correctly.
- [Building outside of Vagrant](dev-setup/build.md#building-outside-of-vagrant):
for the adventurous, you might try to build outside of the standard Vagrant
development environment.
- [Logging control](Setup/logging-control.md): describes how to tweak the logging
levels of various components within the fabric.
- [License header](dev-setup/headers.txt): every source file must include this
license header modified to include a copyright statement for the principle
author(s).

## Chaincode developer guide

- [Setting up the development environment](dev-setup/devenv.md): when developing
and testing Chaincode, or an application that leverages the fabric API or SDK,
you'll probably want to run the fabric locally on your laptop to test. You can
use the same setup that Fabric developers use.
- [Setting Up a Network For Development](Setup/Network-setup.md): alternately, you
can follow these instructions for setting up a local network for Chaincode
development without the entire fabric development environment setup.
- [Writing, Building, and Running Chaincode in a Development
Environment](Setup/Chaincode-setup.md): a step-by-step guide to writing and
testing Chaincode.
- [Chaincode FAQ](FAQ/chaincode_FAQ.md): a FAQ for all of your burning questions
relating to Chaincode.

## API developer guide

- [APIs - CLI, REST, and Node.js](API/CoreAPI.md)
     - [CLI](API/CoreAPI.md#cli): working with the command-line interface.
     - [REST](API/CoreAPI.md#rest-api): working with the REST API.
     - [Node.js SDK](nodeSDK/node-sdk-guide.md): working with the Node.js SDK.

# Operations guide

- [Setting Up a Network](Setup/Network-setup.md): instructions for setting up a
network of fabric peers.
- [Certificate Authority (CA) Setup](Setup/ca-setup.md): setting up a CA to
support identity, security (authentication/authorization), privacy and
confidentiality.
- [Application ACL](tech/application-ACL.md): working with access control lists.

## License <a name="license"></a>
The Hyperledger Project uses the [Apache License Version 2.0](LICENSE) software
license.
