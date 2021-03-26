# Security Model

Hyperledger Fabric is a permissioned blockchain where each component and actor has an identity, and policies define access control and governance.
This topic provides an overview of the Fabric security model and includes links to additional information.

## Identities

The different actors in a blockchain network include peers, orderers, client applications, administrators and more.
Each of these actors — active elements inside or outside a network able to consume services — has a digital identity encapsulated in an X.509 digital certificate issued by a Certificate Authority (CA).
These identities matter because they determine the exact permissions over resources and access to information that actors have in a blockchain network.

For more information see the [Identity topic](./identity/identity.html).

## Membership Service Providers

For an identity to be verifiable, it must come from a trusted authority.
A membership service provider (MSP) is that trusted authority in Fabric.
More specifically, an MSP is a component that defines the rules that govern the valid identities for an organization.
A Hyperledger Fabric channel defines a set of organization MSPs as members.
The default MSP implementation in Fabric uses X.509 certificates issued by a Certificate Authority (CA) as identities, adopting a traditional Public Key Infrastructure (PKI) hierarchical model.
Identities can be associated with roles within a MSP such as ‘client’ and ‘admin’ by utilizing Node OU roles.
Node OU roles can be used in policy definitions in order to restrict access to Fabric resources to certain MSPs and roles.

For more information see the [Membership Service Providers (MSPs) topic](./membership/membership.html).

## Policies

In Hyperledger Fabric, policies are the mechanism for infrastructure management.
Fabric policies represent how members come to agreement on accepting or rejecting changes to the network, a channel, or a smart contract.
Policies are agreed to by the channel members when the channel is originally configured, but they can also be modified as the channel evolves.
For example, they describe the criteria for adding or removing members from a channel, change how blocks are formed, or specify the number of organizations required to endorse a smart contract.
All of these actions are described by a policy which defines who can perform the action.
Simply put, everything you want to do on a Fabric network is controlled by a policy.
Once they are written, policies evaluate the collection of signatures attached to transactions and proposals and validate if the signatures fulfill the governance agreed to by the network.

Policies can be used in Channel Policies, Channel Modification Policies, Access Control Lists, Chaincode Lifecycle Policies, and Chaincode Endorsement Policies.

For more information see the [Policies topic](./policies/policies.html).

### Channel Policies

Policies in the channel configuration define various usage and administrative policies on a channel.
For example, the policy for adding a peer organization to a channel is defined within the administrative domain of the peer organizations (known as the Application group).
Similarly, adding ordering nodes in the consenter set of the channel is controlled by a policy inside the Orderer group.
Actions that cross both the peer and orderer organizational domains are contained in the Channel group.

For more information see the [Channel Policies topic](./policies/policies.html#how-are-policies-implemented).

### Channel Modification Policies

Modification policies specify the group of identities required to sign (approve) any channel configuration update.
It is the policy that defines how a channel policy is updated.
Thus, each channel configuration element includes a reference to a policy which governs its modification.

For more information see the [Modification Policies topic](./policies/policies.html#modification-policies).

### Access Control Lists

Access Control Lists (ACLs) provide the ability to configure access to channel resources by associating those resources with existing policies.

For more information see the [Access Control Lists (ACLs) topic](./access_control.html).

### Chaincode Lifecycle Policy

The number of organizations that need to approve a chaincode definition before it can be successfully committed to a channel is governed by the channel’s LifecycleEndorsement policy.

For more information see the [Chaincode Lifecycle topic](./chaincode_lifecycle.html).

### Chaincode Endorsement Policies

Every smart contract inside a chaincode package has an endorsement policy that specifies how many peers belonging to different channel members need to execute and validate a transaction against a given smart contract in order for the transaction to be considered valid.
Hence, the endorsement policies define the organizations (through their peers) who must “endorse” (i.e., sign) the execution of a proposal.

For more information see the [Endorsement policies topic](./policies/policies.html#chaincode-endorsement-policies).

## Peers

Peers are a fundamental element of the network because they host ledgers and smart contracts.
Peers have an identity of their own, and are managed by an administrator of an organization.

For more information see the [Peers and Identity topic](./peers/peers.html#peers-and-identity) and [Peer Deployment and Administration topic](./deploypeer/peerdeploy.html).

## Ordering service nodes

Ordering service nodes order transactions into blocks and then distribute blocks to connected peers for validation and commit.
Ordering service nodes have an identity of their own, and are managed by an administrator of an organization.

For more information see the [Ordering Nodes and Identity topic](./orderer/ordering_service.html#orderer-nodes-and-identity) and [Ordering Node Deployment and Administration topic](./deployorderer/ordererdeploy.html).

## Tranport Layer Security (TLS)

Fabric supports secure communication between nodes using Transport Layer Security (TLS).
TLS communication can use both one-way (server only) and two-way (server and client) authentication.

For more information see the [Transport Layer Security (TLS) topic](./enable_tls.html).

## Peer and Ordering service node operations service

The peer and the orderer host an HTTP server that offers a RESTful “operations” API.
This API is unrelated to the Fabric network services and is intended to be used by operators, not administrators or “users” of the network.

As the operations service is focused on operations and intentionally unrelated to the Fabric network, it does not use the Membership Services Provider for access control.
Instead, the operations service relies entirely on mutual TLS with client certificate authentication.

For more information see the [Operations Service topic](./operations_service.html).

## Hardware Security Modules

The cryptographic operations performed by Fabric nodes can be delegated to a Hardware Security Module (HSM).
An HSM protects your private keys and handles cryptographic operations, allowing your peers to endorse transactions and orderer nodes to sign blocks without exposing their private keys.

Fabric currently leverages the PKCS11 standard to communicate with an HSM.

For more information see the [Hardware Security Module (HSM) topic](./hsm.html).

## Fabric Applications

A Fabric application can interact with a blockchain network by submitting transactions to a ledger or querying ledger content.
An application interacts with a blockchain network using one of the Fabric SDKs.

The Fabric v2.x SDKs only support transaction and query functions and event listening.
Support for administrative functions for channels and nodes has been removed from the SDKs in favor of the CLI tools.

Applications typically reside in a managed tier of an organization's infrastructure.
The organization may create client identities for the organization at large, or client identities for individual end users of the application.
Client identities only have permission to submit transactions and query the ledger, they do not have administrative or operational permissions on channels or nodes.

In some use cases the application tier may persist user credentials including the private key and sign transactions.
In other use cases end users of the application may want to keep their private key secret.
To support these use cases, the Node.js SDK supports offline signing of transactions.
In both cases, a Hardware Security Module can be used to store private keys meaning that the client application does not have access to them.

Regardless of application design, the SDKs do not have any privileged access to peer or orderer services other than that provided by the client identity.
From a security perspective, the SDKs are merely a set of language specific convenience functions for interacting with the gRPC services exposed by the Fabric peers and orderers.
All security enforcement is carried out by Fabric nodes as highlighted earlier in this topic, not the client SDK.

For more information see the [Applications topic](./developapps/application.html) and [Offline Signing tutorial](https://hyperledger.github.io/fabric-sdk-node/release-2.2/tutorial-sign-transaction-offline.html).

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
