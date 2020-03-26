# Channel capabilities

**Audience**: Channel administrators, node administrators

*Note: this is an advanced Fabric concept that is not necessary for new users or application developers to understand. However, as channels and networks mature, understanding and managing capabilities becomes vital. Furthermore, it is important to recognize that updating capabilities is a different, though often related, process to upgrading nodes. We'll describe this in detail in this topic.*

Because Fabric is a distributed system that will usually involve multiple organizations, it is possible (and typical) that different versions of Fabric code will exist on different nodes within the network as well as on the channels in that network. Fabric allows this --- it is not necessary for every peer and ordering node to be at the same version level. In fact, supporting different version levels is what enables rolling upgrades of Fabric nodes.

What **is** important is that networks and channels process things in the same way, creating deterministic results for things like channel configuration updates and chaincode invocations. Without deterministic results, one peer on a channel might invalidate a transaction while another peer may validate it.

To that end, Fabric defines levels of what are called "capabilities". These capabilities, which are defined in the configuration of each channel, ensure determinism by defining a level at which behaviors produce consistent results. As you'll see, these capabilities have versions which are closely related to node binary versions. Capabilities enable nodes running at different version levels to behave in a compatible and consistent way given the channel configuration at a specific block height. You will also see that capabilities exist in many parts of the configuration tree, defined along the lines of administration for particular tasks.

As you'll see, sometimes it is necessary to update your channel to a new capability level to enable a new feature.

## Node versions and capability versions

If you're familiar with Hyperledger Fabric, you're aware that it follows a typical versioning pattern: v1.1, v1.2.1, v2.0, etc. These versions refer to releases and their related binary versions.

Capabilities follow the same versioning convention. There are v1.1 capabilities and v1.2 capabilities and 2.0 capabilities and so on. But it's important to note a few distinctions.

* **There is not necessarily a new capability level with each release**.
  The need to establish a new capability is determined on a case by case basis and relies chiefly on the backwards compatibility of new features and older binary versions. Adding Raft ordering services in v1.4.1, for example, did not change the way either transactions or ordering service functions were handled and thus did not require the establishment of any new capabilities. [Private Data](./private-data/private-data.html), on the other hand, could not be handled by peers before v1.2, requiring the establishment of a v1.2 capability level. Because not every release contains a new feature (or a bug fix) that changes the way transactions are processed, certain releases will not require any new capabilities (for example, v1.4) while others will only have new capabilities at particular levels (such as v1.2 and v1.3). We'll discuss the "levels" of capabilities and where they reside in the configuration tree later.

* **Nodes must be at least at the level of certain capabilities in a channel**.
  When a peer joins a channel, it reads all of the blocks in the ledger sequentially, starting with the genesis block of the channel and continuing through the transaction blocks and any subsequent configuration blocks. If a node, for example a peer, attempts to read a block containing an update to a capability it doesn't understand (for example, a v1.4.x peer trying to read a block containing a v2.0 application capability), **the peer will crash**. This crashing behavior is intentional, as a v1.4.x peer should not attempt validate or commit any transactions past this point. Before joining a channel, **make sure the node is at least the Fabric version (binary) level of the capabilities specified in the channel config relevant to the node**. We'll discuss which capabilities are relevant to which nodes later. However, because no user wants their nodes to crash, it is strongly recommended to update all nodes to the required level (preferably, to the latest release) before attempting to update capabilities. This is in line with the default Fabric recommendation to **always** be at the latest binary and capability levels.

If users are unable to upgrade their binaries, then capabilities must be left at their lower levels. Lower level binaries and capabilities will still work together as they're meant to. However, keep in mind that it is a best practice to always update to new binaries even if a user chooses not to update their capabilities. Because capabilities themselves also include bug-fixes, it is always recommended to update capabilities once the network binaries support them.

## Capability configuration groupings

As we discussed earlier, there is not a single capability level encompassing an entire channel. Rather, there are three capabilities, each representing an area of administration.

* **Orderer**: These capabilities govern tasks and processing exclusive to the ordering service. Because these capabilities do not involve processes that affect transactions or the peers, updating them falls solely to the ordering service admins (peers do not need to understand orderer capabilities and will therefore not crash no matter what the orderer capability is updated to). Note that these capabilities did not change between v1.1 and v1.4.2. However, as we'll see in the **channel** section, this does not mean that v1.1 ordering nodes will work on all channels with capability levels below v1.4.2.

* **Application**: These capabilities govern tasks and processing exclusive to the peers. Because ordering service admins have no role in deciding the nature of transactions between peer organizations, changing this capability level falls exclusively to peer organizations. For example, Private Data can only be enabled on a channel with the v1.2 (or higher) application group capability enabled. In the case of Private Data, this is the only capability that must be enabled, as nothing about the way Private Data works requires a change to channel administration or the way the ordering service processes transactions.

* **Channel**: This grouping encompasses tasks that are **jointly administered** by the peer organizations and the ordering service. For example, this is the capability that defines the level at which channel configuration updates, which are initiated by peer organizations and orchestrated by the ordering service, are processed. On a practical level, **this grouping defines the minimum level for all of the binaries in a channel, as both ordering nodes and peers must be at least at the binary level corresponding to this capability in order to process the capability**.

The **orderer** and **channel** capabilities of a channel are inherited by default from the ordering system channel, where modifying them are the exclusive purview of ordering service admins. As a result, peer organizations should inspect the genesis block of a channel prior to joining their peers to that channel. Although the channel capability is administered by the orderers in the orderer system channel (just as the consortium membership is), it is typical and expected that the ordering admins will coordinate with the consortium admins to ensure that the channel capability is only upgraded when the consortium is ready for it.

Because the ordering system channel does not define an **application** capability, this capability must be specified in the channel profile when creating the genesis block for the channel.

**Take caution** when specifying or modifying an application capability. Because the ordering service does not validate that the capability level exists, it will allow a channel to be created (or modified) to contain, for example, a v1.8 application capability even if no such capability exists. Any peer attempting to read a configuration block with this capability would, as we have shown, crash, and even if it was possible to modify the channel once again to a valid capability level, it would not matter, as no peer would be able to get past the block with the invalid v1.8 capability.

For a full look at the current valid orderer, application, and channel capabilities check out a [sample `configtx.yaml` file](http://github.com/hyperledger/fabric/blob/{BRANCH}/sampleconfig/configtx.yaml), which lists them in the "Capabilities" section.

For more specific information about capabilities and where they reside in the channel configuration, check out [defining capability requirements](capability_requirements.html).

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
