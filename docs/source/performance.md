# Performance considerations

Various Hyperledger Fabric component, configuration, and workflow decisions contribute to the overall performance of your network. Managing these variables, such as the number of channels, chaincode implementations, and transaction policies, can be complex with multiple participating organizations contributing their own hardware and networking infrastructures to the environment. 

This topic walks you through the considerations that can help you optimize performance for your Hyperledger Fabric network. 

## Hardware considerations

Each participating organization can provide hardware for hosting peer and ordering service nodes. Ordering service nodes can be provided by a single organization or by multiple organizations. Because any organization can impact the performance of the entire network (depending on factors such as transaction endorsement policies), each participating organization must provide adequate resources for network services and applications which they deploy.

### Persistent storage

Because a Hyperledger Fabric network performs a high volume of disk I/O, you must use the fastest disk storage that is practicable. If you are using network-attached storage, ensure that you choose the fastest IOPs available.

### Network connectivity

A Hyperledger Fabric network is highly-distributed across multiple nodes and is typically hosted on multiple clusters, in different cloud environments, and even across legal and geographic boundaries. Therefore, high-speed network connectivity between nodes is vital, and a minimum of 1 Gbps should be deployed between all nodes and organizations.

### CPU and memory

The final hardware consideration is the amount of CPU and Memory to allocate to peer and ordering service nodes, and for any CouchDB state databases used for peer nodes. These allocations affect network performance and even node stability, so you must continuously monitor CPU, Memory, Disk Space, and Disk and Network I/O to ensure that you are well within the limits of your allocated resources. Do not wait to approach maximum utilization before increasing node resources - a threshold of  70%-80% usage is a general guideline for maximum workload.

Network performance generally scales with CPU allocated to peer nodes, so providing each peer (and CouchDB if used) with the maximum CPU capacity is recommended. For orderer nodes, a general guideline is 1 CPU with 2 GB of Memory.

As the amount of state data grows, database storage performance can slow down, especially when using CouchDB as the state database. Therefore, you should plan to add more compute resources to your environment over time, while continuously monitoring your network components and adjusting to any thresholds being exceeded.

## Peer considerations

Peer considerations for overall network performance include the number of active channels and node configuration properties. The default peer and orderer configurations, from the peer **core.yaml** and orderer **orderer.yaml** files, are referenced below.

### Number of peers

The more peers that each organization deploys the better the network should perform, because transaction endorsement requests can be load-balanced across each  organization's peers. However, gateway peers select peers for transaction endorsement based on their current block height, so distribution amongst peers eligible for endorsement is not easily predicted. Work done by a single gateway peer can be predictably reduced by selecting a gateway peer from among multiple gateway peers to submit or evaluate a transaction based on a defined policy such as round robin. This selection of a gateway peer could be done via a load balancer, coded into the client application, or by the grpc library used for gateway selection. (Note: these scenarios have **NOT** been formally benchmarked for any performance gains.)

### Number of channels per peer

If a peer has ample CPU and is participating in only one channel, it is not possible to drive the peer beyond 65-70% CPU because of how the peer does serialization and locking internally. If a peer is participating in multiple channels, the peer will consume the available resource as it increases parallelism in block processing.

In general, ensure that there is a CPU core available for each channel that is running at maximum load. As a general guideline, if the load in each channel is not highly-correlated (i.e., channels are not all contending for resources simultaneously), the number of channels can exceed the number of CPU cores.

### Total query limit

A peer limits the total number of records a range or JSON (rich) query will return, in order to avoid larger than expected result sets that could slow down a peer. This limit is configured in the peer's **core.yaml** file:

```yaml

ledger:
  state:
    # Limit on the number of records to return per query
    totalQueryLimit: 100000
```

The Hyperledger Fabric total query limit default is 100000, as set in the sample **core.yaml** file and test docker images. You can increase this default value if necessary, but you should also consider alternative designs that do not require the higher number of query scans.

