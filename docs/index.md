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

The Fabric is an implementation of blockchain technology, leveraging familiar
and proven technologies. It is a modular architecture allowing pluggable
implementations of various function. It features powerful container technology
to host any mainstream language for smart contracts development.

## Releases

The Fabric releases are documented [here](releases.md). We
released our second release under the governance of the Hyperledger Project -
v0.6-preview in October, 2016.

If you are seeking a stable version of the Hyperledger Fabric on which to
develop applications or explore the technology, we **strongly recommend** that
you use the v0.6 [Starter Kit](http://hyperledger-fabric.readthedocs.io/en/v0.6/starter/fabric-starter-kit/)
while the v1.0 architectural refactoring is in development.

If you'd like a taste of what the Fabric v1.0 architecture has in store, we
invite you to read the preview [here](abstract_v1.md). Finally, if you are
adventurous, we invite you to explore the current state of development with
the caveat that it is not yet completely stable.

## Contributing to the project

We welcome contributions to the Hyperledger Project in many forms. There's
always plenty to do! Full details of how to contribute to this project are
documented in the [Fabric developer's guide](#fabric-developer-guide) below.

## Maintainers

The project's [maintainers](MAINTAINERS.md) are responsible for reviewing and
merging all patches submitted for review and they guide the over-all technical
direction of the project within the guidelines established by the Hyperledger
Project's Technical Steering Committee (TSC).

## Communication <a name="communication"></a>

We use [Hyperledger Slack](https://slack.hyperledger.org/) for communication and
Google Hangouts&trade; for screen sharing between developers. Our development
planning and prioritization is done in [JIRA](https://jira.hyperledger.org),
and we take longer running discussions/decisions to the
[mailing list](http://lists.hyperledger.org/mailman/listinfo/hyperledger-fabric).

## Still Have Questions?
We try to maintain a comprehensive set of documentation (see below) for various
audiences. However, we realize that often there are questions that remain
unanswered. For any technical questions relating to the Hyperledger Fabric
project not answered in this documentation, please use
[StackOverflow](http://stackoverflow.com/questions/tagged/hyperledger). If you
need help finding things, please don't hesitate to send a note to the
[mailing list](http://lists.hyperledger.org/mailman/listinfo/hyperledger-fabric),
or ask on [Slack]((https://slack.hyperledger.org/)).

# Hyperledger Fabric Documentation

The Hyperledger Fabric is an implementation of blockchain technology, that has
been collaboratively developed under the Linux Foundation's
[Hyperledger Project](http://hyperledger.org). It leverages familiar and
proven technologies, and offers a modular architecture
that allows pluggable implementations of various function including membership
services, consensus, and smart contracts (Chaincode) execution. It features
powerful container technology to host any mainstream language for smart
contracts development.

## Table of Contents

Below, you'll find the following sections:

* [Read All About It](#read-all-about-it)
* [Developer guides](#developer-guides)
    * [Application developer's guide](#application-developer-guide)
    * [Fabric developer's guide](#fabric-developer-guide)
* [Operations guide](#operations-guide)

## Read all about it

If you are new to the project, you might want to familiarize yourself with some
of the basics before diving into either developing applications using the
Hyperledger Fabric, or collaborating with us on our journey to continuously
extend and improve its capabilities.

- [Canonical use cases](biz/usecases.md)
- [Glossary](glossary.md): to understand the terminology that we use throughout
the Fabric project's documentation.
- [Fabric FAQs](https://github.com/hyperledger/fabric/tree/master/docs/FAQ)

# Developer guides

There are two distinct types of developers for which we have authored this
documentation: 1) [application developers](#application-developer-guide)
building applications and solutions using the Hyperledger Fabric 2) developers
who want to engage in the [development of the Hyperledger Fabric](#fabric-developer-guide)
itself. We distinguish these two personas because the development setup
for the Hyperledger Fabric developer is much more involved as they need to
be able to build the software and there are additional prerequisites that need
to be installed that are largely unnecessary for developers building
applications.

## Application developer guide

- [CLI](API/CLI.md): working with the command-line interface.
- [Node.js SDK](http://fabric-sdk-node.readthedocs.io/en/latest/node-sdk-guide):
working with the Node.js SDK.
- Java SDK (coming soon).
- Python SDK (coming soon).
- [Writing, Building, and Running Chaincode](Setup/Chaincode-setup.md):
a step-by-step guide to developing and testing Chaincode.
- [Chaincode FAQ](FAQ/chaincode_FAQ.md): a FAQ for all of your burning questions
relating to Chaincode.

## Fabric developer guide

- [Making code contributions](CONTRIBUTING.md): First, you'll want to familiarize
     yourself with the project's contribution guidelines.
- [Setting up the development environment](dev-setup/devenv.md): after that, you
     will want to set up your development environment.
- [Building the Fabric core](dev-setup/build.md): next, try building the project
     in your local development environment to ensure that everything is set up
     correctly.
- [Building outside of Vagrant](dev-setup/build.md#building-outside-of-vagrant):
     for the *adventurous*, you might try to build outside of the standard
     Vagrant development environment.
- [Logging control](Setup/logging-control.md): describes how to tweak the logging
     levels of various components within the Fabric.
- [License header](dev-setup/headers.txt): every source file must include this
     license header modified to include a copyright statement for the principle
     author(s).

# Operations guide

(coming soon)

**Note:** if you are looking for instructions to operate the fabric for a POC,
we recommend that you use the more stable v0.6 [Starter Kit](http://hyperledger-fabric.readthedocs.io/en/v0.6/starter/fabric-starter-kit/)

# License <a name="license"></a>
The Hyperledger Project uses the [Apache License Version 2.0](LICENSE) software
license.
