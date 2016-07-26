## Hyperledger Fabric Documentation
The Hyperledger [fabric](https://github.com/hyperledger/fabric) is an implementation of blockchain technology, that has been collaboratively developed under the Linux Foundation's [Hyperledger Project](http://hyperledger.org). It leverages familiar and proven technologies, and offers a modular architecture that allows pluggable implementations of various function including membership services, consensus, and smart contracts (Chaincode) execution. It features powerful container technology to host any mainstream language for smart contracts development.

To contribute to this documentation, create an issue for any requests for clarification or to highlight any errors, or you may fork and update the [source](https://github.com/hyperledger/fabric), and submit a pull request.

Below, you'll find the following sections:

- [Getting started](#getting-started)
- [Quickstart](#quickstart-documentation)
- Developer guides
  - [Fabric developer guide](#fabric-developer-guide)
  - [Chaincode developer guide](#chaincode-developer-guide)
  - [API developer guide](#api-developer-guide)
- [Operations guide](#operations-guide)

## Getting started

If you are new to the project, you can begin by reviewing the following links. If you'd prefer to dive right in, see the [Quickstart](#quickstart-documentation) section, below.

- [Whitepaper WG](https://github.com/hyperledger/hyperledger/wiki/Whitepaper-WG): where the community is developing a whitepaper to explain the motivation and goals for the project.
- [Requirements WG](https://github.com/hyperledger/hyperledger/wiki/Requirements-WG): where the community is developing use cases and requirements.
- [Canonical use cases](biz/usecases.md)
- [Glossary](glossary.md): to understand the terminology that we use throughout the Fabric project's documentation.
- [Fabric FAQs](https://github.com/hyperledger/fabric/tree/master/docs/FAQ)

## Quickstart documentation

- [Development environment set-up](dev-setup/devenv.md): if you are considering helping with development of the Hyperledger Fabric or Fabric-API projects themselves, this guide will help you install and configure all you'll need. The development environment is also useful (but, not necessary) for developing blockchain applications and/or Chaincode.
- [Network setup](Setup/Network-setup.md): This document covers setting up a network on your local machine for development.
- [Chaincode development environment](Setup/Chaincode-setup.md): Chaincode developers need a way to test and debug their Chaincode without having to set up a complete peer network. This document describes how to write, build, and test Chaincode in a local development environment.
- [APIs](API/CoreAPI.md): This document covers the available APIs for interacting with a peer node.

## Fabric developer guide

When you are ready to start contributing to the Hyperledger fabric project, we strongly recommend that you read the [protocol specification](protocol-spec.md) for the technical details so that you have a better understanding of how the code fits together.

- [Making code contributions](https://github.com/hyperledger/fabric/blob/master/CONTRIBUTING.md): First, you'll want to familiarize yourself with the project's contribution guidelines.
- [Setting up the development environment](dev-setup/devenv.md): after that, you will want to set up your development environment.
- [Building the fabric core](dev-setup/build.md): next, try building the project in your local development environment to ensure that everything is set up correctly.
- [Building outside of Vagrant](dev-setup/build.md#building-outside-of-vagrant): for the adventurous, you might try to build outside of the standard Vagrant development environment.
- [Logging control](Setup/logging-control.md): describes how to tweak the logging levels of various components within the fabric.
- [License header](dev-setup/headers.txt): every source file must include this license header modified to include a copyright statement for the principle author(s).

## Chaincode developer guide

- [Setting up the development environment](dev-setup/devenv.md): when developing and testing Chaincode, or an application that leverages the fabric API or SDK, you'll probably want to run the fabric locally on your laptop to test. You can use the same setup that Fabric developers use.
- [Setting Up a Network For Development](Setup/Network-setup.md): alternately, you can follow these instructions for setting up a local network for Chaincode development without the entire fabric development environment setup.
- [Writing, Building, and Running Chaincode in a Development Environment](Setup/Chaincode-setup.md): a step-by-step guide to writing and testing Chaincode.
- [Chaincode FAQ](FAQ/chaincode_FAQ.md): a FAQ for all of your burning questions relating to Chaincode.

## API developer guide

- [APIs - CLI, REST, and Node.js](API/CoreAPI.md)
     - [CLI](API/CoreAPI.md#cli): working with the command-line interface.
     - [REST](API/CoreAPI.md#rest-api): working with the REST API.
     - [Node.js SDK](https://github.com/hyperledger/fabric/blob/master/sdk/node/README.md): working with the Node.js SDK.

## Operations guide

- [Setting Up a Network](Setup/Network-setup.md): instructions for setting up a network of fabric peers.
- [Certificate Authority (CA) Setup](Setup/ca-setup.md): setting up a CA to support identity, security (authentication/authorization), privacy and confidentiality.
- [Application ACL](tech/application-ACL.md): working with access control lists.