### Concurrency limits

Each peer node has limits to ensure it is not overwhelmed by excessive concurrent client requests:

```yaml
peer:
# Limits is used to configure some internal resource limits.
    limits:
        # Concurrency limits the number of concurrently running requests to a service on each peer.
        # Currently this option is only applied to endorser service and deliver service.
        # When the property is missing or the value is 0, the concurrency limit is disabled for the service.
        concurrency:
            # endorserService limits concurrent requests to endorser service that handles chaincode deployment, query and invocation,
            # including both user chaincodes and system chaincodes.
            endorserService: 2500
            # deliverService limits concurrent event listeners registered to deliver service for blocks and transaction events.
            deliverService: 2500
            # gatewayService limits concurrent requests to gateway service that handles the submission and evaluation of transactions.
            gatewayService: 500
```

The Peer Gateway Service, first released in v2.4 of Hyperledger Fabric, introduced the `gatewayService` limit with a default of 500. However, this default can restrict network TPS, so you may need to increase this value to allow more concurrent requests.

### CouchDB cache setting

If you are using CouchDB and have a large number of keys being read repeatedly (not via queries), you may choose to increase the peer's CouchDB cache to avoid database lookups:

```yaml

state:
    couchDBConfig:
       # CacheSize denotes the maximum mega bytes (MB) to be allocated for the in-memory state
       # cache. Note that CacheSize needs to be a multiple of 32 MB. If it is not a multiple
       # of 32 MB, the peer would round the size to the next multiple of 32 MB.
       # To disable the cache, 0 MB needs to be assigned to the cacheSize.
       cacheSize: 64
```

## Orderer considerations

The ordering service uses Raft consensus to cut blocks. Factors such as the number of orderers participating in consensus and block cutting parameters will affect overall network performance, as described below.

### Number of orderers

The number of ordering service nodes will impact performance because all orderers contribute to Raft consensus. Using five ordering service nodes will provide two orderer crash fault tolerance (a required majority of three remain available) and is a good starting point. Additional ordering service nodes may reduce network performance. If a single set of ordering service nodes becomes a network bottleneck, you can try deploying a unique set of ordering service nodes for each channel.

### SendBufferSize

Prior to Hyperledger Fabric v2.5, the default SendBufferSize of each orderer node was set to 10, which causes a bottleneck. In Fabric v2.5, this default was changed to 100, which provides improved throughput. The SendBufferSize parameter is configured in **orderer.yaml**:

```yaml
General:
    Cluster:
        # SendBufferSize is the maximum number of messages in the egress buffer.
        # Consensus messages are dropped if the buffer is full, and transaction
        # messages are waiting for space to be freed.
        SendBufferSize: 100
```

### Block cutting parameters in channel configuration

Channel configuration settings can also affect performance of the ordering service. Increasing the channel block size and timeout parameters can increase throughput but can also increase latency. As described below, you can configure the ordering service for increased transactions per block and longer block cutting times to test the performance results. 

Three orderer batch size parameters - Max Message Count, Absolute Max Bytes, and Preferred Max Bytes - perform block cutting based on the specified block size and maximum number of transactions. These parameters are set when you create or update a channel configuration. If you use **configtxgen** and **configtx.yaml** as a starting point for creating channels, the following section in **configtx.yaml** applies:

