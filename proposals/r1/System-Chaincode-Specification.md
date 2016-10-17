
System Chaincode
================

Introduction
------------

User written Chaincode runs in a container (called "user chaincode" in
this doc) and communicates with the peer over the network. These
chaincodes have restrictions with regards to what code they can execute.
For example, they can only interact with the peer via \`ChaincodeStub\`
interface such as GetState or PutState. There is a need for chaincode
that have these restrictions relaxed. Such chaincode is broadly termed
"System Chaincode".

The purpose of system chaincode is to enable customization of certain
behaviors of the fabric such as timer service or naming service.

The simplest way to design a system chaincode is to run it in the same
process as the Peer. This document goes into this design.

In-process chaincode
--------------------

The approach is to consider the changes and generalizations needed to
run chaincode in-process as the hosting peer.

Requirements

-   coexist with user chaincode
-   adhere to same lifecycle as user chaincode - except for possibly
    "deploy"
-   easy to install

The above requirements impose the following design principles

-   design consistent with user chaincode
-   uniform treatment of both types of chaincode in the codebase

Though the impetus for the in-process chaincode design comes from
"system chaincode", in principle the general "in process chaincode"
design would allow any chaincode to be deployed.

Key considerations for a chaincode environment
----------------------------------------------

There are two main considerations for any chaincode environment
(container, in-process,etc)

-   Runtime Considerations
-   Lifecycle Considerations

### Runtime Considerations

Runtime considerations can be broadly classified into Transport and
Execution mechanisms. The abstractions provided by the Fabric for these
mechanisms are key to the ability to extend the system to support new
chaincode runtimes consistent with existing ones in a fairly
straightforward manner.

### Transport mechanism

Peer and chaincode communicate using ChaincodeMessages.

-   the protocol is completely encapsulated via ChaincodeMessage
-   current gRPC streaming maps nicely to Go Channels

The above allow abstraction of a small section of code dealing with
chaincode transport, leaving most of the code common to both
environments. This is the key to the uniform treatment of runtime
environments.

### Execution mechanism

The fabric provides a basic interface for a “vm”. The Docker container
is implemented on top of this abstraction. The “vm” ([vm interface](https://github.com/hyperledger/fabric/blob/master/core/container/controller.go))
can be used to provide an execution environment for the in-process
runtime as well. This has the obvious benefit of encapsulating all
execution access (deploy, launch, stop, etc) transparent to the
implementation.

Lifecycle Considerations
------------------------

| Life cycle             | Container model                                                  | In-process Model                                                                                                                                                      |
|------------------------|------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Deploy                 | User deploys with an external request. Participates in consensus | Two possibilities +++  (1) comes up automatically with the peer, does not participate in consensus (2) user starts the chaincode with a request. Participates in consensus  |
| Invoke/query           | User sends request                                               | User sends request                                                                                                                                                    |
| Stop (not implemented) | User sends request                                               | User sends request                                                                                                                                                    |
| Upgrade *** (not impemented)           |                                                                  |                                                                                                                                                                       |
+++ System chaincodes that are part of genesis block cannot/should not
be part of consensus. E.g., bootstrapping requirements that would
prevent them from taking part in consensus. That is not true for
upgrading those chaincodes.

So this may then be the model - “system chaincode that are in genesis
block will not be deployed via consensus. All other chaincode deployment
and all chaincode upgrades (including system chaincode) will be part of
consensus.

\*\*\*\* Upgrade is mentioned here just for completeness. It is an
important part of life-cycle that should be a separate discussion unto
itself involving devops considerations such as versioning, staging,
staggering etc.

Deploying System Chaincodes
---------------------------

System chaincodes come up with a system. System chaincodes will be
defined in core.yaml. Minimally, a chaincode definition would contain
the following information { Name, Path }. Each chaincode definition
could also specify dependent chaincodes such that a chaincode will be
brought up only if its dependent chaincodes are. The chaincodes would be
brought up in the order specified in the list.
