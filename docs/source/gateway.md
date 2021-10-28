# Gateway

## Overview

The Gateway is a new service introduced in v2.4 that provides a simplified, minimal API for submitting transactions
to a Fabric network.

## Writing client applications

Starting with v2.4, client applications should use one of the v2.4 SDKs which are optimised to interact with the Gateway.
These SDKs expose the same high-level programming model which was first introduced with v1.4 of Fabric.

The gateway supports the following actions
- **Evaluate** a transaction proposal.  This will invoke a smart contract (chaincode) function on a single peer and return the result
to the client.  This is typically used to query the current state of the ledger without making any ledger updates.
The gateway will preferably select a peer in the same organization as the gateway peer and choose the peer 
with the highest ledger block height.  If no peer is available in the gateway's organization, then it will choose a peer
from another organization.
- **Endorse** a transaction proposal. This will gather enough endorsement responses to satisfy the combined signature policies 
(see [below](#how-the-gateway-endorses-your-transaction-proposal)) and
return a prepared transaction envelope to the client for signing.
- **Submit** a transaction.  This will send a signed transaction envelope to the ordering service to commit to the ledger.
- Wait for **commit status** events.  This allows the client to wait for the transaction to be committed to the ledger and to
get the commit (validation/invalidation) status code.
- Receive **chaincode events**. This will allow client applications to respond to events that are emitted by a smart contract
function when a transaction is committed to the ledger.

The gateway SDKs combine the Endorse/Submit/CommitStatus actions into a single blocking SubmitTransaction function to support transaction
submission with a single line of code.  Alternatively, the individual actions can be invoked to support flexible application patterns.

## Software Development Kits (SDKs)

The Gateway and its SDKs are designed to allow you, as a client application developer, to concentrate on the *business logic*
of your application without having to concern yourself with the *infrastructure logic* associated with a Fabric network.
As such, the APIs provide logical abstractions such as *organization* and *contract* rather than operational abstractions
such as *peer* and *chaincode*. [Side note - clearly an admin API would want to expose these operational abstractions, 
but this is *not* an admin API].

Hyperledger Fabric currently supports client application development in three languages:

- **Go**.  See the [Go SDK documentation](https://pkg.go.dev/github.com/hyperledger/fabric-gateway/pkg/client) for full details.
- **Node (Typescript/Javascript)**.  See the [Node SDK documentation](https://hyperledger.github.io/fabric-gateway/main/api/node/) for full details.
- **Java**. See the [Java SDK documentation](https://hyperledger.github.io/fabric-gateway/main/api/java/) for full details.

## How the gateway endorses your transaction proposal

In order for a transaction to be successfully committed to the ledger, a sufficient number of endorsements are required in order to satisfy
the [endorsement policy](endorsement-policies.html).  Getting an endorsement from an organization involves connecting to one 
of its peers and have it simulate (execute) the transaction proposal against its copy of the ledger.  The peer does this by
invoking the chaincode function whose name and arguments are specified in the proposal and building (and signing) a read-write set
containing the current ledger state and changes it would make in response to the state get/set instructions in that function.

The endorsement policy that gets applied to a transaction depends on the implementation of the chaincode function that 
is being invoked, and could be a combination of the following:

- **Chaincode policies**.  These are the policies agreed to by channel members when they approve a chaincode definition for their organization.
If the chaincode function invokes a function in another chaincode, then both policies will need to be satisfied.
- **Private data collection policies**.  If the chaincode function writes to a state in a private data collection,
then the endorsement policy for that collection will override the chaincode policy for that state.  If it reads from a private
collection, then it will be restricted to organizations that are members of that collection.
- **State-based endorsement (SBE) policies**. Also known as key-level signature policies, these can be applied to individual
states and will override the chaincode policy (or collection policy if it's a private collection state).
The policies themselves are stored in the ledger and can be modified by transactions.

Although it might seem obvious from the transaction proposal which policies would be applied, the actual combination of
polices depends on what the chaincode function does at runtime and cannot necessarily be derived from static analysis.

The gateway handles all of this complexity on behalf of the client using the following process:

- The gateway selects the endorsing peer in the gateway's organization (MSPID) that has the highest ledger block height.
The assumption is that all peers within this organization are *trusted* by the client application that connects
to this gateway.
- It simulates the transaction proposal on that peer.  During simulation, it captures all the relevant information on which
states are accessed and hence which policies will need to be combined (including any individual state-based policies 
as stored in the peer's copy of the ledger).  
- This information is assembled into a `ChaincodeInterest` protobuf structure and passed to the discovery service in order
to derive an endorsement plan specific for this transaction.
- Using the endorsement plan, the gateway will connect to and request endorsement from other organizations' peers to satisfy
the overall endorsement policy.  It will generally request endorsement from the peer with the highest block height for any
given organization. 
  
The gateway is dependent on the [discovery service](discovery-overview.html) to get the connection details of available peers
and ordering service nodes in a network, as well as for calculating the various permutations of peers that are required for 
endorsement.  It is important, therefore, that the discovery service has not been disabled in a gateway peer.

If the transaction proposal contains transient data, then the gateway takes a more cautious approach.
Transient data is often used to carry sensitive or personal information that must not persist in the ledger.
A typical usage pattern involves sending the proposal to specific organizations which write the data to their private collections.
The gateway will restrict the set of endorsing organizations to those that are members of the collections involved.
If this restriction does not satisfy the endorsement policy, it will return an error to the client in preference
to potentially leaking sensitive data.  In this situation, the client should [explicitly define which organizations should 
endorse](#how-to-override-endorsement-policy-detection) the transaction.

### How to override endorsement policy detection

There may be situations where the client application needs to explicitly define which organizations should be targeted to
evaluate or endorse a transaction proposal.  One such example is when sensitive data is being passed in the transient field,
and the client wants to be *absolutely sure* which organizations will receive it.  In these situations the client application
can specify the endorsing organizations. The gateway will select a peer from each of these (bypassing the mechanism described above)
and build a transaction using these endorsements.  It's important to note that if this set of organizations does not satisfy
the endorsement policy, then the transaction will be included in a block but invalidated by all peers, and the 
transaction's writes will not be updated in the state database.

### Error and retry handling

#### Retry
The gateway, using information from the discovery service, will make every effort to evaluate or submit a transaction in a network
that could suffer from peers or ordering nodes being unavailable for any reason.  If a peer fails, and its organization is 
running multiple peers, then another one will be selected.  If an organization fails to endorse entirely, then another
set of organizations which also satisfies the endorsement policy will be targeted.  Only if there is no combination of available peers
that satisfy the endorsement policy will the gateway stop retrying.  The gateway will not retry the same peer for 
any given transaction proposal.

#### Errors
The gateway manages gPRC connections to other peers and ordering nodes in the network.  If the failure of a gateway service
request is due to an error returned from one or more of these external nodes, then the details of this error, together with
endpoint address and MSPID of the failing node will be included in the `Details` field of the error returned from the gateway
to the client.  If the `Details` field is empty, then the error would have originated in the gateway.

#### Timeouts

The gateway `Evaluate` and `Endorse` methods involve making gRPC requests to external peers.  In order to limit the length 
of time that the client must wait for these collective responses, a configurable `EndorsementTimeout` is available
to override in the gateway section of the [peer config file](https://github.com/hyperledger/fabric/blob/main/sampleconfig/core.yaml#L55).

Each SDK also provides a mechanism for setting timeouts for each of the gateway methods when invoked from the client application.

## How to listen for events

The gateway provides a simplified API for receiving [chaincode events](peer_event_services.html#how-to-register-for-events) 
in the client applications.  Each SDK provides a mechanism to handle these events using its language-specific idiom.