```yaml
Orderer: &OrdererDefaults
    # Batch Timeout: The amount of time to wait before creating a batch.
    BatchTimeout: 2s

    # Batch Size: Controls the number of messages batched into a block.
    # The orderer views messages opaquely, but typically, messages may
    # be considered to be Fabric transactions. The 'batch' is the group
    # of messages in the 'data' field of the block. Blocks will be a few KB 
    # larger than the batch size, when signatures, hashes, and other metadata
    # is applied.
    BatchSize:

        # Max Message Count: The maximum number of messages to permit in a
        # batch. No block will contain more than this number of messages.
        MaxMessageCount: 500

        # Absolute Max Bytes: The absolute maximum number of bytes allowed for
        # the serialized messages in a batch. The maximum block size is this value
        # plus the size of the associated metadata (usually a few KB depending
        # upon the size of the signing identities). Any transaction larger than
        # this value will be rejected by ordering. It is recommended not to exceed 
        # 49 MB, given the default grpc max message size of 100 MB configured on 
        # orderer and peer nodes (and allowing for message expansion during communication).
        AbsoluteMaxBytes: 10 MB

        # Preferred Max Bytes: The preferred maximum number of bytes allowed
        # for the serialized messages in a batch. Roughly, this field may be considered
        # the best effort maximum size of a batch. A batch will fill with messages
        # until this size is reached (or the max message count, or batch timeout is
        # exceeded). If adding a new message to the batch would cause the batch to
        # exceed the preferred max bytes, then the current batch is closed and written
        # to a block, and a new batch containing the new message is created. If a
        # message larger than the preferred max bytes is received, then its batch
        # will contain only that message. Because messages may be larger than
        # preferred max bytes (up to AbsoluteMaxBytes), some batches may exceed
        # the preferred max bytes, but will always contain exactly one transaction.
        PreferredMaxBytes: 2 MB
```

#### Batch timeout

Set the BatchTimeout value to the amount of time, in seconds, to wait after the first transaction arrives before cutting the block. If you set this value too low, you risk preventing the batches from filling to your preferred size. Setting this value too high, however, can cause the orderer to wait for blocks and overall performance and latency to degrade. A general guideline is to set the value of BatchTimeout to be at a minimum the max message count divided by the maximum transactions per second.

#### Max message count

Set the Max message count value to the maximum number of transactions to include in a single block.

#### Absolute max bytes

Set the Absolute max bytes value to the largest block size in bytes for the ordering service to cut. No transaction can be larger than the value of Absolute max bytes. This setting can typically be 2-10 times larger than the Preferred max bytes value. Note: The maximum recommended size is 49 MB, based on the headroom needed for the default grpc size limit of 100 MB.

#### Preferred max bytes

Set the Preferred max bytes value to your ideal block size in bytes, which must be less than the Absolute max bytes. A minimum transaction size, one that contains no endorsements, is around 1 KB. If you add 1 KB per required endorsement, a typical transaction size is approximately 3-4 KB. Therefore, it is recommended to set the value of Preferred max bytes to be close to the Max message count multiplied by the expected average transaction size. At run time, whenever possible, blocks will not exceed this size. If a transaction arrives that causes the block to exceed the Absolute max bytes size, the block will be cut and the transaction included in a new block. The new block will be cut with the single transaction ONLY, which also cannot exceed the Absolute max bytes.

## Application considerations

When designing your application architecture, various choices can affect the overall performance of both the application and the network. These application considerations are described below.

### Avoid CouchDB for high-throughput applications

