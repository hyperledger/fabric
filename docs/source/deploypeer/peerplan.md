# Planning for a production peer

Audience: Architects, network operators, users setting up a production Fabric network who are familiar with Transport Layer Security (TLS), Public Key Infrastructure (PKI) and Membership Service Providers (MSPs).

**Note: "chaincode" refers to the packages that are installed on peers, while "smart contracts" refers to the business logic that is agreed to by organizations.**

Peer nodes are a fundamental element of a Fabric network because they host ledgers and smart contracts that are used to encapsulate the shared processes and shared information in a blockchain network. These instructions assume you are already familiar with the concept of a [peer](../peers/peers.html) and provides guidance for the various decisions you will have to make about a peer you will deploy and join to a production Fabric network channel. If you need to quickly stand up a network for education or testing purposes, check out the [Fabric test network](../test_network.html).

## Generate peer identities and Membership Service Providers (MSPs)

Before proceeding with this topic, you should have reviewed the process for a [Deploying a Certificate Authority (CA)](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/ca-deploy-topology.html) for your organization in order to generate the identities and MSPs for the admins and peers in your organization. To learn how to use a CA to create these identities, check out [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html)

**Note that the “cryptogen” tool should never be used to generate any identities in a production scenario**.

### Folder management

While it is possible to bootstrap a peer using a number of folder structures for your MSPs and certificates, we do recommend a particular [folder structure](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html#folder-structure-for-your-org-and-node-admin-identities) for the sake of consistency and repeatability. These instructions will presume that you have used that folder structure.

### Certificates from a non-Fabric CA

While it is possible to use a non-Fabric CA to generate identities, this process requires that you manually construct the MSP folders the peer needs to be deployed. That process will not be covered here and will instead focus on using a Fabric CA to generate the identities and MSP folders for you.

### Transport Layer Security (TLS) enablement

To prevent "man in the middle" attacks and otherwise secure communications, using TLS is a requirement for any production network. Therefore, in addition to registering your peer identities with your organization CA, you will also need to register your peer identities with the TLS CA for the organization. These TLS certificates will be used by the peer when communicating with the network.

## State database

Each peer maintains a state database that tracks the current value for all of the assets (also known as "keys") listed on the ledger. Two types of state databases are supported: External CouchDB (which allows JSON queries of the database) or embedded Goleveldb (which does not). The choice of database largely depends on whether you need the CouchDB JSON query support. If JSON query is not needed, Goleveldb improves performance and requires less management since it is embedded in the peer process. Because all of the peers on a channel must use the same state database, your choice of database might already be dictated by the channels you wish to join.

Beyond the ability to execute JSON queries when using CouchDB, the choice of the database is invisible to a smart contract.

You can review [State Database options](../couchdb_as_state_database.html#state-database-options) for more details.

## Sizing your peer resources

A peer typically has multiple containers associated with it.

* **Peer container**: Encapsulates the peer process that validates and commits transactions for all channels a peer belongs to. The peer storage includes each channel's blockchain (in other words, the transaction history), local databases including the state database if using Goleveldb, and any chaincodes that are installed on the peer. The size of peer storage depends on the number of channels, and the number and size of transactions in each channel.

* **CouchDB container** (optional): If using CouchDB as the state database, the CouchDB container will be used to store the state database of each channel.

* **Chaincode launcher container** (optional): Used to launch a separate container for each chaincode, eliminating the need for a Docker-in-Docker container in the peer container. Note that the chaincode launcher container is not where smart contracts actually run, and is therefore given a smaller default resource than the "smart contracts" container that used to be deployed along with a peer. It only exists to help create the containers where a smart contract will run. You must make your own allowances in your cluster for the containers for the chaincodes deployed by the launcher.

* **Chaincode container**: The container where the chaincode runs. Note that the recommended process is to deploy each chaincode into a separate container, even if you have multiple peers on the same channel that have all installed the same chaincode. So if you have three peers on a channel, and install a smart contract on each one, you will have three smart contract containers running. However, if these three peers are on more than one channel using the exact same smart contract, you will still only have three pods running.

## Storage considerations

Chaincodes and the ledger (one for each channel) are physically stored on a peer according to the `peer.fileSystemPath` parameter, while identities and MSP are stored according to the `peer.mspConfigPath` parameter (by default, both locations are at `/var/hyperledger/production`). **This file system needs to be protected, secured, and writable by authorized users only** and should also be regularly backed up. Note that the best practice is to use externally mounted volumes for both of these parameters, as they will therefore be easy to reference when restarting or upgrading the peer.

When you configure your peer, you need to decide if the state database will be stored in CouchDB or LevelDB (default) by configuring the `ledger.state.stateDatabase` parameter.

While this topic is focused on how to use the peer binary images, there are important storage considerations you need to be aware of when you run the Fabric images in Docker containers or use Kubernetes. Docker containers requires a volume bind mount that mounts the external folder pathing to your container. This is critical when the container restarts, so that the storage is not lost. Similarly, if you are using Kubernetes, you need to provision storage for the peer and then map it in your Kubernetes pod deployment YAML file.

## High Availability

As part of planning to create a peer, you will need consider your strategy at an organization level in order to ensure zero downtime of your components. This means building redundant components, and specifically redundant peers. To ensure zero downtown, you need at least one redundant peer **in a separate virtual machine** so that peers can go down for maintenance while client applications go on submitting endorsement proposals uninterrupted.

Along similar lines, client applications should be configured to use Service Discovery to ensure that transactions are only submitted to peers that are currently available. As long as at least one peer from each organization is available, and service discovered is being used, any endorsement policy will be able to be satisfied. It is the responsibility of each organization to make sure their high availability strategy is robust enough to ensure that at least one peer owned by their organization is available at all times in every channel they're joined to.

## Monitoring

All blockchain nodes require careful monitoring, but it is critically important to monitor the peer and ordering nodes. By virtue of being immutable, the ledger inevitably grows. As a result, storage must be monitored and extended as needed. If the storage for a peer is exhausted you also have the option to deploy a new peer with a larger storage allocation and let the ledger sync. In a production environment you should also monitor the CPU and memory allocated to a peer using widely available tooling. If you see the peer struggling to keep up with the transaction load or when performing relatively simple tasks (querying the ledger, for example), it is a sign that you might need to increase its resource allocation.

## Chaincode

Prior to Hyperledger Fabric 2.0, the process used to build and launch chaincode was part of the peer implementation and could not be easily customized. All chaincode installed on the peer would be “built” using language specific logic hard coded in the peer. This build process would generate a Docker container image that would be launched to execute chaincode that connected as a client to the peer.

This approach limited chaincode implementations to a handful of languages, required Docker to be part of the deployment environment, prevented running chaincode as a long-running server process, and required that the peer have privileged access to the chaincode container.

Starting with Fabric 2.0, External Builders and Launchers enable operators to extend the peer with programs that can build, launch, and discover chaincode. To leverage this capability on peers that already exist you will need to create your own buildpack and then modify `core.yaml` to include a new externalBuilder configuration element which lets the peer know an external builder is available.

## Gossip

Peers leverage the [gossip data dissemination protocol](../gossip.html) to broadcast ledger and channel data in a scalable fashion. Gossip messaging is continuous, and each peer on a channel is constantly receiving data from multiple peers, including peers in other organizations (if cross-organization gossip is enabled).

For peer gossip to work you need to configure four parameters. Three of them --- `peer.gossip.bootstrap`, `peer.gossip.endpoint`, `peer.gossip.externalEndpoint` --- are in the peer’s `core.yaml` file. The fourth enables gossip between organization by specifying an anchor peer in the channel configuration.

To reduce network traffic, in Fabric v2.2 the default core.yaml is configured for peers to pull blocks from the ordering service instead of through gossip dissemination among peers (with the exception of private data, which are still sent from peer to peer using gossip). To get all blocks from the orderer, you must use the following parameters in the `core.yaml` file:

* `peer.gossip.useLeaderElection = false`
* `peer.gossip.orgLeader = true`
* `peer.gossip.state.enabled = false`

If all peers have `orgLeader=true` (recommended), then each peer will get blocks from the ordering service.

### Service Discovery

In any network it is possible that peer nodes can be down for maintenance, unreachable due to network issues, or the peer ledger has fallen behind while being offline. For this reason, Fabric includes a “discovery service” that enables client applications that use the SDK to locate good candidate peers to target with endorsement requests. If service discovery is not enabled, when a client application targets a peer that is offline, the request fails and will need to be resubmitted to another peer. The discovery service runs on peers and uses the network metadata information maintained by the gossip communication layer to find out which peers are online and can be targeted for requests.

Service discovery (and private data) requires that gossip is enabled, therefore you should configure the `peer.gossip.bootstrap`, `peer.gossip.endpoint` , and `peer.gossip.externalEndpoint` parameters, as well as anchor peers on each channel, to take advantage of this feature.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
