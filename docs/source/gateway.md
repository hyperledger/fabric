# Fabric Gateway

Fabric Gateway (the gateway) is a service, introduced in Hyperledger Fabric v2.4, which provides a simplified, minimal API for submitting transactions to a Fabric network. Requirements previously placed on the client application, such as managing transaction endorsements, are delegated to a gateway peer to provide simplified application development and transaction processing in v2.4.

## Writing client applications

Starting with Fabric v2.4, client applications should use a gateway SDK (Go, Node, or Java), which are optimized to interact with Fabric Gateway. The gateway SDKs expose the same high-level programming model which was initially introduced in Fabric v1.4.

The gateway manages the following transaction steps:

- **Evaluate** a transaction proposal. This invokes a smart contract (chaincode) function on a single peer and returns the result to the client. This is typically used to query the current state of the ledger without making any ledger updates.
The gateway will preferably select a peer in the same organization as the gateway peer and choose the peer
with the highest ledger block height. If no peer is available in the gateway's organization, then it will choose a peer
from another organization.
- **Endorse** a transaction proposal. This collects endorsement responses, which must satisfy the combined signature policies (see [below](#how-the-gateway-endorses-your-transaction-proposal)) and return a prepared transaction envelope to the client for signing.
- **Submit** a transaction. This sends a signed transaction envelope to the ordering service to commit to the ledger.
- Wait for **commit status** events. This allows the client to wait for the transaction to be committed to the ledger and to
get the commit (validation/invalidation) status code.
- Receive **chaincode events**. This allows client applications to respond to events that are emitted by a smart contract
function when a transaction is committed to the ledger.

The gateway SDKs combine the Endorse/Submit/CommitStatus actions into a single blocking **SubmitTransaction** function to support transaction submission with a single line of code. Alternatively, the individual actions can be invoked.

## Software Development Kits (SDKs)

Fabric Gateway and its SDKs are designed to allow you, as a client application developer, to concentrate on the *business logic* of your application without having to concern yourself with the *infrastructure logic* associated with a Fabric network. As such, the APIs provide logical abstractions such as *organization* and *contract* rather than operational abstractions such as *peer* and *chaincode*. (The Fabric Gateway API is not a system admin API which would expose these operational abstractions.)

Hyperledger Fabric currently supports client application development in three languages:

- **Go**.  See the [Go SDK for Fabric Gateway documentation](https://pkg.go.dev/github.com/hyperledger/fabric-gateway/pkg/client) for full details.
- **Node (Typescript/Javascript)**.  See the [Node SDK for Fabric Gateway documentation](https://hyperledger.github.io/fabric-gateway/main/api/node/) for full details.
- **Java**. See the [Java SDK for Fabric Gateway documentation](https://hyperledger.github.io/fabric-gateway/main/api/java/) for full details.

## How the gateway endorses your transaction proposal

For a transaction to be successfully committed to the ledger, a sufficient number of endorsements, from the required organizations, must be received to satisfy the [endorsement policy](endorsement-policies.html). Getting an endorsement from an organization involves connecting to one of its peers to have it simulate (execute) the transaction proposal against its copy of the ledger. The peer simulates the transaction by invoking the chaincode function, as specified by its name and arguments in the proposal, and building (and signing) a read-write set. The read-write set contains the current ledger state and proposed changes in response to the state get/set instructions in that function.

The endorsement policy, or sum of endorsement policies, that gets applied to a transaction depends on the implementation of the chaincode function that is being invoked, and could be a combination of the following:

- **Chaincode endorsement policies**. These are the policies agreed to by channel members when they approve a chaincode definition for their organization. If the chaincode function invokes a function in another chaincode, then both policies must be satisfied.
- **Private data collection endorsement policies**. If the chaincode function writes to a state in a private data collection, then the endorsement policy for that collection will override the chaincode policy for that state. If the chaincode function reads from a private data collection, it will be restricted to organizations that are members of that collection.
- **State-based endorsement (SBE) policies**. Also known as key-level signature policies, these can be applied to individual states and will override the chaincode policy or collection policy for private data collection states. The endorsement policies are stored in the ledger and can be updated by new transactions.

The combination of endorsement policies to be applied to the transaction proposal is determined at chaincode runtime and cannot necessarily be derived from static analysis.

The Fabric Gateway manages the complexity of transaction endorsement on behalf of the client, using the following process:

- Fabric Gateway selects the endorsing peer from the gateway peer's organization (MSPID) by identifying the (available) peer with the highest ledger block height. The assumption is that all peers within the gateway peer's organization are *trusted* by the client application that connects to the gateway peer.
- The transaction proposal is simulated on the selected endorsement peer. This simulation captures information about the accessed states, and therefore the endorsement policies to be combined (including any individual state-based policies stored on the endorsement peer's ledger).  
- The captured policy information is assembled into a `ChaincodeInterest` protobuf structure and passed to the discovery service in order to derive an endorsement plan specific to the proposed transaction.
- The gateway applies the endorsement plan by requesting endorsement from the organizations required to satisfy all policies in the plan. For each organization, the gateway peer requests endorsement from the (available) peer with the highest block height.

The gateway is dependent on the [discovery service](discovery-overview.html) to get the connection details of both the available peers and ordering service nodes, and for calculating the combination of peers that are required to endorse the transaction proposal. The discovery service must therefore always remain enabled on peers where the gateway service is enabled.

### Endorsing private data

The gateway endorsement process is more restrictive for private data, passed in a transaction proposal as transient data, because it often contains sensitive or personal information that must never be shared with all organizations. In this case, the gateway will restrict the set of endorsing organizations to only members of the private data collection to be accessed (either read or write). If this restriction for private data would not satisfy the endorsement policy, the gateway returns an error to the client and does not submit the transaction to any organization for endorsement. In this case, the client application must explicitly [specify the organizations](#specifying-endorsement-organizations) to request endorsement from.

### Specifying endorsement organizations

A client application can explicitly specify the endorsing organizations to meet both the privacy and endorsement requirements of the application; the gateway will then select the (available) endorsing peer with the highest block count in each specified organization. If the client specifies a set of organizations that does not satisfy an endorsement policy, the transaction can still receive endorsement from the specified peers and be submitted for ordering. However, the transaction will ultimately be rejected by all peers in the channel, during the validation and commit phase. The invalidated transaction is recorded on the channel ledger, but its results are not written to the state database on any peer.

### Retry and error handling

Fabric Gateway handles node connectivity retry attempts, errors, and timeouts as described below.

#### Retry attempts

The gateway will use discovery service information to retry any transaction that fails due to an unavailable peer or ordering node. If an organization is running multiple peer or ordering nodes, then another qualifying node will be attempted. If an organization fails to endorse a transaction proposal, then another one will be selected. If an organization fails to endorse entirely, a group of organizations that satisfies the endorsement policy will be targeted. Only if there is no combination of available peers that satisfies the endorsement policy will the gateway stop retrying. The gateway will continue with retry attempts until all possible combinations of endorsing peers have been tried once.

#### Error handling

The Fabric Gateway manages gRPC connections to network peer and ordering nodes. If a gateway service request error originates from a network peer or ordering node (i.e. external to the gateway service), the gateway returns error, endpoint, and organization (MSPID) information to the client in the message `Details` field. If the `Details` field is empty, then the error originated from the gateway peer.

#### Timeouts

The Fabric Gateway `Evaluate` and `Endorse` methods make gRPC requests to peers external to the gateway. In order to limit the length of time that the client must wait for these collective responses, the `peer.gateway.endorsementTimeout` value can be overridden in the gateway section of the peer `core.yaml` configuration file.

Each Gateway SDK also provides a mechanism for setting timeouts for each gateway method when invoked from the client application.

## Listening for events

The gateway provides a simplified API for client applications to receive [chaincode events](peer_event_services.html#how-to-register-for-events) in the client applications. Each SDK provides a mechanism to handle these events using its language-specific idiom.