CouchDB performance is noticably slower than embedded LevelDB, sometimes by a factor of 2x slower. The only additional capability that a CouchDB state database provides is JSON (rich) queries, as stated in the [Fabric state database documentation](./deploypeer/peerplan.html#state-database). In addition, to ensure the integrity of the state data you must never allow direct access to CouchDB data (for instance via Fauxton UI). 

CouchDB as a state database has other limitations which reduce overall network performance and require additional hardware resource (and cost). For one, JSON queries are not re-executed at validation time, so Fabric does not offer built-in protection from phantom reads. Such protection is provided, however, by range queries. (Your application must be designed to tolerate phantom reads when using JSON queries, such as when records are added to state between the times of chaincode execution and block validation). You should therefore consider using range queries based on additional keys for high-throughput applications, rather than using CouchDB with JSON queries.

Alternatively, consider using an off-chain store to support queries, as seen in the [Off-chain sample](https://github.com/hyperledger/Fabric-samples/tree/main/off_chain_data). Using an off-chain store for queries gives you much more control over the data, and query transactions do not affect Hyperledger Fabric peer and network performance. It also enables you to use a fit-for-purpose data store for off-chain storage, such as an SQL database or analytics service more aligned with your query needs.

CouchDB performance also degrades more than LevelDB does as the amount of state data increases, requiring you to provide adequate resources for CouchDB instances for the life of the application.

### General chaincode query considerations

Avoid writing chaincode queries that could be unbounded in the amount of data returned, or bounded but returning an amount of data large enough to cause a transaction to time out. It is against Fabric best practices for transactions to run for a long time, so ensure that your queries are optimized to limit the amount of data returned. If JSON queries are used, ensure they are indexed. See [Good practice for queries](./couchdb_as_state_database.html#good-practices-for-queries) for guidance on JSON queries.

### Use the Peer Gateway Service

The Peer Gateway Service (new in v2.4) and new Fabric-Gateway client SDKs offer substantial enhancements over the legacy Go, Java, and Node SDKs. The Peer Gateway Service provides much improved throughput, and more capabilities and reduced complexity to client applications. For example, the service automatically (attempts to) collect the endorsements required to satisfy not only the chaincode endorsement policy, but also any state-based endorsement policies included with transaction simulation (which is not possible using the legacy SDKs).

The Peer Gateway Service also reduces the number of network connections a client needs to maintain to submit a transaction. Using the legacy SDKs, clients may need to connect to multiple peer and orderer nodes across organizations. By contrast, the Peer Gateway Service service enables an application to connect to a single trusted peer, which then collects endorsements from other peer and orderer nodes and submits the transaction on behalf of the client application. Your application can target multiple trusted peers for high concurrency and redundancy, when necessary.

Changing your application development environment from the legacy SDKs to the new Peer Gateway Service also reduces client CPU and memory resource requirements, with a modest increase in peer resource requirements.

See the [Sample gateway application](https://github.com/hyperledger/Fabric-samples/blob/main/full-stack-asset-transfer-guide/docs/ApplicationDev/01-FabricGateway.md) for more details about the Peer Gateway Service.

### Payload size

The amount of data that is submitted to a transaction, along with the amount of data written to keys in a transaction will affect the application performance. Note that the payload size includes more than just the data. It includes structures required by Fabric plus client and endorsing peer signatures.

Suffice to say large payload sizes are an anti-pattern in any blockchain solution. Consider storing large data off-chain and storing a hash of the data on-chain.

### Chaincode language

Go chaincode has shown the best performance in Hyperledger Fabric environments, followed by Node chaincode. Java chaincode performance is the least performant of the three, and is not recommended for high-throughput applications.

### Node chaincode

Node is an asynchronous runtime implementation that utilizes only a single thread to execute code. Node does run background threads for activities such as garbage collection, but it may not improve performance to allocate more than two vCPUs (generally equivalent to the number of concurrent threads which can be executed) to a Node chaincode (for example on Kubernetes which is limited to available resources). It is worth monitoring performance of Node chaincode to see how much vCPU it uses. Node chaincode is self-limiting and not unbounded, though you could also assign resource restrictions to Node chaincode processes. 

Prior to Node v12, a Node process was limited to 1.5 GB of Memory by default, which to increase for running Node chaincode required passing a parameter to the Node executable. You should not be running chaincode processes on any version earlier than Node v12, and Hyperledger Fabric v2.5 mandates using Node v16, or later. Various parameters can be provided when launching Node chaincode, but few if any scenarios would require overriding the default values for Node v12 and later.

### Go chaincode

The Golang runtime implementation is ideal for concurrency. It is capable of using all CPUs available to it, and thus, is only limited by the resources allocated to the chaincode process. No tuning is required.

### Chaincode processes and channels

Hyperledger Fabric reuses chaincode processes across channels if the chaincode ID and versions match. For example, if a peer is joined to two channels with the same chaincode ID and version deployed, the peer will only interact with one chaincode process for both channels. This can place more load on the chaincode process than expected, especially for Node chaincode which is self-limiting. In this case, you can either use Go chaincode or deploy Node chaincode to each channel using different IDs or version numbers to ensure a distinct chaincode process per channel.

### Endorsement policies

For a transaction to be committed as valid, it must contain the signatures required to satisfy the chaincode endorsement policy and any state-based endorsement policies. The Peer Gateway Service (Fabric v2.4 and later) will only send requests to enough peers to satisfy the collection of policies (and will try other peers if the preferred peers are not available). Endorsement policies therefore affect performance because they dictate how many peers (and their signatures) are required for a transaction to be committed as valid.

### Private Data Collections (PDCs) vs World State

In general, the decision of whether to use Private Data Collections (PDCs) is an application architecture decision and not a performance decision. However, it should be noted that, for example, using a PDC to store an asset rather than using the world state results in approximately half the TPS.

### Single or multiple channel architecture

When designing your application, consider whether a single channel or multiple channels can be utilized. If data silos are not a problem for a given scenario, the application could use multiple channel architecture to improve performance (because multiple channels enable using more of the peer resource).

## CouchDB considerations

As mentioned, CouchDB is not recommended for high-throughput applications, and all limitations should be considered, as described below.

### Resources

Ensure monitoring of resources used by CouchDB instances, because as the size of the state database increases, CouchDB consumes more resources.

### CouchDB cache

When using an external CouchDB state database, read delays during the endorsement and validation transaction phases have shown to be a performance bottleneck. In Fabric v2.x, the peer cache replaces many of these expensive lookups with fast local cache reads.

The cache will not improve performance of JSON queries.

See the **CouchDB Cache setting** details above (in **Peer Considerations**) for information on configuring the cache.

### Indexes

Include CouchDB indexes with any chaincode that utilizes CouchDB JSON queries. Test queries to ensure that indexes are utilized, and avoid queries that cannot use indexes. For example, use of the query operators $or, $in, $regex result in full data scans. See details in [Hyperledger Fabric Good Practices For Queries](./couchdb_as_state_database.html#good-practices-for-queries).

Optimize your queries, because complex queries will take more time even with indexing. Ensure that your queries result in a bounded set of data. Hyperledger Fabric may also limit the total number of results returned.

You can check the peer and CouchDB logs to see how long queries are taking, and whether any query was unable to use an index from a warning log entry stating the query "should be indexed".

If queries are taking a long time, increasing the CPU/Memory available to CouchDB or using faster storage may improve their performance.

Check CouchDB logs for warnings such as "The number of documents examined is high in proportion to the number of results returned. Consider adding a more specific index to improve this." These messages indicate that your indexes could be improved because a high number of documents are being scanned.

### Bulk update

Hyperledger Fabric uses bulk update calls to CouchDB to improve CouchDB performance. A bulk update is done at the block level, so including more transactions in a block can improve throughput. However, increasing the time before a block is cut to include more transactions will also have an impact on latency.

## HSM

Using HSMs in a Fabric network will have an impact on performance. It is not possible to quantify the impact, but variables such as HSM performance and its network connection will impact the overall performance of the network.

If you have configured Peers, Orderers, and Clients to use an HSM, then anything that requires signing will utilize the HSM, including creation of blocks by orderers, endorsements and block events by peers, and all client requests.

Note that HSMs are NOT involved in the verification of signatures, which is done by each node.

## Other performance considerations

Sending single transactions periodically has a latency correlating to the block cutting parameters. For example, if you send a transaction of 100 Bytes with a  BatchTimeout of 2 seconds, then the time from transaction submission to commitment will be just over 2 seconds. However, this is not a true benchmark of your Fabric network performance, which should be obtained by submitting multiple transactions simultaneously.
