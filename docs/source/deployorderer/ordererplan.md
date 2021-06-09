# Planning for an ordering service

Audience: Architects, network operators, users setting up a production Fabric network who are familiar with Transport Layer Security (TLS), Public Key Infrastructure (PKI) and Membership Service Providers (MSPs).

Check out the conceptual topic on [The Ordering Service](../orderer/ordering_service.html) for an overview on ordering service concepts, implementations, and the role an ordering service plays in a transaction.

In a Hyperledger Fabric network, a node or collection of nodes together form what's called an "ordering service", which literally orders transactions into blocks, which peers will then validate and commit to their ledgers. This separates Fabric from other distributed blockchains, such as Ethereum and Bitcoin, in which this ordering is done by any and all nodes.

Whereas Fabric networks that will only be used for testing and development purposes (such as our [test network](../test_network.html)) often feature an ordering service made up of only one node (these nodes are typically referred to as "orderers" or "ordering nodes"), production networks require a more robust deployment of at least three nodes. For this reason, our deployment guide will feature instructions on how to create a three-node ordering service. For more guidance on the number of nodes you should deploy, check out [Cluster considerations](#cluster-considerations).

This tutorial assumes that a system channel genesis block will not be used when bootstrapping the orderer. Instead, these nodes (or a subset of them), will be joined to a channel using the process to [Create a channel](../create_channel/create_channel_participation.html). For information on how to create an orderer that will be bootstrapped with a system channel genesis block, check out [Deploy the ordering service](https://hyperledger-fabric.readthedocs.io/en/release-2.2/deployorderer/ordererdeploy.html) from the Fabric v2.2 documentation.

## Generate ordering node identities and Membership Service Providers (MSPs)

Before proceeding with this topic, you should have reviewed the process for a Deploying a Certificate Authority (CA) for your organization in order to generate the identities and MSPs for the admins and ordering nodes in your organization. To learn how to use a CA to create these identities, check out [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html). Note that the best practice is to register and enroll a separate node identity for each ordering node and to use distinct TLS certificates for each node.

Note that the `cryptogen` tool should never be used to generate any identities in a production scenario.

In this deployment guide, we’ll assume that all ordering nodes will be created and owned by the same orderer organization. However, it is possible for multiple organizations to contribute nodes to an ordering service, both during the creation of the ordering service and after the ordering service has been created.

## Folder management

While it is possible to bootstrap an ordering node using a number of folder structures for your MSPs and certificates, we do recommend the folder structure outlined in [Registering and enrolling identities with a CA](https://hyperledger-fabric-ca.readthedocs.io/en/release-1.4/deployguide/use_CA.html#decide-on-the-structure-of-your-folders-and-certificates) for the sake of consistency and repeatability. Although it is not required, these instructions will presume that you have used that folder structure.

## Certificates from a non-Fabric CA

While it is possible to use a non-Fabric CA to generate identities, this process requires that you manually construct the MSP folders the ordering service and its organization need. That process will not be covered here and will instead focus on using a Fabric CA to generate the identities and MSP folders for you.

## Transport Layer Security (TLS) enablement

To prevent “man in the middle” attacks and otherwise secure communications, the use of TLS is a requirement for any production network. Therefore, in addition to registering your ordering nodes identities with your organization CA, you will also need to create certificates for your ordering nodes with the TLS CA of your organization. These TLS certificates will be used by the ordering nodes when communicating with the network.

## Sizing your ordering node resources

Because ordering nodes do not host a state database or chaincode, an ordering node will typically only have a single container associated with it. Like the “peer container” associated with the peer, this container encapsulates the ordering process that orders transactions into blocks for all channels on which the ordering node is a consenter (ordering nodes also validate actions in particular cases). The ordering node storage includes the blockchain for all of the channels on which the node is a consenter.

Note that, at a logical level, every “consenter set” for each channel is a separate ordering service, in which “alive” messages and other communications are duplicated. This affects the CPU and memory required for each node. Similarly, there is a direct relationship between the size of a consenter set and the amount of resources each node will need. This is because in a Raft ordering service, the nodes do not collaborate in ordering transactions. One node, a "leader" elected by the other nodes, performs all ordering and validation functions, and then replicates decisions to the other nodes. As a result, as consenter sets increase in size, there is more traffic and burden on the leader node and more communications across the consenter set.

More on this in [Cluster considerations](#cluster-considerations).

## Cluster considerations

For more guidance on the number of nodes you should deploy, check out [Raft](../orderer/ordering_service.html#raft).

Raft is a leader based protocol, where a single leader validates transactions, orders blocks, and replicates the data out to the followers. Raft works based on the concept of a quorum in which as long as a majority of the Raft nodes are online, the Raft cluster stays available.

On the one hand, the more Raft nodes that are deployed, the more nodes can be lost while maintaining that a majority of the nodes are still available (unless a majority of nodes are available, the cluster will cease to process and create blocks). A five node cluster, for example, can tolerate two down nodes, while a seven node cluster can tolerate three down nodes.

However, more nodes means a larger communication overhead, as the leader must communicate with all of the nodes in order for the ordering service to function properly. If a node thinks it has lost connection with the leader, even if this loss of communication is only due to a networking or processing delay, it is designed to trigger a leader election. Unnecessary leader elections only add to the communications overhead for the leader, progressively escalating the burden on the cluster. And because, each channel an ordering node participates in is, logically, a separate Raft instance, an orderer participating in 100 channels is actually doing 100x the work as an ordering node in a single channel.

For these reasons, Raft clusters of more than a few dozen nodes begin to see noticeable performance degradation. Once clusters reach about 100 nodes, they begin having trouble maintaining quorum. The stage at which a deployment experiences issues is dependent on factors such as networking speeds and other resources available, and there are parameters such as the tick interval which can be used to mitigate the larger communications overhead.

The optimal number of ordering nodes for your ordering service ultimately depends on your use case, your resources, and your topology. However, clusters of three, five, seven, or nine nodes, are the most popular, with no more than about 50 channels per orderer.

## Storage considerations and monitoring

The storage that should be allocated to an ordering node depends on factors such as the expected transaction throughput, the size of blocks, and number of channels the node will be joined to. Your needs will depend on your use case. However, the best practice is to monitor the storage available to your nodes closely. You may also decide to enable an autoscaler, which will allocate more resources to your node, if your infrastructure allows it.

If the storage for an ordering node is exhausted you also have the option to deploy a new node with a larger storage allocation and allow it to sync with the relevant ledgers. If you have several ordering nodes available to use, ensure that each node is a consenter on approximately the same number of channels.

In a production environment you should also monitor the CPU and memory allocated to an ordering node using widely available tooling. If you see an ordering node struggling to keep up (for example, it might be calling for leader elections when none is needed), it is a sign that you might need to increase its resource allocation.

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
