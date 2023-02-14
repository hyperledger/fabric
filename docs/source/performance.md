# Performance considerations

Various Hyperledger Fabric component, configuration, and workflow decisions contribute to the overall performance of your network. Managing these variables, such as the number of channels, chaincode implementations, and transaction policies, can be complex with participating organizations contributing their own hardware and networking infrastructures to the environment. 

This topic walks you through the considerations that can help optimize performance for your Hyperledger Fabric network. 

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

The ordering service uses raft consensus to cut blocks, factors such as the number of orderers in consensus and block cutting parameters will affect performance.

### Number of orderers

The number of ordering service nodes you have will impact performance as all orderers are involved in Raft consensus. Five ordering service nodes will provide two orderer crash fault tolerance (a majority must remain available) and is a good starting point. Additional ordering service nodes may reduce performance. You have the option to deploy different sets of ordering service nodes for each channel if you find that a single set of ordering service nodes becomes a bottleneck.

### SendBufferSize

Prior to Fabric v2.5 the SendBufferSize of the orderer was set to 10 as a default which causes a bottleneck. In Fabric 2.5 this was changed to 100 which provides much better throughput. The configuration for this can be found in orderer.yaml:

```yaml
General:
    Cluster:
        # SendBufferSize is the maximum number of messages in the egress buffer.
        # Consensus messages are dropped if the buffer is full, and transaction
        # messages are waiting for space to be freed.
        SendBufferSize: 100
```

### Block cutting parameters in channel configuration

A larger block size and timeout could increase the throughput but latency would increase as well.

You might want to try to configure the ordering service with more transactions per block and longer block cutting times to see if that helps. We have seen this increase the overall throughput at the cost of additional latency.

The following three parameters work together to control when a block is cut, based on a combination of setting the maximum number of transactions in a block as well as the block size itself. These are defined when you create or update a channel configuration. If you use configtxgen and configtx.yaml as a starting point for creating channels then the following section applies in configtx.yaml:

```yaml
Orderer: &OrdererDefaults
    # Batch Timeout: The amount of time to wait before creating a batch.
    BatchTimeout: 2s

    # Batch Size: Controls the number of messages batched into a block.
    # The orderer views messages opaquely, but typically, messages may
    # be considered to be Fabric transactions.  The 'batch' is the group
    # of messages in the 'data' field of the block.  Blocks will be a few kb
    # larger than the batch size, when signatures, hashes, and other metadata
    # is applied.
    BatchSize:

        # Max Message Count: The maximum number of messages to permit in a
        # batch.  No block will contain more than this number of messages.
        MaxMessageCount: 500

        # Absolute Max Bytes: The absolute maximum number of bytes allowed for
        # the serialized messages in a batch. The maximum block size is this value
        # plus the size of the associated metadata (usually a few KB depending
        # upon the size of the signing identities). Any transaction larger than
        # this value will be rejected by ordering.
        # It is recommended not to exceed 49 MB, given the default grpc max message size of 100 MB
        # configured on orderer and peer nodes (and allowing for message expansion during communication).
        AbsoluteMaxBytes: 10 MB

        # Preferred Max Bytes: The preferred maximum number of bytes allowed
        # for the serialized messages in a batch. Roughly, this field may be considered
        # the best effort maximum size of a batch. A batch will fill with messages
        # until this size is reached (or the max message count, or batch timeout is
        # exceeded).  If adding a new message to the batch would cause the batch to
        # exceed the preferred max bytes, then the current batch is closed and written
        # to a block, and a new batch containing the new message is created.  If a
        # message larger than the preferred max bytes is received, then its batch
        # will contain only that message.  Because messages may be larger than
        # preferred max bytes (up to AbsoluteMaxBytes), some batches may exceed
        # the preferred max bytes, but will always contain exactly one transaction.
        PreferredMaxBytes: 2 MB
```

#### Absolute max bytes

Set this value to the largest block size in bytes that can be cut by the ordering service. No transaction may be larger than the value of Absolute max bytes. Usually, this setting can safely be two to ten times larger than your Preferred max bytes. Note: The maximum recommended size is 49MB based on the headroom needed for the default grpc size limit of 100MB.

#### Max message count

Set this value to the maximum number of transactions that can be included in a single block.

#### Preferred max bytes

Set this value to the ideal block size in bytes, but it must be less than Absolute max bytes. A minimum transaction size, one that contains no endorsements, is around 1KB. If you add 1KB per required endorsement, a typical transaction size is approximately 3-4KB. Therefore, it is recommended to set the value of Preferred max bytes to be around Max message count times expected averaged transaction size. At run time, whenever possible, blocks will not exceed this size. If a transaction arrives that causes the block to exceed this size, the block is cut and a new block is created for that transaction. But if a transaction arrives that exceeds this value without exceeding the Absolute max bytes, the transaction will be included. If a transaction arrives that is larger than Preferred max bytes, then a block will be cut with a single transaction, and that transaction size can be no larger than Absolute max bytes.

Together, these parameters can be configured to optimize throughput of your orderer.

#### Batch timeout

Set the BatchTimeout value to the amount of time, in seconds, to wait after the first transaction arrives before cutting the block. If you set this value too low, you risk preventing the batches from filling to your preferred size. Setting this value too high can cause the orderer to wait for blocks and overall performance and latency to degrade. In general, we recommend that you set the value of BatchTimeout to be at least max message count / maximum transactions per second.

## Application considerations

When designing the application architecture, decisions can affect the overall performance of the network and the application. Here we cover some of the considerations.

### Avoid CouchDB for high throughput applications

CouchDB performance is noticably slower than embedded LevelDB, sometimes by a factor of 2x slower. The only additional capability that a CouchDB state database provides is JSON (rich) queries as stated in the [Fabric state database documentation](./deploypeer/peerplan.html#state-database). You should also never allow direct access to the CouchDB data (for instance via Fauxton UI) to ensure the integrity of the state data.

CouchDB as a state database also has other limitations which will have impacts on performance and requires additional hardware resource (and cost). Additionally, JSON queries are not re-executed at validation time and thus there is no built-in protection from phantom reads as there is with range queries (your application must be designed to tolerate phantom reads when using JSON queries, for example when records are added to state between the time of chaincode execution and block validation). For these reasons, consider using range queries based on additional keys rather than using CouchDB JSON with queries.

Alternatively, consider using an off-chain store to support queries, as seen in the [Off-chain sample](https://github.com/hyperledger/Fabric-samples/tree/main/off_chain_data). Using an off-chain store for queries gives you much more control over the data and query transactions do not affect the Fabric peer and network performance. It also enables you to use a fit-for-purpose data store for off-chain storage, for example you could use a SQL database or an analytics service more aligned with your query needs.

CouchDB performance degrades more than LevelDB as the amount of state data increases, requiring you to provide adequate resources for CouchDB instances for the life of the application.

### General chaincode query considerations

Avoid writing queries that could be unbounded in the amount of data returned, and even if it's bounded if it's likely to return a large amount of data then this will cause a transaction to run for a long time and possibly time out. It's not best practice for fabric transactions to run for a long time, so ensure your queries are optimised to not return large amounts of data. If JSON queries are used ensure they are indexed. See [Good practice for queries](./couchdb_as_state_database.html#good-practices-for-queries) for guidance on JSON queries.

### Use the new peer gateway service

The peer Gateway Service and the new Fabric-Gateway client SDKs are a substantial improvement over the legacy Go, Java and Node SDKs. Not only do they provide much improved throughput, they also provide better capability reducing the complexity of a client application. For example the Gateway SDKs will automatically collect enough endorsements to satisfy not only the chaincode endorsement policy but also any state-based endorsement policies that get included when the transaction is simulated, something that was not possible with the legacy SDKs.

It also reduces the number of network connections a client needs to maintain in order for a client to submit a transaction. Previously, clients may need to connect to multiple peer and orderer nodes across organizations. The peer Gateway Service service enables an application to target a single trusted peer, then the peer Gateway Service connects to other peer and orderer nodes to gather endorsements and submit the transaction on behalf of the client application. Of course, you may want to target multiple trusted peers for high concurrency and redundancy.

One point to consider is that shifting from legacy SDKs to the new Peer Gateway service reduces the client CPU and memory resource requirements. However, it does increase the peer resource requirements slightly.

See the [Sample gateway application](https://github.com/hyperledger/Fabric-samples/blob/main/full-stack-asset-transfer-guide/docs/ApplicationDev/01-FabricGateway.md) for more details about the new peer Gateway Service.

### Payload size

The amount of data that is submitted to a transaction, along with the amount of data written to keys in a transaction will affect the application performance. Note that the payload size includes more than just the data. It includes structures required by Fabric plus client and endorsing peer signatures.

Suffice to say large payload sizes are an anti-pattern in any blockchain solution. Consider storing large data off-chain and storing a hash of the data on-chain.

### Chaincode language

Go chaincode performs best, followed by Node chaincode.  Java chaincode performance is the least performant and would not be recommended for high throughput applications.

### Node chaincode

Node is an asynchronous runtime implementation that utilises only a single thread to execute code. It does however run background threads for activities such as garbage collection, however when allocating resources to a Node chaincode, for example in Kubernetes where you are limited to available resources, it doesn't make sense to allocate multiple vCPUs for Node chaincode. The number of vCPUs in a system usually refers to the number of concurrent threads that can be executed. It is worth monitoring performance of Node chaincode to see how much vCPU it uses but it probably doesn't make sense to allocate anything more than a max of 2 vCPUs for Node chaincode. In fact you could not assign any resource restrictions to Node chaincode as it is self limiting.

Prior to Node 12, a Node process was limited to 1.5Gb Memory by default and would require you to pass a parameter to the Node executable in order to increase this when running a node chaincode process. You should not be running Node chaincode processes on anything less than Node 12 now and Hyperledger Fabric 2.5 mandates that Node 16 or later should be used. There are various parameters that can be provided to the Node process when you launch your Node chaincode, however it's unlikely you would ever need to override the defaults of Node so no tuning would be required.

### Go chaincode

The Golang runtime provides an excellent implementation for concurrency. It is capable of using all CPUs available to it and thus is only limited by the resources allocated to the chaincode process to use. No tuning is required.

### Chaincode processes and channels

Hyperledger Fabric will reuse chaincode processes across channels if the chaincode id and versions match. For example if you have a peer joined to 2 channels (channel_a and channel_b) and you have deployed one chaincode to each channel with the same id and version number, then the peer will only interact with 1 chaincode process for both those channels. It will not try to work with a separate chaincode process for each channel. This means that you may be putting more load on that chaincode process than expected especially if it's node chaincode that is self limiting. If this is a problem you should consider using Go for your chaincode language or you could deploy the same chaincode with a different id or version to the other channel and that will ensure there is a chaincode process per channel.

### Endorsement policies

For a transaction to be committed as valid, it must contain enough signatures to satisfy the chaincode endorsement policy and any state-based endorsement policies. The peer Gateway service will only send requests to enough peers to satisfy this collection of policies (and will also try other peers if the preferred ones are not available). Thus we can see that endorsement policies will affect performance as it dictates how many peers and thus how many signatures are required to ensure that a transaction can be committed.

### Private Data Collections (PDCs) vs World State

In general the decision whether or not to use Private Data Collections (PDCs) will be an application architecture decision rather than a performance decision but for awareness it should be noted that, for example, using a PDC to store an asset verses using the world state will result in approximately half the TPS.

### Single channel vs multiple channel architecture

When designing your application, consideration should be given to whether a single channel or multiple channels can be utilised. If data silos are not a problem for a given application scenario, the application could be architected to use multiple channels to improve the performance. As previously mentioned it is possible to utilise more of the peer resource if more than one channel is active.

## Couchdb considerations

As mentioned earlier CouchDB is not recommended for high throughput applications, but if you do plan to use it these are the things that need to be considered.

### Resources

Ensure you monitor the resources of the CouchDB instances, as the larger the state database becomes the more resources CouchDB will consume.

### CouchDB cache

When using external CouchDB state database, read delays during endorsement and validation phases have historically been a performance bottleneck. In Fabric v2.x, the peer cache replaces many of these expensive lookups with fast local cache reads.

The cache will not improve performance of JSON queries.

See `CouchDB Cache setting` in the Peer Considerations section for information on configuring the cache.

### Indexes

Include CouchDB indexes along with any chaincode that utilises CouchDB JSON queries. Test queries to ensure indexes will be utilised, and avoid queries that can't use indexes. For example use of the query operators $or, $in, $regex result in full data scans. See [Hyperledger Fabric Good Practices For Queries](./couchdb_as_state_database.html#good-practices-for-queries).

Optimise your queries, complex queries will take more time even with indexing. Ensure your queries result in a bounded set of data. Remember Fabric may also limit the total number of results returned.

You can check peer and CouchDB logs to see how long queries are taking and also whether a query was unable to use an index from warning log entries stating the query "should be indexed".

If queries are taking a long time, you could try to increase CPU/Memory available to CouchDB or use faster storage.

Check CouchDB logs for warnings such as "The number of documents examined is high in proportion to the number of results returned. Consider adding a more specific index to improve this." These messages show you that your indexes may not be good enough for the query being performed as it is resulting in too many documents having to be scanned.

### Bulk update

Fabric uses bulk update calls to CouchDB to improve CouchDB performance. A bulk update is done at the block level so including more transactions in a block could potentially improve throughput. However increasing the time before a block is cut to include more transactions will have an impact on latency.

## HSM

Using HSMs within a Fabric network will have an impact on performance. It's not possible to quantify the impact here but things like the performance of the HSM and the network connection to the HSM will impact the Fabric network.

If you have configured Peers, Orderers, and Clients to use an HSM then anything that requires signing will utilise the HSM, including creation of blocks by orderers, endorsements and block events by peers, and all client requests.

HSMs are NOT involved in the verification of signatures. This is still done by the nodes themselves.

## Other miscellaneous considerations

Sending single transactions periodically will have a latency correlating to the block cutting parameters. For example if you send a transaction of 100 Bytes and the BatchTimeout is 2 seconds then the time from submission to being committed will be just over 2 seconds. This is not a true benchmark of your Fabric network performance, it's expected that multiple transactions will be submitted simultaneously to gauge the true performance of a Fabric network.
